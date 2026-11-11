package setting

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/internal/creem"
	"github.com/KiClover/orzAdmin/internal/walletpolicy"
)

// CreemProfileValidator 在持久化钱包 Profile 之前，用 Creem 官方接口做远程校验。
//
// 校验只读取 Product 与 Webhook 元数据，不创建、不修改任何远端对象；
// 任何错误文案都不会包含 API Key 或 Webhook Secret。
type CreemProfileValidator struct {
	cfg *conf.Config
	// newClient 便于测试注入假客户端。
	newClient func(creem.Options) (*creem.Client, error)
	// doer 便于测试注入 HTTP 传输层；生产环境为 nil。
	doer creem.Doer
}

// NewCreemProfileValidator 构造基于 Creem 的 Profile 校验器。
func NewCreemProfileValidator(cfg *conf.Config) *CreemProfileValidator {
	return &CreemProfileValidator{
		cfg:       cfg,
		newClient: creem.NewClient,
	}
}

var _ WalletProfileValidator = (*CreemProfileValidator)(nil)

// ValidateWalletProfile 实现 WalletProfileValidator。
func (v *CreemProfileValidator) ValidateWalletProfile(ctx context.Context, input WalletProfileValidationInput) error {
	environment, err := creemEnvironment(input.Environment)
	if err != nil {
		return walletSettingErr(WalletSettingCodeInvalidArgument, err.Error())
	}
	if strings.TrimSpace(input.APIKey) == "" {
		return walletSettingErr(WalletSettingCodeInvalidArgument, "api key is required")
	}
	if strings.TrimSpace(input.ProductID) == "" || strings.TrimSpace(input.WebhookID) == "" {
		return walletSettingErr(WalletSettingCodeInvalidArgument, "product id and webhook id are required")
	}

	client, err := v.clientFor(environment, input.APIKey)
	if err != nil {
		return walletSettingErr(WalletSettingCodeProviderRejected, err.Error())
	}

	product, err := client.GetProduct(ctx, input.ProductID)
	if err != nil {
		return providerValidationError("product", err)
	}
	if err := v.validateProduct(product); err != nil {
		return err
	}

	// Webhook 必须存在，且其 Secret 不能与其它环境的 Secret 重复。
	if _, err := client.GetWebhook(ctx, input.WebhookID); err != nil {
		return providerValidationError("webhook", err)
	}
	webhookSecret, err := client.GetWebhookSecret(ctx, input.WebhookID)
	if err != nil {
		return providerValidationError("webhook secret", err)
	}
	if strings.TrimSpace(webhookSecret.Secret) == "" {
		return walletSettingErr(WalletSettingCodeProviderRejected, "provider webhook secret is empty")
	}
	fingerprint := FingerprintSecret(webhookSecret.Secret)
	for _, other := range input.OtherWebhookFingerprints {
		if other == fingerprint {
			return walletSettingErr(WalletSettingCodeProviderRejected,
				"webhook secret is already used by another profile")
		}
	}

	// API Key 轮换时，旧 Key 必须仍能读取旧 Product，否则无法完成切换。
	if input.PreviousAPIKey != "" && input.PreviousAPIKey != input.APIKey && input.PreviousProductID != "" {
		previousClient, err := v.clientFor(environment, input.PreviousAPIKey)
		if err != nil {
			return walletSettingErr(WalletSettingCodeProviderRejected, err.Error())
		}
		if _, err := previousClient.GetProduct(ctx, input.PreviousProductID); err != nil {
			return providerValidationError("previous product", err)
		}
	}
	return nil
}

// validateProduct 校验 Product 的计费、币种、税模式与跳转地址白名单。
func (v *CreemProfileValidator) validateProduct(product *creem.Product) error {
	if product == nil {
		return walletSettingErr(WalletSettingCodeProviderRejected, "provider product not found")
	}
	if !strings.EqualFold(product.Status, "active") {
		return walletSettingErr(WalletSettingCodeProviderRejected, "provider product is not active")
	}
	if !strings.EqualFold(product.BillingType, "onetime") {
		return walletSettingErr(WalletSettingCodeProviderRejected, "provider product must be one-time")
	}
	if !strings.EqualFold(product.Currency, "usd") {
		return walletSettingErr(WalletSettingCodeProviderRejected, "provider product must be priced in USD")
	}
	if !strings.EqualFold(product.TaxMode, "inclusive") {
		return walletSettingErr(WalletSettingCodeProviderRejected, "provider product tax mode must be inclusive")
	}
	if product.BusinessNetPricing {
		return walletSettingErr(WalletSettingCodeProviderRejected, "provider product must not use business net pricing")
	}
	successURL := strings.TrimSpace(product.DefaultSuccessURL)
	if successURL == "" {
		return walletSettingErr(WalletSettingCodeProviderRejected, "provider product has no default success url")
	}
	allowed := v.cfg != nil && v.cfg.Wallet.OriginAllowed(successURL)
	if !allowed {
		return walletSettingErr(WalletSettingCodeProviderRejected,
			"provider product default success url is not in the allowed origins")
	}
	return nil
}

// clientFor 按环境与 API Key 构造 Creem 客户端。
func (v *CreemProfileValidator) clientFor(environment creem.Environment, apiKey string) (*creem.Client, error) {
	options := creem.Options{
		Environment:      environment,
		APIKey:           apiKey,
		Timeout:          walletpolicy.ProviderRequestTimeout,
		MaxResponseBytes: walletpolicy.ProviderMaxResponseBytes,
		Doer:             v.doer,
	}
	client, err := v.newClient(options)
	if err != nil {
		return nil, fmt.Errorf("creem client is unavailable: %w", err)
	}
	return client, nil
}

// creemEnvironment 把配置里的环境名转换成 Creem 环境枚举。
func creemEnvironment(environment string) (creem.Environment, error) {
	switch strings.ToUpper(strings.TrimSpace(environment)) {
	case ProfileEnvironmentTest:
		return creem.EnvironmentTest, nil
	case ProfileEnvironmentProduction:
		return creem.EnvironmentProduction, nil
	default:
		return "", fmt.Errorf("unknown profile environment %q", environment)
	}
}

// providerValidationError 把 Creem 错误映射为稳定的业务错误，且不泄漏上游响应体。
func providerValidationError(resource string, err error) error {
	switch {
	case errors.Is(err, creem.ErrNotFound):
		return walletSettingErr(WalletSettingCodeProviderRejected, "provider "+resource+" not found")
	case errors.Is(err, creem.ErrUnauthorized):
		return walletSettingErr(WalletSettingCodeProviderRejected, "provider credentials were rejected")
	case errors.Is(err, creem.ErrResponseTooLarge), errors.Is(err, creem.ErrInvalidResponse):
		return walletSettingErr(WalletSettingCodeUnavailable, "provider returned an unusable response")
	default:
		return walletSettingErr(WalletSettingCodeUnavailable, "provider is temporarily unavailable")
	}
}
