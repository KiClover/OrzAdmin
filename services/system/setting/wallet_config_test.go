package setting

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/cloudwego/kitex/pkg/kerrors"
)

// mustDecode 解码快照并在失败时终止测试。
func mustDecode(t *testing.T, raw []byte) *walletSnapshot {
	t.Helper()
	snapshot, err := decodeWalletSnapshot(raw)
	if err != nil {
		t.Fatalf("解码快照失败: %v", err)
	}
	return snapshot
}

// marshalSnapshot 序列化快照。
func marshalSnapshot(t *testing.T, snapshot *walletSnapshot) []byte {
	t.Helper()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("序列化快照失败: %v", err)
	}
	return raw
}

// completeProfile 构造一个可通过校验的完整 Profile。
func completeProfile(env string) *walletProviderProfile {
	return &walletProviderProfile{
		Environment:     env,
		ProfileID:       "wp_test_" + strings.ToLower(env),
		ProfileRevision: 1,
		APIKey:          "creem_api_key_123",
		ProductID:       "prod_123",
		WebhookID:       "wh_123",
		WebhookSecrets: []walletWebhookSecret{{
			Fingerprint: FingerprintSecret("whsec_abc"),
			Secret:      "whsec_abc",
			EndpointID:  "ep_1",
			RetireAfter: time.Now().Add(walletSecretDrainWindow).UnixMilli(),
		}},
	}
}

// bizCode 返回业务错误码；非业务错误返回 0。
func bizCode(err error) int32 {
	var bizErr *kerrors.BizStatusError
	if errors.As(err, &bizErr) {
		return bizErr.BizStatusCode()
	}
	return 0
}

func TestParseWalletRate(t *testing.T) {
	valid := []struct {
		input     string
		canonical string
		numerator int64
		denom     int64
		scaled    int64
	}{
		{input: "7.2", canonical: "7.200000", numerator: 36, denom: 5, scaled: 7_200_000},
		{input: "7.200000", canonical: "7.200000", numerator: 36, denom: 5, scaled: 7_200_000},
		{input: "0.1", canonical: "0.100000", numerator: 1, denom: 10, scaled: 100_000},
		{input: "100", canonical: "100.000000", numerator: 100, denom: 1, scaled: 100_000_000},
		{input: "0.123456", canonical: "0.123456", numerator: 1929, denom: 15_625, scaled: 123_456},
	}
	for _, tc := range valid {
		rate, err := ParseWalletRate(tc.input)
		if err != nil {
			t.Fatalf("ParseWalletRate(%q) 返回错误: %v", tc.input, err)
		}
		if rate.Canonical != tc.canonical {
			t.Fatalf("ParseWalletRate(%q).Canonical = %q，期望 %q", tc.input, rate.Canonical, tc.canonical)
		}
		if rate.Numerator != tc.numerator || rate.Denominator != tc.denom || rate.Scaled != tc.scaled {
			t.Fatalf("ParseWalletRate(%q) = %+v，期望分子/分母 %d/%d", tc.input, rate, tc.numerator, tc.denom)
		}
	}

	invalid := []string{
		"", " ", " 7.2", "7.2 ", "+7.2", "-7.2", "7.", ".5", "7.1234567",
		"0.099999", "100.000001", "07.2", "1e3", "7,2", "abc", "1.2.3",
	}
	for _, input := range invalid {
		if _, err := ParseWalletRate(input); err == nil {
			t.Fatalf("ParseWalletRate(%q) 应返回错误", input)
		}
	}
}

func TestSampleProviderAmount(t *testing.T) {
	rate, err := ParseWalletRate("7.2")
	if err != nil {
		t.Fatalf("解析汇率失败: %v", err)
	}

	// 100 元（10000 分）按 7.2 换算 = 13.8888 美元 → 1389 美分（向上取整）。
	got, err := SampleProviderAmount(10_000, rate)
	if err != nil {
		t.Fatalf("换算失败: %v", err)
	}
	if got != 1389 {
		t.Fatalf("换算结果 = %d，期望 1389", got)
	}

	// 1 分至少产生 1 美分（向上取整）。
	if got, err := SampleProviderAmount(1, rate); err != nil || got != 1 {
		t.Fatalf("1 分换算 = %d err=%v，期望 1", got, err)
	}

	for _, amount := range []int64{0, -1} {
		if _, err := SampleProviderAmount(amount, rate); err == nil {
			t.Fatalf("金额 %d 应被拒绝", amount)
		}
	}
}

func TestDefaultSnapshotMatchesInitializationSeed(t *testing.T) {
	// 初始化种子与服务内默认值必须是同一份语义，避免空库初始化后
	// 管理 GET 与数据库内容不一致。
	var fromSeed, fromService map[string]any
	if err := json.Unmarshal(WalletDefaultConfigJSON(), &fromSeed); err != nil {
		t.Fatalf("解析初始化种子失败: %v", err)
	}
	if err := json.Unmarshal(marshalSnapshot(t, defaultWalletSnapshot()), &fromService); err != nil {
		t.Fatalf("解析服务默认值失败: %v", err)
	}
	if !reflect.DeepEqual(fromSeed, fromService) {
		t.Fatalf("默认快照不一致:\n种子 = %v\n服务 = %v", fromSeed, fromService)
	}
}

func TestDecodeWalletSnapshotRejectsBrokenConfig(t *testing.T) {
	base := marshalSnapshot(t, defaultWalletSnapshot())

	// 未知字段
	var withUnknown map[string]any
	if err := json.Unmarshal(base, &withUnknown); err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}
	withUnknown["top_up_enabled"] = true
	unknownRaw, _ := json.Marshal(withUnknown)
	if _, err := decodeWalletSnapshot(unknownRaw); err == nil {
		t.Fatalf("未知字段应被拒绝")
	}

	// 重复键
	duplicated := []byte(`{"revision":0,"revision":1}`)
	if _, err := decodeWalletSnapshot(duplicated); err == nil {
		t.Fatalf("重复键应被拒绝")
	}

	// 尾随数据
	if _, err := decodeWalletSnapshot(append(base, []byte(` {}`)...)); err == nil {
		t.Fatalf("尾随数据应被拒绝")
	}

	// 超大配置
	tooLarge := []byte(`{"revision":0,"updated_by":"` + strings.Repeat("x", walletConfigMaxBytes) + `"}`)
	if _, err := decodeWalletSnapshot(tooLarge); err == nil {
		t.Fatalf("超限配置应被拒绝")
	}

	// 非法汇率
	badRate := cloneWalletSnapshot(defaultWalletSnapshot())
	badRate.CNYPerUSDRate = "0.01"
	if _, err := decodeWalletSnapshot(marshalSnapshot(t, badRate)); err == nil {
		t.Fatalf("非法汇率应被拒绝")
	}

	// 币种错误
	badCurrency := cloneWalletSnapshot(defaultWalletSnapshot())
	badCurrency.WalletCurrency = "USD"
	if _, err := decodeWalletSnapshot(marshalSnapshot(t, badCurrency)); err == nil {
		t.Fatalf("错误币种应被拒绝")
	}

	// 启用但模式为 DISABLED
	enabledDisabled := cloneWalletSnapshot(defaultWalletSnapshot())
	enabledDisabled.Enabled = true
	if _, err := decodeWalletSnapshot(marshalSnapshot(t, enabledDisabled)); err == nil {
		t.Fatalf("启用但模式为 DISABLED 应被拒绝")
	}

	// 启用生产模式但缺少 Profile
	enabledProduction := cloneWalletSnapshot(defaultWalletSnapshot())
	enabledProduction.Enabled = true
	enabledProduction.RechargeMode = RechargeModeProduction
	if _, err := decodeWalletSnapshot(marshalSnapshot(t, enabledProduction)); err == nil {
		t.Fatalf("缺少 Profile 的生产模式应被拒绝")
	}

	if _, err := decodeWalletSnapshot(nil); err == nil {
		t.Fatalf("空配置应被拒绝")
	}
}

func TestDecodeWalletSnapshotAcceptsCompleteProfile(t *testing.T) {
	snapshot := cloneWalletSnapshot(defaultWalletSnapshot())
	snapshot.Revision = 3
	snapshot.Enabled = true
	snapshot.RechargeMode = RechargeModeTest
	snapshot.Profiles[ProfileEnvironmentTest] = completeProfile(ProfileEnvironmentTest)

	decoded := mustDecode(t, marshalSnapshot(t, snapshot))
	if decoded.rate.Canonical != "7.200000" {
		t.Fatalf("汇率未规范化: %q", decoded.rate.Canonical)
	}
	if got := decoded.Profiles[ProfileEnvironmentTest]; got == nil || got.Environment != ProfileEnvironmentTest {
		t.Fatalf("Profile 环境未回填: %+v", got)
	}
}

func TestMaskedSettingsNeverExposeSecrets(t *testing.T) {
	snapshot := cloneWalletSnapshot(defaultWalletSnapshot())
	snapshot.Revision = 5
	snapshot.Enabled = true
	snapshot.RechargeMode = RechargeModeTest
	snapshot.UpdatedAt = time.Now().UnixMilli()
	snapshot.Profiles[ProfileEnvironmentTest] = completeProfile(ProfileEnvironmentTest)

	masked, err := maskedWalletSettings(snapshot, time.Now())
	if err != nil {
		t.Fatalf("构造掩码视图失败: %v", err)
	}

	raw, err := json.Marshal(masked)
	if err != nil {
		t.Fatalf("序列化掩码视图失败: %v", err)
	}
	for _, secret := range []string{"creem_api_key_123", "whsec_abc"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("掩码视图泄漏秘密 %q: %s", secret, raw)
		}
	}
	if strings.Contains(string(raw), `"secret"`) {
		t.Fatalf("掩码视图不应包含 secret 字段: %s", raw)
	}

	if masked.GetTestProfile() == nil {
		t.Fatalf("缺少 test profile")
	}
	testProfile := masked.GetTestProfile()
	if !testProfile.GetApiKeyConfigured() {
		t.Fatalf("api key configured 应为 true")
	}
	if testProfile.GetApiKeyMasked() != walletSecretMasked {
		t.Fatalf("api key 掩码 = %q", testProfile.GetApiKeyMasked())
	}
	if testProfile.GetApiKeyFingerprint() != FingerprintSecret("creem_api_key_123") {
		t.Fatalf("api key 指纹不正确")
	}
	if len(testProfile.GetWebhookSecrets()) != 1 {
		t.Fatalf("webhook secret 数量 = %d", len(testProfile.GetWebhookSecrets()))
	}
	if !testProfile.GetWebhookSecrets()[0].GetSecretConfigured() {
		t.Fatalf("webhook secret configured 应为 true")
	}
	if masked.GetExchangeSampleWalletAmount() != walletSampleWalletAmount {
		t.Fatalf("示例金额 = %d", masked.GetExchangeSampleWalletAmount())
	}
	if masked.GetExchangeSampleProviderAmount() != 1389 {
		t.Fatalf("示例换算 = %d，期望 1389", masked.GetExchangeSampleProviderAmount())
	}
	if masked.GetStale() {
		t.Fatalf("刚保存的配置不应是 stale")
	}
}

func TestRuntimeSettingsCarrySecretsAndFixedPointRate(t *testing.T) {
	snapshot := cloneWalletSnapshot(defaultWalletSnapshot())
	snapshot.Profiles[ProfileEnvironmentProduction] = completeProfile(ProfileEnvironmentProduction)

	runtime := runtimeWalletSettings(snapshot)
	if runtime.GetCnyPerUsdNumerator() != 36 || runtime.GetCnyPerUsdDenominator() != 5 {
		t.Fatalf("汇率分子/分母 = %d/%d", runtime.GetCnyPerUsdNumerator(), runtime.GetCnyPerUsdDenominator())
	}
	profile := runtime.GetProductionProfile()
	if profile == nil {
		t.Fatalf("缺少 production profile")
	}
	if profile.GetApiKey() != "creem_api_key_123" {
		t.Fatalf("运行时视图应含明文 api key")
	}
	if len(profile.GetWebhookSecrets()) != 1 || profile.GetWebhookSecrets()[0].GetSecret() != "whsec_abc" {
		t.Fatalf("运行时视图应含明文 webhook secret")
	}
}

func TestApplyWalletSettingsPatchKeepsOmittedFields(t *testing.T) {
	base := defaultWalletSnapshot()
	req := &setting.UpdateWalletSettingsReq{
		ExpectedRevision: 0,
		OperatorId:       "1",
	}
	candidate, changed, err := applyWalletSettingsPatch(base, req, time.Now())
	if err != nil {
		t.Fatalf("空补丁应成功: %v", err)
	}
	if len(changed) != 0 {
		t.Fatalf("空补丁不应有变更字段: %v", changed)
	}
	if candidate.Revision != base.Revision+1 {
		t.Fatalf("revision = %d，期望 %d", candidate.Revision, base.Revision+1)
	}
	if candidate.CNYPerUSDRate != base.CNYPerUSDRate || candidate.RechargeMode != base.RechargeMode {
		t.Fatalf("未提交的字段不应变化")
	}
	if candidate.UpdatedBy != "1" {
		t.Fatalf("updated_by = %q", candidate.UpdatedBy)
	}
}

func TestApplyWalletSettingsPatchRevisionConflict(t *testing.T) {
	base := defaultWalletSnapshot()
	base.Revision = 2
	req := &setting.UpdateWalletSettingsReq{ExpectedRevision: 1, OperatorId: "1"}

	_, _, err := applyWalletSettingsPatch(base, req, time.Now())
	if got := bizCode(err); got != WalletSettingCodeRevisionConflict {
		t.Fatalf("错误码 = %d，期望 %d（err=%v）", got, WalletSettingCodeRevisionConflict, err)
	}
}

func TestApplyWalletSettingsPatchRejectsInvalidInput(t *testing.T) {
	base := defaultWalletSnapshot()

	enabled := true
	mode := "NOPE"
	badMode := &setting.UpdateWalletSettingsReq{ExpectedRevision: 0, OperatorId: "1", Enabled: &enabled, RechargeMode: &mode}
	if _, _, err := applyWalletSettingsPatch(base, badMode, time.Now()); bizCode(err) != WalletSettingCodeInvalidArgument {
		t.Fatalf("非法模式应返回参数错误，实际 %v", err)
	}

	badRate := "0.01"
	rateReq := &setting.UpdateWalletSettingsReq{ExpectedRevision: 0, OperatorId: "1", CnyPerUsdRate: &badRate}
	if _, _, err := applyWalletSettingsPatch(base, rateReq, time.Now()); bizCode(err) != WalletSettingCodeInvalidArgument {
		t.Fatalf("非法汇率应返回参数错误，实际 %v", err)
	}

	// 启用模式但没有 Profile。
	mode = RechargeModeTest
	enableReq := &setting.UpdateWalletSettingsReq{ExpectedRevision: 0, OperatorId: "1", Enabled: &enabled, RechargeMode: &mode}
	if _, _, err := applyWalletSettingsPatch(base, enableReq, time.Now()); bizCode(err) != WalletSettingCodeInvalidArgument {
		t.Fatalf("启用但缺少 Profile 应返回参数错误，实际 %v", err)
	}
}

func TestApplyWalletSettingsPatchNormalizesRate(t *testing.T) {
	base := defaultWalletSnapshot()
	rate := "7.2"
	req := &setting.UpdateWalletSettingsReq{ExpectedRevision: 0, OperatorId: "1", CnyPerUsdRate: &rate}

	candidate, changed, err := applyWalletSettingsPatch(base, req, time.Now())
	if err != nil {
		t.Fatalf("更新汇率失败: %v", err)
	}
	if candidate.CNYPerUSDRate != "7.200000" {
		t.Fatalf("汇率未规范化: %q", candidate.CNYPerUSDRate)
	}
	if len(changed) != 0 {
		t.Fatalf("值未变化时不应计入变更字段: %v", changed)
	}

	rate = "7.30"
	candidate, changed, err = applyWalletSettingsPatch(base, &setting.UpdateWalletSettingsReq{
		ExpectedRevision: 0, OperatorId: "1", CnyPerUsdRate: &rate,
	}, time.Now())
	if err != nil {
		t.Fatalf("更新汇率失败: %v", err)
	}
	if candidate.CNYPerUSDRate != "7.300000" || !reflect.DeepEqual(changed, []string{"cny_per_usd_rate"}) {
		t.Fatalf("汇率更新结果 = %q 变更 = %v", candidate.CNYPerUSDRate, changed)
	}
}

func TestApplyProfilePatchesCreatesAndMerges(t *testing.T) {
	base := defaultWalletSnapshot()
	apiKey := "creem_api_key_123"
	productID := "prod_123"
	webhookID := "wh_123"
	secret := "whsec_abc"
	patch := &setting.WalletProviderProfilePatch{
		Environment: ProfileEnvironmentTest,
		ApiKey:      &apiKey,
		ProductId:   &productID,
		WebhookId:   &webhookID,
		WebhookSecrets: []*setting.WalletWebhookSecretPatch{
			{Secret: secret, EndpointId: "ep_1"},
		},
	}
	req := &setting.UpdateWalletSettingsReq{
		ExpectedRevision: 0,
		OperatorId:       "1",
		Profiles:         []*setting.WalletProviderProfilePatch{patch},
	}

	candidate, changed, err := applyWalletSettingsPatch(base, req, time.Now())
	if err != nil {
		t.Fatalf("创建 Profile 失败: %v", err)
	}
	// 审计只记录字段名与 fingerprint，不记录任何明文 Secret。
	wantChanged := []string{
		"profiles.TEST",
		"profiles.TEST.api_key:" + FingerprintSecret(apiKey),
		"profiles.TEST.product_id",
		"profiles.TEST.webhook_id",
		"profiles.TEST.webhook_secret:" + FingerprintSecret(secret),
	}
	sort.Strings(wantChanged)
	if !reflect.DeepEqual(changed, wantChanged) {
		t.Fatalf("变更字段 = %v，期望 %v", changed, wantChanged)
	}
	for _, field := range changed {
		if strings.Contains(field, apiKey) || strings.Contains(field, secret) {
			t.Fatalf("审计字段泄漏明文秘密: %q", field)
		}
	}
	profile := candidate.Profiles[ProfileEnvironmentTest]
	if profile == nil {
		t.Fatalf("Profile 未创建")
	}
	if !strings.HasPrefix(profile.ProfileID, "wp_") || len(profile.ProfileID) != len("wp_")+32 {
		t.Fatalf("Profile ID 不是服务端生成的随机值: %q", profile.ProfileID)
	}
	if profile.ProfileRevision != 1 {
		t.Fatalf("Profile 版本 = %d，期望 1", profile.ProfileRevision)
	}
	if len(profile.WebhookSecrets) != 1 || profile.WebhookSecrets[0].Fingerprint != FingerprintSecret(secret) {
		t.Fatalf("webhook secret 未按指纹保存: %+v", profile.WebhookSecrets)
	}
	if profile.WebhookSecrets[0].RetireAfter <= time.Now().UnixMilli() {
		t.Fatalf("webhook secret 应带轮换保留期")
	}

	// 重复提交同一 Secret 不应重复追加，但其它字段更新会让 Profile 版本递增。
	productID2 := "prod_456"
	second := &setting.UpdateWalletSettingsReq{
		ExpectedRevision: candidate.Revision,
		OperatorId:       "1",
		Profiles: []*setting.WalletProviderProfilePatch{{
			Environment: ProfileEnvironmentTest,
			ProductId:   &productID2,
			WebhookSecrets: []*setting.WalletWebhookSecretPatch{
				{Secret: secret},
			},
		}},
	}
	updated, _, err := applyWalletSettingsPatch(candidate, second, time.Now())
	if err != nil {
		t.Fatalf("更新 Profile 失败: %v", err)
	}
	profile = updated.Profiles[ProfileEnvironmentTest]
	if len(profile.WebhookSecrets) != 1 {
		t.Fatalf("重复 Secret 不应追加: %d", len(profile.WebhookSecrets))
	}
	if profile.ProductID != "prod_456" || profile.ProfileRevision != 2 {
		t.Fatalf("Profile 更新结果 = %+v", profile)
	}
}

func TestApplyProfilePatchesRejectsClientAssignedIdentity(t *testing.T) {
	base := defaultWalletSnapshot()
	profileID := "wp_client_choice"
	req := &setting.UpdateWalletSettingsReq{
		ExpectedRevision: 0,
		OperatorId:       "1",
		Profiles: []*setting.WalletProviderProfilePatch{{
			Environment: ProfileEnvironmentTest,
			ProfileId:   &profileID,
		}},
	}
	if _, _, err := applyWalletSettingsPatch(base, req, time.Now()); bizCode(err) != WalletSettingCodeInvalidArgument {
		t.Fatalf("客户端指定 profile_id 应被拒绝，实际 %v", err)
	}
}

func TestApplyProfilePatchesRejectsEmptySecrets(t *testing.T) {
	base := defaultWalletSnapshot()
	empty := ""
	base.Profiles[ProfileEnvironmentTest] = completeProfile(ProfileEnvironmentTest)
	req := &setting.UpdateWalletSettingsReq{
		ExpectedRevision: 0,
		OperatorId:       "1",
		Profiles: []*setting.WalletProviderProfilePatch{{
			Environment: ProfileEnvironmentTest,
			ApiKey:      &empty,
		}},
	}
	if _, _, err := applyWalletSettingsPatch(base, req, time.Now()); bizCode(err) != WalletSettingCodeInvalidArgument {
		t.Fatalf("空 api_key 应被拒绝，实际 %v", err)
	}
}

func TestValidateWalletProfileRequiresCompleteProfile(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*walletProviderProfile)
	}{
		{"empty", func(p *walletProviderProfile) { *p = walletProviderProfile{} }},
		{"no_api_key", func(p *walletProviderProfile) { p.APIKey = "" }},
		{"no_product", func(p *walletProviderProfile) { p.ProductID = "" }},
		{"no_webhook", func(p *walletProviderProfile) { p.WebhookID = "" }},
		{"no_secret", func(p *walletProviderProfile) { p.WebhookSecrets = nil }},
		{"bad_fingerprint", func(p *walletProviderProfile) { p.WebhookSecrets[0].Fingerprint = "zzz" }},
		{"empty_secret", func(p *walletProviderProfile) { p.WebhookSecrets[0].Secret = "" }},
		{"negative_revision", func(p *walletProviderProfile) { p.ProfileRevision = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile := completeProfile(ProfileEnvironmentTest)
			tc.mutate(profile)
			if err := validateWalletProfile(profile); err == nil {
				t.Fatalf("期望校验失败")
			}
		})
	}

	duplicated := completeProfile(ProfileEnvironmentTest)
	duplicated.WebhookSecrets = append(duplicated.WebhookSecrets, duplicated.WebhookSecrets[0])
	if err := validateWalletProfile(duplicated); err == nil {
		t.Fatalf("重复指纹应被拒绝")
	}
}

func TestWalletSnapshotStale(t *testing.T) {
	now := time.Now()

	empty := defaultWalletSnapshot()
	if !walletSnapshotStale(empty, now) {
		t.Fatalf("从未保存的配置应视为 stale")
	}

	fresh := defaultWalletSnapshot()
	fresh.UpdatedAt = now.UnixMilli()
	if walletSnapshotStale(fresh, now) {
		t.Fatalf("刚保存的配置不应 stale")
	}

	old := defaultWalletSnapshot()
	old.UpdatedAt = now.Add(-2 * time.Hour).UnixMilli()
	if !walletSnapshotStale(old, now) {
		t.Fatalf("超过 stale 阈值的配置应 stale")
	}
}

func TestFingerprintSecretIsStable(t *testing.T) {
	first := FingerprintSecret("whsec_abc")
	second := FingerprintSecret("whsec_abc")
	if first != second {
		t.Fatalf("同一秘密的指纹应稳定")
	}
	if len(first) != walletFingerprintLen {
		t.Fatalf("指纹长度 = %d，期望 %d", len(first), walletFingerprintLen)
	}
	if FingerprintSecret("whsec_abc") == FingerprintSecret("whsec_abd") {
		t.Fatalf("不同秘密的指纹不应相同")
	}
	if strings.Contains(first, "whsec") {
		t.Fatalf("指纹不应包含明文")
	}
}
