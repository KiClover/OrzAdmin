package creem

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// Webhook 验签。
//
// 规则（WalletDesign §8.2）：先保留原始 body 字节；creem-signature 头部必须恰好
// 有一个非空值，重复或歧义值直接拒绝；再在有限个候选 Secret 中逐一验证，要求
// 恰好一个命中。零命中意味着伪造或密钥已轮换，多命中意味着配置重复，两者都拒绝。

var (
	// ErrWebhookBodyEmpty 表示回调没有 body。
	ErrWebhookBodyEmpty = errors.New("creem webhook: body is empty")
	// ErrWebhookBodyTooLarge 表示回调 body 超过允许上限。
	ErrWebhookBodyTooLarge = errors.New("creem webhook: body is too large")
	// ErrWebhookSignatureMissing 表示缺少 creem-signature。
	ErrWebhookSignatureMissing = errors.New("creem webhook: signature header is missing")
	// ErrWebhookSignatureDuplicate 表示出现重复或歧义的 creem-signature。
	ErrWebhookSignatureDuplicate = errors.New("creem webhook: signature header is duplicated")
	// ErrWebhookSignatureInvalid 表示签名格式或内容不合法。
	ErrWebhookSignatureInvalid = errors.New("creem webhook: signature is invalid")
	// ErrWebhookSecretAmbiguous 表示多个候选 Secret 同时命中。
	ErrWebhookSecretAmbiguous = errors.New("creem webhook: multiple secrets match")
	// ErrWebhookSecretMissing 表示没有候选 Secret 可用。
	ErrWebhookSecretMissing = errors.New("creem webhook: no secret candidate is configured")
)

// maxWebhookSecretCandidates 是单次验签允许的候选 Secret 数量上限。
const maxWebhookSecretCandidates = 8

// SecretCandidate 是一个可参与验签的 Webhook Secret。
type SecretCandidate struct {
	// ProfileID 与 Environment 用于在命中后确定订单归属。
	ProfileID   string
	Environment wallet.ProviderEnvironment
	// Secret 是 Webhook 签名密钥，禁止写日志。
	Secret string
	// Retiring 表示该 Secret 已轮换但仍处于排空期。
	Retiring bool
}

// VerifyWebhookSignature 用原始 body 与候选 Secret 验证 creem-signature。
//
// 返回唯一命中的候选；任何歧义都返回错误。函数不记录 body、Secret 或签名原文。
func VerifyWebhookSignature(
	rawBody []byte,
	signatureHeaders []string,
	candidates []SecretCandidate,
) (SecretCandidate, error) {
	if len(rawBody) == 0 {
		return SecretCandidate{}, ErrWebhookBodyEmpty
	}
	if int64(len(rawBody)) > walletpolicy.WebhookMaxBodyBytes {
		return SecretCandidate{}, ErrWebhookBodyTooLarge
	}
	if len(signatureHeaders) == 0 {
		return SecretCandidate{}, ErrWebhookSignatureMissing
	}
	if len(signatureHeaders) > 1 {
		return SecretCandidate{}, ErrWebhookSignatureDuplicate
	}
	signature := strings.TrimSpace(signatureHeaders[0])
	if signature == "" {
		return SecretCandidate{}, ErrWebhookSignatureMissing
	}
	// Creem 使用 HMAC-SHA256 的十六进制摘要；这里要求 64 位十六进制，
	// 其它形态（base64、前缀、空格分隔）一律视为非法。
	if len(signature) != sha256.Size*2 {
		return SecretCandidate{}, ErrWebhookSignatureInvalid
	}
	provided, err := hex.DecodeString(strings.ToLower(signature))
	if err != nil {
		return SecretCandidate{}, ErrWebhookSignatureInvalid
	}

	usable := make([]SecretCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Secret == "" {
			continue
		}
		usable = append(usable, candidate)
	}
	if len(usable) == 0 {
		return SecretCandidate{}, ErrWebhookSecretMissing
	}
	if len(usable) > maxWebhookSecretCandidates {
		// 超出上限说明配置异常；宁可拒绝，也不做无界遍历。
		return SecretCandidate{}, ErrWebhookSecretAmbiguous
	}

	var matched []SecretCandidate
	for _, candidate := range usable {
		mac := hmac.New(sha256.New, []byte(candidate.Secret))
		mac.Write(rawBody)
		if hmac.Equal(mac.Sum(nil), provided) {
			matched = append(matched, candidate)
		}
	}
	switch len(matched) {
	case 1:
		return matched[0], nil
	case 0:
		return SecretCandidate{}, ErrWebhookSignatureInvalid
	default:
		return SecretCandidate{}, ErrWebhookSecretAmbiguous
	}
}

// SignWebhookPayload 计算与 Creem 一致的小写十六进制签名。
//
// 仅用于测试与本地联调；生产入站方向只做验签。
func SignWebhookPayload(secret string, rawBody []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	return hex.EncodeToString(mac.Sum(nil))
}
