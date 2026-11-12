package setting

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/KiClover/orzAdmin/ent"
	settingEnt "github.com/KiClover/orzAdmin/ent/setting"
	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/cloudwego/kitex/pkg/kerrors"
	"github.com/sirupsen/logrus"
)

// 钱包设置存在 settings 表的固定键下，值是 WalletRuntimeSettings 的 JSON 快照。
const (
	// WalletConfigKey 是钱包配置在 settings 表中的键。
	WalletConfigKey = "wallet_config"

	// walletConfigMaxBytes 限制单个快照的大小，超出的配置一律拒绝。
	walletConfigMaxBytes = 64 << 10

	// 币种取值固定，不随部署变化。
	WalletCurrencyCNY = "CNY"
	WalletCurrencyUSD = "USD"

	// 充值模式取值。
	RechargeModeDisabled   = "DISABLED"
	RechargeModeTest       = "TEST"
	RechargeModeProduction = "PRODUCTION"

	// Provider 环境取值。
	ProfileEnvironmentTest       = "TEST"
	ProfileEnvironmentProduction = "PRODUCTION"

	// walletSecretMasked 是管理视图里秘密字段的固定掩码，不含任何明文片段。
	walletSecretMasked = "********"

	// 汇率以"1 USD = N CNY"表示，最多 6 位小数，范围 0.100000..100.000000。
	walletRateDecimalScale = int64(1_000_000)
	walletRateMinScaled    = int64(100_000)
	walletRateMaxScaled    = int64(100_000_000)

	// walletSecretDrainWindow 是 webhook Secret 轮换后的保留期。
	walletSecretDrainWindow = 7 * 24 * time.Hour

	// walletMaxWebhookSecrets 是单个 Profile 允许并存的最大 Secret 数量。
	walletMaxWebhookSecrets = 8

	// walletSampleWalletAmount 是管理视图示例换算使用的 CNY 分金额（100 元）。
	walletSampleWalletAmount = int64(10_000)

	// walletFingerprintLen 是 SHA-256 十六进制指纹的长度。
	walletFingerprintLen = sha256.Size * 2
)

// 钱包设置业务错误码。HTTP 层据此映射为稳定的 4xx/5xx。
const (
	// WalletSettingCodeInvalidArgument 表示参数或配置内容不合法。
	WalletSettingCodeInvalidArgument int32 = 2
	// WalletSettingCodeNotFound 表示钱包配置不存在。
	WalletSettingCodeNotFound int32 = 16
	// WalletSettingCodeRevisionConflict 表示 expected_revision 与当前版本不一致。
	WalletSettingCodeRevisionConflict int32 = 20
	// WalletSettingCodeProviderRejected 表示 Provider 远程校验未通过。
	WalletSettingCodeProviderRejected int32 = 21
	// WalletSettingCodeUnavailable 表示 Provider 暂时不可用或校验器未配置。
	WalletSettingCodeUnavailable int32 = 22
	// WalletSettingCodeInternal 表示配置存储或解码失败。
	WalletSettingCodeInternal int32 = 23
)

// walletSettingErr 返回带业务错误码的安全错误（不含秘密与 SQL 细节）。
func walletSettingErr(code int32, msg string) error {
	return kerrors.NewBizStatusError(code, msg)
}

// WalletRate 是规范化的汇率：1 USD = Numerator/Denominator CNY。
type WalletRate struct {
	// Numerator 与 Denominator 是约分后的正整数分子/分母。
	Numerator   int64
	Denominator int64
	// Scaled 是放大 1,000,000 倍后的整数，便于校验位数与范围。
	Scaled int64
	// Canonical 是固定 6 位小数的十进制字符串。
	Canonical string
}

// ParseWalletRate 解析"1 USD = N CNY"的十进制字符串。
//
// 允许 1~3 位整数与最多 6 位小数，范围 [0.100000, 100.000000]；
// 拒绝正负号、科学计数法、多余小数点与前后缀字符。
func ParseWalletRate(raw string) (WalletRate, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return WalletRate{}, fmt.Errorf("rate is empty")
	}
	if trimmed != raw {
		return WalletRate{}, fmt.Errorf("rate must not contain surrounding spaces")
	}

	intPart, fracPart := trimmed, ""
	hasDecimalPoint := false
	if idx := strings.IndexByte(trimmed, '.'); idx >= 0 {
		intPart, fracPart = trimmed[:idx], trimmed[idx+1:]
		hasDecimalPoint = true
	}
	if intPart == "" || (hasDecimalPoint && fracPart == "") {
		return WalletRate{}, fmt.Errorf("rate must be a decimal number")
	}
	for _, r := range intPart {
		if r < '0' || r > '9' {
			return WalletRate{}, fmt.Errorf("rate contains invalid character")
		}
	}
	for _, r := range fracPart {
		if r < '0' || r > '9' {
			return WalletRate{}, fmt.Errorf("rate contains invalid character")
		}
	}
	if len(intPart) > 3 {
		return WalletRate{}, fmt.Errorf("rate integer part must be at most 3 digits")
	}
	if len(intPart) > 1 && intPart[0] == '0' {
		return WalletRate{}, fmt.Errorf("rate must not have leading zeros")
	}
	if len(fracPart) > 6 {
		return WalletRate{}, fmt.Errorf("rate must have at most 6 decimal places")
	}

	scaled := int64(0)
	for _, r := range intPart {
		scaled = scaled*10 + int64(r-'0')
	}
	for i := 0; i < 6; i++ {
		scaled *= 10
		if i < len(fracPart) {
			scaled += int64(fracPart[i] - '0')
		}
	}
	if scaled < walletRateMinScaled || scaled > walletRateMaxScaled {
		return WalletRate{}, fmt.Errorf("rate must be between 0.1 and 100")
	}

	divisor := gcdInt64(scaled, walletRateDecimalScale)
	return WalletRate{
		Numerator:   scaled / divisor,
		Denominator: walletRateDecimalScale / divisor,
		Scaled:      scaled,
		Canonical:   fmt.Sprintf("%d.%06d", scaled/walletRateDecimalScale, scaled%walletRateDecimalScale),
	}, nil
}

// gcdInt64 返回两个正整数的最大公约数。
func gcdInt64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

// SampleProviderAmount 按"向上取整到 USD 美分"计算示例换算：
// provider_amount = ceil(wallet_amount * denominator / numerator)。
func SampleProviderAmount(walletAmount int64, rate WalletRate) (int64, error) {
	if walletAmount <= 0 {
		return 0, fmt.Errorf("wallet amount must be positive")
	}
	if rate.Numerator <= 0 || rate.Denominator <= 0 {
		return 0, fmt.Errorf("rate must be positive")
	}

	product := new(big.Int).Mul(big.NewInt(walletAmount), big.NewInt(rate.Denominator))
	product.Add(product, big.NewInt(rate.Numerator-1))
	product.Div(product, big.NewInt(rate.Numerator))
	if !product.IsInt64() {
		return 0, fmt.Errorf("provider amount overflows int64")
	}
	return product.Int64(), nil
}

// FingerprintSecret 返回秘密的 SHA-256 十六进制指纹，用于配置比对与审计。
func FingerprintSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// walletWebhookSecret 是快照中保留的 webhook Secret 及其轮换元数据。
type walletWebhookSecret struct {
	Fingerprint string `json:"fingerprint"`
	Secret      string `json:"secret"`
	EndpointID  string `json:"endpoint_id"`
	DisabledAt  int64  `json:"disabled_at"`
	RetireAfter int64  `json:"retire_after"`
}

// walletProviderProfile 是单个环境（TEST/PRODUCTION）的 Creem Profile。
type walletProviderProfile struct {
	// Environment 由 map 的键决定，不写入 JSON。
	Environment     string                `json:"-"`
	ProfileID       string                `json:"profile_id"`
	ProfileRevision int64                 `json:"profile_revision"`
	APIKey          string                `json:"api_key"`
	ProductID       string                `json:"product_id"`
	WebhookID       string                `json:"webhook_id"`
	WebhookSecrets  []walletWebhookSecret `json:"webhook_secrets"`
	VerifiedAt      int64                 `json:"verified_at"`
}

// walletSnapshot 是 settings.value 中 wallet_config 的完整内容。
// 字段顺序固定，便于人工比对与审计。
type walletSnapshot struct {
	Revision                    int64                             `json:"revision"`
	WalletCurrency              string                            `json:"wallet_currency"`
	ProviderCurrency            string                            `json:"provider_currency"`
	CNYPerUSDRate               string                            `json:"cny_per_usd_rate"`
	Enabled                     bool                              `json:"enabled"`
	RechargeMode                string                            `json:"recharge_mode"`
	RedemptionEnabled           bool                              `json:"redemption_enabled"`
	RedemptionRateLimit         uint32                            `json:"redemption_rate_limit"`
	RedemptionRateWindowSeconds uint32                            `json:"redemption_rate_window_seconds"`
	Profiles                    map[string]*walletProviderProfile `json:"profiles"`
	UpdatedBy                   string                            `json:"updated_by"`
	UpdatedAt                   int64                             `json:"updated_at"`

	// rate 是解码或写入时缓存的规范化汇率，不落库。
	rate WalletRate
}

// defaultWalletSnapshot 返回"未配置"时的关闭态快照。
// 与初始化种子写入的 JSON 内容必须一致（由测试保证）。
func defaultWalletSnapshot() *walletSnapshot {
	rate, err := ParseWalletRate("7.200000")
	if err != nil {
		// 常量字面量不可能非法，属于编程错误。
		panic("wallet: invalid default rate: " + err.Error())
	}
	return &walletSnapshot{
		Revision:                    0,
		WalletCurrency:              WalletCurrencyCNY,
		ProviderCurrency:            WalletCurrencyUSD,
		CNYPerUSDRate:               "7.200000",
		Enabled:                     false,
		RechargeMode:                RechargeModeDisabled,
		RedemptionEnabled:           false,
		RedemptionRateLimit:         3,
		RedemptionRateWindowSeconds: 600,
		Profiles:                    map[string]*walletProviderProfile{},
		UpdatedBy:                   "initialize",
		UpdatedAt:                   0,
		rate:                        rate,
	}
}

// cloneWalletSnapshot 深拷贝快照，避免补丁修改到已加载的基线。
func cloneWalletSnapshot(in *walletSnapshot) *walletSnapshot {
	if in == nil {
		return nil
	}
	out := *in
	out.Profiles = make(map[string]*walletProviderProfile, len(in.Profiles))
	for env, profile := range in.Profiles {
		if profile == nil {
			continue
		}
		copied := *profile
		copied.WebhookSecrets = append([]walletWebhookSecret(nil), profile.WebhookSecrets...)
		out.Profiles[env] = &copied
	}
	return &out
}

// decodeWalletSnapshot 严格解码快照：限制大小、拒绝未知字段与重复键、拒绝尾随数据。
func decodeWalletSnapshot(raw []byte) (*walletSnapshot, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("wallet config is empty")
	}
	if len(raw) > walletConfigMaxBytes {
		return nil, fmt.Errorf("wallet config exceeds %d bytes", walletConfigMaxBytes)
	}
	if err := checkDuplicateJSONKeys(raw); err != nil {
		return nil, fmt.Errorf("wallet config has duplicate keys: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot walletSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decode wallet config: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("wallet config has trailing data")
	}
	if snapshot.Profiles == nil {
		snapshot.Profiles = map[string]*walletProviderProfile{}
	}
	for env, profile := range snapshot.Profiles {
		if profile != nil {
			profile.Environment = env
		}
	}
	if err := validateWalletSnapshot(&snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// checkDuplicateJSONKeys 逐 token 扫描 JSON，拒绝对象内重复键。
func checkDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("unexpected trailing data")
	}
	return nil
}

// scanJSONValue 递归扫描一个 JSON 值。
func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected delimiter %q", delim)
	}
}

// validateWalletSnapshot 校验快照的枚举、币种、汇率与 Profile 完整性。
func validateWalletSnapshot(snapshot *walletSnapshot) error {
	if snapshot.Revision < 0 {
		return fmt.Errorf("revision must not be negative")
	}
	if snapshot.UpdatedAt < 0 {
		return fmt.Errorf("updated_at must not be negative")
	}
	if snapshot.WalletCurrency != WalletCurrencyCNY {
		return fmt.Errorf("wallet_currency must be %s", WalletCurrencyCNY)
	}
	if snapshot.ProviderCurrency != WalletCurrencyUSD {
		return fmt.Errorf("provider_currency must be %s", WalletCurrencyUSD)
	}

	rate, err := ParseWalletRate(snapshot.CNYPerUSDRate)
	if err != nil {
		return fmt.Errorf("cny_per_usd_rate: %w", err)
	}
	snapshot.rate = rate

	switch snapshot.RechargeMode {
	case RechargeModeDisabled, RechargeModeTest, RechargeModeProduction:
	default:
		return fmt.Errorf("recharge_mode must be one of DISABLED, TEST, PRODUCTION")
	}

	if snapshot.Enabled && snapshot.RechargeMode == RechargeModeDisabled {
		return fmt.Errorf("enabled wallet requires a recharge mode")
	}
	if snapshot.RedemptionEnabled {
		if snapshot.RedemptionRateLimit == 0 {
			return fmt.Errorf("redemption_rate_limit must be positive when redemption is enabled")
		}
		if snapshot.RedemptionRateWindowSeconds == 0 {
			return fmt.Errorf("redemption_rate_window_seconds must be positive when redemption is enabled")
		}
	}

	for env, profile := range snapshot.Profiles {
		if env != ProfileEnvironmentTest && env != ProfileEnvironmentProduction {
			return fmt.Errorf("unknown profile environment %q", env)
		}
		if profile == nil {
			return fmt.Errorf("profile %s is null", env)
		}
		profile.Environment = env
		if err := validateWalletProfile(profile); err != nil {
			return fmt.Errorf("profile %s: %w", env, err)
		}
	}

	// 启用生产/测试模式时，对应环境的 Profile 必须完整可用。
	if snapshot.Enabled && snapshot.RechargeMode != RechargeModeDisabled {
		profile, ok := snapshot.Profiles[snapshot.RechargeMode]
		if !ok || profile == nil {
			return fmt.Errorf("enabled recharge mode %s requires a complete profile", snapshot.RechargeMode)
		}
	}
	return nil
}

// validateWalletProfile 校验单个 Profile 的必填项、指纹与 Secret 数量。
func validateWalletProfile(profile *walletProviderProfile) error {
	if profile.ProfileID == "" {
		return fmt.Errorf("profile_id is required")
	}
	if profile.ProfileRevision < 0 {
		return fmt.Errorf("profile_revision must not be negative")
	}
	for name, value := range map[string]string{
		"api_key":    profile.APIKey,
		"product_id": profile.ProductID,
		"webhook_id": profile.WebhookID,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if len(profile.WebhookSecrets) == 0 {
		return fmt.Errorf("at least one webhook secret is required")
	}
	if len(profile.WebhookSecrets) > walletMaxWebhookSecrets {
		return fmt.Errorf("webhook secrets exceed %d", walletMaxWebhookSecrets)
	}

	seen := map[string]struct{}{}
	for _, secret := range profile.WebhookSecrets {
		if !isHexFingerprint(secret.Fingerprint) {
			return fmt.Errorf("webhook secret fingerprint must be a sha256 hex string")
		}
		if secret.Secret == "" {
			return fmt.Errorf("webhook secret must not be empty")
		}
		if _, exists := seen[secret.Fingerprint]; exists {
			return fmt.Errorf("duplicate webhook secret fingerprint")
		}
		seen[secret.Fingerprint] = struct{}{}
	}
	return nil
}

// isHexFingerprint 判断字符串是否是小写的 64 位 SHA-256 十六进制指纹。
func isHexFingerprint(value string) bool {
	if len(value) != walletFingerprintLen {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// walletSnapshotStale 判断快照是否超过 stale 阈值。
// 从未保存过（updated_at 为 0）视为 stale，促使调用方走只读或恢复动作。
func walletSnapshotStale(snapshot *walletSnapshot, now time.Time) bool {
	if snapshot == nil || snapshot.UpdatedAt <= 0 {
		return true
	}
	updated := time.UnixMilli(snapshot.UpdatedAt)
	if updated.After(now) {
		return false
	}
	return now.Sub(updated) > walletpolicy.SettingStaleThreshold
}

// walletRate 返回快照的规范化汇率；快照可能来自只设置了字符串的调用方，
// 此时按惰性方式解析一次，避免视图输出 0/0。
func (snapshot *walletSnapshot) walletRate() (WalletRate, error) {
	if snapshot.rate.Scaled > 0 {
		return snapshot.rate, nil
	}
	rate, err := ParseWalletRate(snapshot.CNYPerUSDRate)
	if err != nil {
		return WalletRate{}, fmt.Errorf("wallet config has invalid rate: %w", err)
	}
	snapshot.rate = rate
	return rate, nil
}

// maskedWalletSettings 构造管理视图：只含掩码、configured 标记与指纹。
func maskedWalletSettings(snapshot *walletSnapshot, now time.Time) (*setting.WalletSettingsMasked, error) {
	rate, err := snapshot.walletRate()
	if err != nil {
		return nil, err
	}
	sample, err := SampleProviderAmount(walletSampleWalletAmount, rate)
	if err != nil {
		return nil, err
	}

	masked := &setting.WalletSettingsMasked{
		Revision:                     snapshot.Revision,
		Enabled:                      snapshot.Enabled,
		RechargeMode:                 snapshot.RechargeMode,
		RedemptionEnabled:            snapshot.RedemptionEnabled,
		RedemptionRateLimit:          snapshot.RedemptionRateLimit,
		RedemptionRateWindowSeconds:  snapshot.RedemptionRateWindowSeconds,
		WalletCurrency:               snapshot.WalletCurrency,
		ProviderCurrency:             snapshot.ProviderCurrency,
		CnyPerUsdRate:                snapshot.CNYPerUSDRate,
		ExchangeSampleWalletAmount:   walletSampleWalletAmount,
		ExchangeSampleProviderAmount: sample,
		UpdatedBy:                    snapshot.UpdatedBy,
		UpdatedAt:                    snapshot.UpdatedAt,
		Stale:                        walletSnapshotStale(snapshot, now),
	}
	for env, profile := range snapshot.Profiles {
		if profile == nil {
			continue
		}
		view := maskedWalletProfile(profile)
		switch env {
		case ProfileEnvironmentTest:
			masked.TestProfile = view
		case ProfileEnvironmentProduction:
			masked.ProductionProfile = view
		}
	}
	return masked, nil
}

// maskedWalletProfile 构造单个 Profile 的掩码视图。
func maskedWalletProfile(profile *walletProviderProfile) *setting.WalletProviderProfileMasked {
	view := &setting.WalletProviderProfileMasked{
		Environment:       profile.Environment,
		ProfileId:         profile.ProfileID,
		ProfileRevision:   profile.ProfileRevision,
		ProductId:         profile.ProductID,
		WebhookId:         profile.WebhookID,
		ApiKeyMasked:      walletSecretMasked,
		ApiKeyConfigured:  profile.APIKey != "",
		ApiKeyFingerprint: FingerprintSecret(profile.APIKey),
		VerifiedAt:        profile.VerifiedAt,
	}
	for _, secret := range profile.WebhookSecrets {
		view.WebhookSecrets = append(view.WebhookSecrets, &setting.WalletWebhookSecretMasked{
			EndpointId:       secret.EndpointID,
			Fingerprint:      secret.Fingerprint,
			SecretMasked:     walletSecretMasked,
			SecretConfigured: secret.Secret != "",
			DisabledAt:       secret.DisabledAt,
			RetireAfter:      secret.RetireAfter,
		})
	}
	return view
}

// runtimeWalletSettings 构造运行时视图：含完整 Secret 与定点汇率分子/分母。
func runtimeWalletSettings(snapshot *walletSnapshot) *setting.WalletRuntimeSettings {
	// 运行时视图只由已通过校验的快照构造，汇率此时必然可解析。
	rate, err := snapshot.walletRate()
	if err != nil {
		return &setting.WalletRuntimeSettings{}
	}
	runtime := &setting.WalletRuntimeSettings{
		Revision:                    snapshot.Revision,
		Enabled:                     snapshot.Enabled,
		RechargeMode:                snapshot.RechargeMode,
		RedemptionEnabled:           snapshot.RedemptionEnabled,
		RedemptionRateLimit:         snapshot.RedemptionRateLimit,
		RedemptionRateWindowSeconds: snapshot.RedemptionRateWindowSeconds,
		WalletCurrency:              snapshot.WalletCurrency,
		ProviderCurrency:            snapshot.ProviderCurrency,
		CnyPerUsdRate:               snapshot.CNYPerUSDRate,
		CnyPerUsdNumerator:          rate.Numerator,
		CnyPerUsdDenominator:        rate.Denominator,
		UpdatedBy:                   snapshot.UpdatedBy,
		UpdatedAt:                   snapshot.UpdatedAt,
	}
	for env, profile := range snapshot.Profiles {
		if profile == nil {
			continue
		}
		view := &setting.WalletProviderProfileRuntime{
			Environment:     profile.Environment,
			ProfileId:       profile.ProfileID,
			ProfileRevision: profile.ProfileRevision,
			ApiKey:          profile.APIKey,
			ProductId:       profile.ProductID,
			WebhookId:       profile.WebhookID,
		}
		for _, secret := range profile.WebhookSecrets {
			view.WebhookSecrets = append(view.WebhookSecrets, &setting.WalletWebhookSecretRuntime{
				EndpointId:  secret.EndpointID,
				Fingerprint: secret.Fingerprint,
				Secret:      secret.Secret,
				DisabledAt:  secret.DisabledAt,
				RetireAfter: secret.RetireAfter,
			})
		}
		switch env {
		case ProfileEnvironmentTest:
			runtime.TestProfile = view
		case ProfileEnvironmentProduction:
			runtime.ProductionProfile = view
		}
	}
	return runtime
}

// WalletProfileValidationInput 是远程校验 Profile 所需的输入（不含完整历史数据）。
type WalletProfileValidationInput struct {
	Environment              string
	APIKey                   string
	ProductID                string
	WebhookID                string
	PreviousAPIKey           string
	PreviousProductID        string
	OtherWebhookFingerprints []string
}

// WalletProfileValidator 在 CAS 持久化前远程校验候选 Profile。
// 实现方负责调用 Provider 接口并返回安全错误；不得记录 Secret。
type WalletProfileValidator interface {
	ValidateWalletProfile(ctx context.Context, input WalletProfileValidationInput) error
}

// loadWalletSnapshot 读取并解码钱包配置；第二个返回值是原始 JSON，用于 CAS。
func (s SystemSettingService) loadWalletSnapshot(ctx context.Context) (*walletSnapshot, string, bool, error) {
	row, err := s.DBAll.Setting.Query().Where(settingEnt.Key(WalletConfigKey)).Only(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, "", false, nil
		}
		return nil, "", false, fmt.Errorf("load %s: %w", WalletConfigKey, err)
	}
	snapshot, err := decodeWalletSnapshot([]byte(row.Value))
	if err != nil {
		return nil, "", false, fmt.Errorf("decode %s: %w", WalletConfigKey, err)
	}
	return snapshot, row.Value, true, nil
}

// getWalletSettingsMasked 返回管理视图；配置缺失时返回关闭态默认值并标记 stale。
func (s SystemSettingService) getWalletSettingsMasked(ctx context.Context) (*setting.WalletSettingsMasked, error) {
	snapshot, _, found, err := s.loadWalletSnapshot(ctx)
	if err != nil {
		logger.Errorf("wallet settings masked load err: %v", err)
		return nil, walletSettingErr(WalletSettingCodeInternal, "wallet settings are unavailable")
	}
	if !found {
		return maskedWalletSettings(defaultWalletSnapshot(), time.Now())
	}
	return maskedWalletSettings(snapshot, time.Now())
}

// getWalletRuntimeSettings 返回含 Secret 的运行时视图，仅供内部 Commerce 调用。
func (s SystemSettingService) getWalletRuntimeSettings(ctx context.Context) (*setting.WalletRuntimeSettings, error) {
	snapshot, _, found, err := s.loadWalletSnapshot(ctx)
	if err != nil {
		logger.Errorf("wallet settings runtime load err: %v", err)
		return nil, walletSettingErr(WalletSettingCodeInternal, "wallet settings are unavailable")
	}
	if !found {
		return nil, walletSettingErr(WalletSettingCodeNotFound, "wallet settings are not configured")
	}
	return runtimeWalletSettings(snapshot), nil
}

// updateWalletSettings 应用补丁、远程校验 Profile，然后以 revision CAS 写回。
func (s SystemSettingService) updateWalletSettings(ctx context.Context, req *setting.UpdateWalletSettingsReq) (*setting.WalletSettingsMasked, error) {
	if req == nil {
		return nil, walletSettingErr(WalletSettingCodeInvalidArgument, "request is required")
	}
	if strings.TrimSpace(req.OperatorId) == "" {
		return nil, walletSettingErr(WalletSettingCodeInvalidArgument, "operator_id is required")
	}

	base, oldRaw, found, err := s.loadWalletSnapshot(ctx)
	if err != nil {
		logger.Errorf("wallet settings update load err: %v", err)
		return nil, walletSettingErr(WalletSettingCodeInternal, "wallet settings are unavailable")
	}
	if !found {
		base = defaultWalletSnapshot()
		if req.ExpectedRevision != 0 {
			return nil, walletSettingErr(WalletSettingCodeRevisionConflict, "wallet settings revision conflict")
		}
	}

	now := time.Now()
	candidate, changed, err := applyWalletSettingsPatch(base, req, now)
	if err != nil {
		return nil, err
	}

	if err := s.validateWalletProfiles(ctx, base, candidate); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(candidate)
	if err != nil {
		return nil, walletSettingErr(WalletSettingCodeInternal, "wallet settings could not be encoded")
	}
	if len(raw) > walletConfigMaxBytes {
		return nil, walletSettingErr(WalletSettingCodeInvalidArgument, "wallet settings are too large")
	}

	if found {
		affected, err := s.DBAll.Setting.Update().
			Where(settingEnt.Key(WalletConfigKey), settingEnt.ValueEQ(oldRaw)).
			SetValue(string(raw)).
			Save(ctx)
		if err != nil {
			logger.Errorf("wallet settings CAS update err: %v", err)
			return nil, walletSettingErr(WalletSettingCodeInternal, "wallet settings could not be saved")
		}
		if affected == 0 {
			return nil, walletSettingErr(WalletSettingCodeRevisionConflict, "wallet settings revision conflict")
		}
	} else {
		_, err := s.DBAll.Setting.Create().SetKey(WalletConfigKey).SetValue(string(raw)).Save(ctx)
		if err != nil {
			if isConstraintError(err) {
				return nil, walletSettingErr(WalletSettingCodeRevisionConflict, "wallet settings revision conflict")
			}
			logger.Errorf("wallet settings create err: %v", err)
			return nil, walletSettingErr(WalletSettingCodeInternal, "wallet settings could not be saved")
		}
	}

	walletSettingAudit(req.OperatorId, base.Revision, candidate.Revision, changed)
	return maskedWalletSettings(candidate, now)
}

// applyWalletSettingsPatch 把补丁合并到基线快照，返回新快照与变更字段列表。
func applyWalletSettingsPatch(base *walletSnapshot, req *setting.UpdateWalletSettingsReq, now time.Time) (*walletSnapshot, []string, error) {
	if base.Revision != req.ExpectedRevision {
		return nil, nil, walletSettingErr(WalletSettingCodeRevisionConflict, "wallet settings revision conflict")
	}

	candidate := cloneWalletSnapshot(base)
	var changed []string

	if req.Enabled != nil && candidate.Enabled != *req.Enabled {
		candidate.Enabled = *req.Enabled
		changed = append(changed, "enabled")
	}
	if req.RechargeMode != nil && candidate.RechargeMode != *req.RechargeMode {
		switch *req.RechargeMode {
		case RechargeModeDisabled, RechargeModeTest, RechargeModeProduction:
		default:
			return nil, nil, walletSettingErr(WalletSettingCodeInvalidArgument, "recharge_mode is invalid")
		}
		candidate.RechargeMode = *req.RechargeMode
		changed = append(changed, "recharge_mode")
	}
	if req.RedemptionEnabled != nil && candidate.RedemptionEnabled != *req.RedemptionEnabled {
		candidate.RedemptionEnabled = *req.RedemptionEnabled
		changed = append(changed, "redemption_enabled")
	}
	if req.RedemptionRateLimit != nil {
		candidate.RedemptionRateLimit = *req.RedemptionRateLimit
		changed = append(changed, "redemption_rate_limit")
	}
	if req.RedemptionRateWindowSeconds != nil {
		candidate.RedemptionRateWindowSeconds = *req.RedemptionRateWindowSeconds
		changed = append(changed, "redemption_rate_window_seconds")
	}
	if req.CnyPerUsdRate != nil {
		rate, err := ParseWalletRate(*req.CnyPerUsdRate)
		if err != nil {
			return nil, nil, walletSettingErr(WalletSettingCodeInvalidArgument, "cny_per_usd_rate is invalid")
		}
		if rate.Canonical != candidate.CNYPerUSDRate {
			candidate.CNYPerUSDRate = rate.Canonical
			changed = append(changed, "cny_per_usd_rate")
		}
	}

	profileFields, err := applyProfilePatches(candidate, req.Profiles, now)
	if err != nil {
		return nil, nil, err
	}
	changed = append(changed, profileFields...)

	candidate.Revision = base.Revision + 1
	candidate.UpdatedBy = req.OperatorId
	candidate.UpdatedAt = now.UnixMilli()

	if candidate.Profiles == nil {
		candidate.Profiles = map[string]*walletProviderProfile{}
	}
	if err := validateWalletSnapshot(candidate); err != nil {
		return nil, nil, walletSettingErr(WalletSettingCodeInvalidArgument, "wallet settings are invalid: "+err.Error())
	}
	sort.Strings(changed)
	return candidate, changed, nil
}

// applyProfilePatches 合并 Profile 补丁；空字符串不得清空已有秘密。
func applyProfilePatches(candidate *walletSnapshot, patches []*setting.WalletProviderProfilePatch, now time.Time) ([]string, error) {
	var fields []string
	for _, patch := range patches {
		if patch == nil {
			return nil, walletSettingErr(WalletSettingCodeInvalidArgument, "profile patch must not be null")
		}
		env := patch.Environment
		if env != ProfileEnvironmentTest && env != ProfileEnvironmentProduction {
			return nil, walletSettingErr(WalletSettingCodeInvalidArgument, "profile environment is invalid")
		}
		if patch.ProfileId != nil && *patch.ProfileId != "" {
			return nil, walletSettingErr(WalletSettingCodeInvalidArgument, "profile_id is assigned by the server")
		}

		profile := candidate.Profiles[env]
		created := false
		if profile == nil {
			profile = &walletProviderProfile{Environment: env, ProfileID: newWalletProfileID()}
			candidate.Profiles[env] = profile
			created = true
		}

		prefix := "profiles." + env
		if patch.ProductId != nil {
			if strings.TrimSpace(*patch.ProductId) == "" {
				return nil, walletSettingErr(WalletSettingCodeInvalidArgument, "product_id must not be empty")
			}
			if profile.ProductID != *patch.ProductId {
				profile.ProductID = *patch.ProductId
				fields = append(fields, prefix+".product_id")
			}
		}
		if patch.WebhookId != nil {
			if strings.TrimSpace(*patch.WebhookId) == "" {
				return nil, walletSettingErr(WalletSettingCodeInvalidArgument, "webhook_id must not be empty")
			}
			if profile.WebhookID != *patch.WebhookId {
				profile.WebhookID = *patch.WebhookId
				fields = append(fields, prefix+".webhook_id")
			}
		}
		if patch.ApiKey != nil {
			key := strings.TrimSpace(*patch.ApiKey)
			if key == "" {
				return nil, walletSettingErr(WalletSettingCodeInvalidArgument, "api_key must not be empty")
			}
			if key != profile.APIKey {
				profile.APIKey = key
				// 审计只记录 API Key 的 fingerprint，绝不记录明文。
				fields = append(fields, prefix+".api_key:"+FingerprintSecret(key))
			}
		}
		for _, secretPatch := range patch.WebhookSecrets {
			if secretPatch == nil || strings.TrimSpace(secretPatch.Secret) == "" {
				return nil, walletSettingErr(WalletSettingCodeInvalidArgument, "webhook secret must not be empty")
			}
			fingerprint := FingerprintSecret(strings.TrimSpace(secretPatch.Secret))
			if containsFingerprint(profile.WebhookSecrets, fingerprint) {
				continue
			}
			if len(profile.WebhookSecrets) >= walletMaxWebhookSecrets {
				return nil, walletSettingErr(WalletSettingCodeInvalidArgument, "too many webhook secrets")
			}
			profile.WebhookSecrets = append(profile.WebhookSecrets, walletWebhookSecret{
				Fingerprint: fingerprint,
				Secret:      strings.TrimSpace(secretPatch.Secret),
				EndpointID:  strings.TrimSpace(secretPatch.EndpointId),
				RetireAfter: now.Add(walletSecretDrainWindow).UnixMilli(),
			})
			// 审计只记录 webhook Secret 的 fingerprint，绝不记录明文。
			fields = append(fields, prefix+".webhook_secret:"+fingerprint)
		}
		if created {
			fields = append(fields, prefix)
		}
		// 设计 §7.2：每次成功的 Profile 更新都递增配置代际，即使提交值与旧值相同。
		// 只有补丁确实带了 Profile 字段才算一次更新；仅声明环境的空补丁不推进代际。
		requested := created ||
			patch.ProductId != nil ||
			patch.WebhookId != nil ||
			patch.ApiKey != nil ||
			len(patch.WebhookSecrets) > 0
		if requested {
			profile.ProfileRevision++
			profile.VerifiedAt = 0
		}
	}
	return fields, nil
}

// containsFingerprint 判断 Secret 列表里是否已有同一指纹。
func containsFingerprint(secrets []walletWebhookSecret, fingerprint string) bool {
	for _, secret := range secrets {
		if secret.Fingerprint == fingerprint {
			return true
		}
	}
	return false
}

// validateWalletProfiles 在写库前对全部 Profile 做远程校验（失败即拒绝，fail closed）。
func (s SystemSettingService) validateWalletProfiles(ctx context.Context, base, candidate *walletSnapshot) error {
	if len(candidate.Profiles) == 0 {
		return nil
	}
	if s.Validator == nil {
		return walletSettingErr(WalletSettingCodeUnavailable, "provider validation is not configured")
	}

	environments := make([]string, 0, len(candidate.Profiles))
	for env := range candidate.Profiles {
		environments = append(environments, env)
	}
	sort.Strings(environments)

	for _, env := range environments {
		profile := candidate.Profiles[env]
		if profile == nil {
			continue
		}
		previous := base.Profiles[env]
		input := WalletProfileValidationInput{
			Environment: env,
			APIKey:      profile.APIKey,
			ProductID:   profile.ProductID,
			WebhookID:   profile.WebhookID,
		}
		if previous != nil {
			input.PreviousAPIKey = previous.APIKey
			input.PreviousProductID = previous.ProductID
		}
		input.OtherWebhookFingerprints = otherWebhookFingerprints(candidate, env)

		if err := s.Validator.ValidateWalletProfile(ctx, input); err != nil {
			var bizErr *kerrors.BizStatusError
			if errors.As(err, &bizErr) {
				return err
			}
			logger.Errorf("wallet profile validation err env=%s: %v", env, err)
			return walletSettingErr(WalletSettingCodeProviderRejected, "provider profile validation failed")
		}
		profile.VerifiedAt = time.Now().UnixMilli()
	}
	return nil
}

// otherWebhookFingerprints 返回其它环境已使用的 Secret 指纹，用于跨 Profile 去重。
func otherWebhookFingerprints(snapshot *walletSnapshot, environment string) []string {
	var fingerprints []string
	for env, profile := range snapshot.Profiles {
		if env == environment || profile == nil {
			continue
		}
		for _, secret := range profile.WebhookSecrets {
			fingerprints = append(fingerprints, secret.Fingerprint)
		}
	}
	sort.Strings(fingerprints)
	return fingerprints
}

// newWalletProfileID 生成不可猜测的 Profile ID；Profile ID 由服务端分配且不可替换。
func newWalletProfileID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "wp_" + FingerprintSecret(fmt.Sprintf("%d", time.Now().UnixNano()))[:32]
	}
	return "wp_" + hex.EncodeToString(buf)
}

// walletSettingAudit 记录结构化安全审计：只含字段名、revision 与操作人，不含秘密。
func walletSettingAudit(operator string, oldRevision, newRevision int64, changedFields []string) {
	logger.WithFields(logrus.Fields{
		"audit":          "wallet_settings_update",
		"operator":       operator,
		"old_revision":   oldRevision,
		"new_revision":   newRevision,
		"changed_fields": changedFields,
	}).Info("wallet settings updated")
}

// 初始化所需的 conf 引用，保证本地构建不会因未使用导入失败。
func isNotFound(err error) bool { return ent.IsNotFound(err) }

func isConstraintError(err error) bool { return ent.IsConstraintError(err) }
