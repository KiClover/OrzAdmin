package creem

import (
	"errors"
	"strings"
	"testing"

	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

const (
	webhookSecret    = "whsec_current_must_never_leak"
	webhookRetiring  = "whsec_retiring_must_never_leak"
	webhookBodyMark  = "WEBHOOK_BODY_MARKER_71ab"
	webhookProfileID = "wp_0123456789abcdef0123456789abcdef"
)

func webhookCandidate(secret string, retiring bool) SecretCandidate {
	return SecretCandidate{
		ProfileID:   webhookProfileID,
		Environment: wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST,
		Secret:      secret,
		Retiring:    retiring,
	}
}

func TestCreemWebhookVerifyWebhookSignatureAcceptsCurrentSecret(t *testing.T) {
	t.Parallel()

	body := []byte(`{"eventType":"checkout.completed","object":{"id":"chk_1"}}`)
	signature := SignWebhookPayload(webhookSecret, body)

	matched, err := VerifyWebhookSignature(body, []string{signature}, []SecretCandidate{webhookCandidate(webhookSecret, false)})
	if err != nil {
		t.Fatalf("合法签名应通过: %v", err)
	}
	if matched.ProfileID != webhookProfileID || matched.Retiring {
		t.Fatalf("命中候选不符: %+v", matched)
	}
}

func TestCreemWebhookVerifyWebhookSignatureAcceptsRetiringSecret(t *testing.T) {
	t.Parallel()

	body := []byte(webhookBodyMark)
	// 排空期的旧 Secret 仍必须能验签，避免轮换瞬间丢回调。
	signature := SignWebhookPayload(webhookRetiring, body)
	matched, err := VerifyWebhookSignature(body, []string{signature}, []SecretCandidate{
		webhookCandidate(webhookSecret, false),
		webhookCandidate(webhookRetiring, true),
	})
	if err != nil {
		t.Fatalf("旧 Secret 应通过: %v", err)
	}
	if !matched.Retiring || matched.Secret != webhookRetiring {
		t.Fatalf("应命中 retiring 候选: %+v", matched)
	}
}

func TestCreemWebhookVerifyWebhookSignatureAcceptsUppercaseHex(t *testing.T) {
	t.Parallel()

	body := []byte(webhookBodyMark)
	signature := strings.ToUpper(SignWebhookPayload(webhookSecret, body))
	if _, err := VerifyWebhookSignature(body, []string{" " + signature + " "}, []SecretCandidate{webhookCandidate(webhookSecret, false)}); err != nil {
		t.Fatalf("大小写与首尾空白不应影响验签: %v", err)
	}
}

func TestCreemWebhookVerifyWebhookSignatureRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	body := []byte(webhookBodyMark)
	good := SignWebhookPayload(webhookSecret, body)

	cases := []struct {
		name       string
		body       []byte
		headers    []string
		candidates []SecretCandidate
		want       error
	}{
		{name: "empty_body", body: nil, headers: []string{good}, candidates: []SecretCandidate{webhookCandidate(webhookSecret, false)}, want: ErrWebhookBodyEmpty},
		{name: "too_large", body: []byte(strings.Repeat("a", int(walletpolicy.WebhookMaxBodyBytes)+1)), headers: []string{good}, candidates: []SecretCandidate{webhookCandidate(webhookSecret, false)}, want: ErrWebhookBodyTooLarge},
		{name: "missing_header", body: body, headers: nil, candidates: []SecretCandidate{webhookCandidate(webhookSecret, false)}, want: ErrWebhookSignatureMissing},
		{name: "empty_header", body: body, headers: []string{"  "}, candidates: []SecretCandidate{webhookCandidate(webhookSecret, false)}, want: ErrWebhookSignatureMissing},
		{name: "duplicate_header", body: body, headers: []string{good, good}, candidates: []SecretCandidate{webhookCandidate(webhookSecret, false)}, want: ErrWebhookSignatureDuplicate},
		{name: "short_signature", body: body, headers: []string{"abcd"}, candidates: []SecretCandidate{webhookCandidate(webhookSecret, false)}, want: ErrWebhookSignatureInvalid},
		{name: "non_hex", body: body, headers: []string{strings.Repeat("z", 64)}, candidates: []SecretCandidate{webhookCandidate(webhookSecret, false)}, want: ErrWebhookSignatureInvalid},
		{name: "wrong_secret", body: body, headers: []string{SignWebhookPayload("another_secret", body)}, candidates: []SecretCandidate{webhookCandidate(webhookSecret, false)}, want: ErrWebhookSignatureInvalid},
		{name: "no_candidates", body: body, headers: []string{good}, candidates: nil, want: ErrWebhookSecretMissing},
		{name: "empty_secret_only", body: body, headers: []string{good}, candidates: []SecretCandidate{webhookCandidate("", false)}, want: ErrWebhookSecretMissing},
		{
			name: "ambiguous_same_secret", body: body, headers: []string{good},
			candidates: []SecretCandidate{webhookCandidate(webhookSecret, false), webhookCandidate(webhookSecret, true)},
			want:       ErrWebhookSecretAmbiguous,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := VerifyWebhookSignature(tc.body, tc.headers, tc.candidates)
			if !errors.Is(err, tc.want) {
				t.Fatalf("errors.Is(%v, %v) = false", err, tc.want)
			}
		})
	}
}

func TestCreemWebhookVerifyWebhookSignatureRejectsTooManyCandidates(t *testing.T) {
	t.Parallel()

	body := []byte(webhookBodyMark)
	headers := []string{SignWebhookPayload(webhookSecret, body)}
	candidates := make([]SecretCandidate, 0, 9)
	for i := 0; i < 9; i++ {
		candidates = append(candidates, webhookCandidate(webhookSecret+"-"+string(rune('a'+i)), false))
	}
	if _, err := VerifyWebhookSignature(body, headers, candidates); !errors.Is(err, ErrWebhookSecretAmbiguous) {
		t.Fatalf("候选过多应拒绝: %v", err)
	}
}

func TestCreemWebhookVerifyWebhookSignatureDoesNotLeakSecrets(t *testing.T) {
	t.Parallel()

	body := []byte(webhookBodyMark)
	_, err := VerifyWebhookSignature(body, []string{SignWebhookPayload("wrong", body)}, []SecretCandidate{webhookCandidate(webhookSecret, false)})
	if err == nil {
		t.Fatal("错误签名必须报错")
	}
	if strings.Contains(err.Error(), webhookSecret) || strings.Contains(err.Error(), webhookBodyMark) {
		t.Fatalf("错误信息不得泄露 Secret 或 body: %v", err)
	}
}

func TestCreemWebhookVerifyWebhookSignatureBindsRawBytes(t *testing.T) {
	t.Parallel()

	signed := []byte(`{"a":1}`)
	reordered := []byte(`{ "a": 1 }`)
	if _, err := VerifyWebhookSignature(reordered, []string{SignWebhookPayload(webhookSecret, signed)}, []SecretCandidate{webhookCandidate(webhookSecret, false)}); !errors.Is(err, ErrWebhookSignatureInvalid) {
		t.Fatalf("验签必须基于原始字节，格式化后应失败: %v", err)
	}
}
