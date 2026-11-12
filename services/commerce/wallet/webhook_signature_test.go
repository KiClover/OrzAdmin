package wallet

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	settings "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	creemprovider "github.com/KiClover/orzAdmin/services/commerce/wallet/provider/creem"
)

func TestWebhookRefUnmarshal(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"string form", `"chk_1"`, "chk_1"},
		{"object form", `{"id":"chk_2"}`, "chk_2"},
		{"object without id", `{"other":"x"}`, ""},
	}
	for _, tc := range cases {
		var ref webhookRef
		if err := json.Unmarshal([]byte(tc.raw), &ref); err != nil {
			t.Fatalf("%s: 解析失败: %v", tc.name, err)
		}
		if ref.ID != tc.want {
			t.Fatalf("%s: 期望 %q，实际 %q", tc.name, tc.want, ref.ID)
		}
	}
	var ref webhookRef
	if err := json.Unmarshal([]byte(`123`), &ref); err == nil {
		t.Fatal("数字形式的引用必须拒绝")
	}
}

func TestHeaderValuesPreservesDuplicates(t *testing.T) {
	headers := []*wallet.WebhookHeader{
		{Name: "creem-signature", Values: []string{"first"}},
		{Name: "X-Other", Values: []string{"ignored"}},
		{Name: "CREEM-SIGNATURE", Values: []string{"second", "third"}},
	}
	values := headerValues(headers, webhookSignatureHeader)
	if len(values) != 3 || values[0] != "first" || values[1] != "second" || values[2] != "third" {
		t.Fatalf("同名头未完整保留: %#v", values)
	}
	if got := headerValues(nil, webhookSignatureHeader); len(got) != 0 {
		t.Fatalf("缺失头应返回空: %#v", got)
	}
}

func TestWebhookSecretCandidatesSelectsBoundedSecrets(t *testing.T) {
	snapshot := rechargeSnapshot()
	snapshot.TestProfile.WebhookSecrets = []*settings.WalletWebhookSecretRuntime{
		{EndpointId: "ep_1", Fingerprint: "fp_1", Secret: "current-secret"},
		{EndpointId: "ep_2", Fingerprint: "fp_2", Secret: ""},
		{EndpointId: "ep_3", Fingerprint: "fp_3", Secret: "draining-secret", RetireAfter: testNow.UnixMilli()},
	}
	snapshot.ProductionProfile = &settings.WalletProviderProfileRuntime{
		Environment: providerEnvironmentProduction,
		ProfileId:   "wp_prod_1",
		WebhookSecrets: []*settings.WalletWebhookSecretRuntime{
			{EndpointId: "ep_4", Fingerprint: "fp_4", Secret: "prod-secret"},
		},
	}

	candidates := webhookSecretCandidates(snapshot, testNow)
	if len(candidates) != 3 {
		t.Fatalf("应收集 3 个非空 Secret，实际 %d", len(candidates))
	}
	want := map[string]bool{
		"current-secret":  false,
		"draining-secret": true,
		"prod-secret":     false,
	}
	for _, candidate := range candidates {
		retiring, ok := want[candidate.Secret]
		if !ok {
			t.Fatalf("意外候选: %q", candidate.Secret)
		}
		if candidate.Retiring != retiring {
			t.Fatalf("Secret %q 的 drain 标记错误: %v", candidate.Secret, candidate.Retiring)
		}
		if candidate.ProfileID == "" || candidate.Environment == wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_UNSPECIFIED {
			t.Fatalf("候选缺少归属信息: %+v", candidate)
		}
		delete(want, candidate.Secret)
	}
	if len(want) != 0 {
		t.Fatalf("遗漏候选: %v", want)
	}
}

func TestWebhookEventProviderIDs(t *testing.T) {
	var paid webhookEvent
	raw := `{"eventType":"transaction.paid","object":{"id":"txn_1","order":{"id":"ord_1","checkout":"chk_1"}}}`
	if err := json.Unmarshal([]byte(raw), &paid); err != nil {
		t.Fatalf("解析 transaction.paid 失败: %v", err)
	}
	ids := paid.providerIDs()
	if ids.transactionID != "txn_1" || ids.orderID != "ord_1" || ids.checkoutID != "chk_1" {
		t.Fatalf("transaction.* 事件 ID 提取错误: %+v", ids)
	}

	var completed webhookEvent
	raw = `{"eventType":"checkout.completed","object":{"id":"chk_2","request_id":"req_2"}}`
	if err := json.Unmarshal([]byte(raw), &completed); err != nil {
		t.Fatalf("解析 checkout.completed 失败: %v", err)
	}
	ids = completed.providerIDs()
	if ids.checkoutID != "chk_2" || ids.requestID != "req_2" || ids.transactionID != "" {
		t.Fatalf("checkout.* 事件 ID 提取错误: %+v", ids)
	}

	var nested webhookEvent
	raw = `{"eventType":"checkout.completed","object":{"id":"chk_3","transaction":{"id":"txn_3"}}}`
	if err := json.Unmarshal([]byte(raw), &nested); err != nil {
		t.Fatalf("解析嵌套交易失败: %v", err)
	}
	if ids := nested.providerIDs(); ids.transactionID != "txn_3" || ids.checkoutID != "chk_3" {
		t.Fatalf("对象形式引用解析错误: %+v", ids)
	}
}

func TestProcessWebhookRejectsBeforeSnapshot(t *testing.T) {
	svc := &CommerceWalletService{}
	ctx := context.Background()

	resp, err := svc.processPaymentWebhook(ctx, &wallet.ProcessPaymentWebhookReq{
		Provider: wallet.PaymentProvider_PAYMENT_PROVIDER_UNSPECIFIED,
		Body:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("不支持的 provider 应返回 accepted=false 而不是 RPC 错误: %v", err)
	}
	if resp.GetAccepted() || !strings.Contains(resp.GetMessage(), "provider") {
		t.Fatalf("响应错误: %+v", resp)
	}

	resp, err = svc.processPaymentWebhook(ctx, &wallet.ProcessPaymentWebhookReq{
		Provider: wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if err != nil {
		t.Fatalf("空 body 应返回 accepted=false: %v", err)
	}
	if resp.GetAccepted() || !strings.Contains(resp.GetMessage(), "empty") {
		t.Fatalf("空 body 响应错误: %+v", resp)
	}
}

func TestProcessWebhookRequiresSettingsSnapshot(t *testing.T) {
	svc := &CommerceWalletService{Runtime: &Service{}}
	_, err := svc.processPaymentWebhook(context.Background(), &wallet.ProcessPaymentWebhookReq{
		Provider: wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
		Body:     []byte(`{}`),
	})
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_UNAVAILABLE) {
		t.Fatalf("缺少快照应返回 UNAVAILABLE，实际 code=%d", code)
	}
}

func TestProcessWebhookRejectsBadSignature(t *testing.T) {
	svc := &CommerceWalletService{
		Runtime: newPolicyRuntime(t, rechargeSnapshot(), fakePresence{has: true}),
	}
	body := webhookBody()
	ctx := context.Background()

	cases := []struct {
		name    string
		headers []*wallet.WebhookHeader
	}{
		{"wrong secret", []*wallet.WebhookHeader{
			{Name: webhookSignatureHeader, Values: []string{creemprovider.SignWebhookPayload("wrong", body)}},
		}},
		{"missing header", nil},
		{"duplicate header", []*wallet.WebhookHeader{
			{Name: webhookSignatureHeader, Values: []string{
				creemprovider.SignWebhookPayload(testWebhookSecret, body),
				creemprovider.SignWebhookPayload(testWebhookSecret, body),
			}},
		}},
		{"not hex", []*wallet.WebhookHeader{
			{Name: webhookSignatureHeader, Values: []string{"zzzz"}},
		}},
	}
	for _, tc := range cases {
		resp, err := svc.processPaymentWebhook(ctx, &wallet.ProcessPaymentWebhookReq{
			Provider:   wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
			Body:       body,
			Headers:    tc.headers,
			ReceivedAt: testNow.UnixMilli(),
		})
		if err != nil {
			t.Fatalf("%s: 不应返回 RPC 错误: %v", tc.name, err)
		}
		if resp.GetAccepted() {
			t.Fatalf("%s: 必须拒绝", tc.name)
		}
		if strings.Contains(resp.GetMessage(), testWebhookSecret) {
			t.Fatalf("%s: 响应泄漏 Secret: %q", tc.name, resp.GetMessage())
		}
	}
}

func TestProcessWebhookRejectsMalformedPayload(t *testing.T) {
	svc := &CommerceWalletService{
		Runtime: newPolicyRuntime(t, rechargeSnapshot(), fakePresence{has: true}),
	}
	body := []byte(`{"eventType":`)
	resp, err := svc.processPaymentWebhook(context.Background(), &wallet.ProcessPaymentWebhookReq{
		Provider: wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
		Body:     body,
		Headers: []*wallet.WebhookHeader{
			{Name: webhookSignatureHeader, Values: []string{creemprovider.SignWebhookPayload(testWebhookSecret, body)}},
		},
		ReceivedAt: testNow.UnixMilli(),
	})
	if err != nil {
		t.Fatalf("签名有效但 JSON 损坏应返回 accepted=false: %v", err)
	}
	if resp.GetAccepted() || !strings.Contains(resp.GetMessage(), "malformed") {
		t.Fatalf("损坏 JSON 响应错误: %+v", resp)
	}
}
