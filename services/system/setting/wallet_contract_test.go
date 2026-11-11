package setting

import (
	"reflect"
	"testing"

	"github.com/KiClover/orzAdmin/kitex_gen/system/setting"
)

// 这组测试冻结 doc/plan/WalletDesign.MD §10.3：钱包设置有两套严格分离的契约。
//   1. 管理契约（GetWalletSettingsMasked / UpdateWalletSettings）只返回掩码与指纹，禁止明文 Secret。
//   2. 内部契约（GetWalletRuntimeSettings）返回完整 Secret，仅供 Commerce 内网调用。
// 任何任务把 Secret 混进管理 DTO，或让运行时 DTO 复用管理 DTO，都必须先改设计与本测试。

func TestWalletSettingsRPCsExist(t *testing.T) {
	iface := reflect.TypeOf((*setting.SystemSettingService)(nil)).Elem()

	want := []string{
		"GetWalletSettingsMasked",
		"UpdateWalletSettings",
		"GetWalletRuntimeSettings",
	}
	for _, name := range want {
		method, ok := iface.MethodByName(name)
		if !ok {
			t.Fatalf("SystemSettingService 缺少钱包设置 RPC: %s", name)
		}
		switch name {
		case "GetWalletSettingsMasked":
			if got := method.Type.Out(0); got != reflect.TypeOf((*setting.WalletSettingsMasked)(nil)) {
				t.Fatalf("%s 返回类型必须是 *WalletSettingsMasked，实际 %s", name, got)
			}
		case "UpdateWalletSettings":
			if got := method.Type.In(1); got != reflect.TypeOf((*setting.UpdateWalletSettingsReq)(nil)) {
				t.Fatalf("%s 请求类型必须是 *UpdateWalletSettingsReq，实际 %s", name, got)
			}
			if got := method.Type.Out(0); got != reflect.TypeOf((*setting.WalletSettingsMasked)(nil)) {
				t.Fatalf("%s 返回类型必须是 *WalletSettingsMasked，实际 %s", name, got)
			}
		case "GetWalletRuntimeSettings":
			if got := method.Type.Out(0); got != reflect.TypeOf((*setting.WalletRuntimeSettings)(nil)) {
				t.Fatalf("%s 返回类型必须是 *WalletRuntimeSettings，实际 %s", name, got)
			}
		}
	}
}

// TestMaskedSettingsNeverExposePlaintextSecrets 检查管理契约的三种视图都不含明文字段。
func TestMaskedSettingsNeverExposePlaintextSecrets(t *testing.T) {
	forbidden := []string{"ApiKey", "Secret"}

	types := []reflect.Type{
		reflect.TypeOf(setting.WalletSettingsMasked{}),
		reflect.TypeOf(setting.WalletProviderProfileMasked{}),
		reflect.TypeOf(setting.WalletWebhookSecretMasked{}),
	}
	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			name := typ.Field(i).Name
			for _, bad := range forbidden {
				if name == bad {
					t.Fatalf("%s 不得包含明文字段 %s", typ.Name(), name)
				}
			}
		}
	}

	// 掩码视图必须提供配置状态、固定掩码与指纹，供管理员判断“是否已配置”。
	profile := reflect.TypeOf(setting.WalletProviderProfileMasked{})
	for _, name := range []string{"ApiKeyMasked", "ApiKeyConfigured", "ApiKeyFingerprint", "WebhookSecrets"} {
		if _, ok := profile.FieldByName(name); !ok {
			t.Fatalf("WalletProviderProfileMasked 缺少字段 %s", name)
		}
	}

	secret := reflect.TypeOf(setting.WalletWebhookSecretMasked{})
	for _, name := range []string{"Fingerprint", "SecretMasked", "SecretConfigured"} {
		if _, ok := secret.FieldByName(name); !ok {
			t.Fatalf("WalletWebhookSecretMasked 缺少字段 %s", name)
		}
	}
}

// TestRuntimeSettingsCarrySecrets 检查内部契约保留完整 Secret，并带规范化后的整数汇率。
func TestRuntimeSettingsCarrySecrets(t *testing.T) {
	profile := reflect.TypeOf(setting.WalletProviderProfileRuntime{})
	field, ok := profile.FieldByName("ApiKey")
	if !ok || field.Type != reflect.TypeOf("") {
		t.Fatalf("WalletProviderProfileRuntime.ApiKey 必须是明文字符串")
	}

	secret := reflect.TypeOf(setting.WalletWebhookSecretRuntime{})
	field, ok = secret.FieldByName("Secret")
	if !ok || field.Type != reflect.TypeOf("") {
		t.Fatalf("WalletWebhookSecretRuntime.Secret 必须是明文字符串")
	}

	runtime := reflect.TypeOf(setting.WalletRuntimeSettings{})
	for _, name := range []string{"CnyPerUsdNumerator", "CnyPerUsdDenominator"} {
		field, ok := runtime.FieldByName(name)
		if !ok {
			t.Fatalf("WalletRuntimeSettings 缺少字段 %s", name)
		}
		if field.Type != reflect.TypeOf(int64(0)) {
			t.Fatalf("WalletRuntimeSettings.%s 必须是 int64 定点分子/分母", name)
		}
	}

	// 运行时视图用于换算，必须同时给出十进制汇率字符串。
	rate, ok := runtime.FieldByName("CnyPerUsdRate")
	if !ok || rate.Type != reflect.TypeOf("") {
		t.Fatalf("WalletRuntimeSettings.CnyPerUsdRate 必须是十进制字符串")
	}
}

// TestUpdateWalletSettingsRequiresRevision 冻结乐观并发控制与“只写 Secret 补丁”。
func TestUpdateWalletSettingsRequiresRevision(t *testing.T) {
	req := reflect.TypeOf(setting.UpdateWalletSettingsReq{})

	revision, ok := req.FieldByName("ExpectedRevision")
	if !ok || revision.Type != reflect.TypeOf(int64(0)) {
		t.Fatalf("UpdateWalletSettingsReq.ExpectedRevision 必须是 int64")
	}

	// 操作人来自认证上下文，由 Setting HTTP 注入，不属于“可被省略”的补丁字段。
	operator, ok := req.FieldByName("OperatorId")
	if !ok || operator.Type != reflect.TypeOf("") {
		t.Fatalf("UpdateWalletSettingsReq.OperatorId 必须是字符串")
	}

	// 只写 Secret 补丁：补丁结构存在，但不得包含回显用的明文字段读取路径。
	patch := reflect.TypeOf(setting.WalletProviderProfilePatch{})
	if _, ok := patch.FieldByName("ApiKey"); !ok {
		t.Fatalf("WalletProviderProfilePatch 必须提供只写 ApiKey 字段")
	}

	// profile_revision / verified_at 由服务端维护，管理员不能写入。
	for _, name := range []string{"ProfileRevision", "VerifiedAt"} {
		if _, ok := patch.FieldByName(name); ok {
			t.Fatalf("WalletProviderProfilePatch 不得接受服务端维护字段 %s", name)
		}
	}
}

// TestMaskedAndRuntimeSettingsAreDistinct 防回归：两套契约不能共用同一个 Go 类型。
func TestMaskedAndRuntimeSettingsAreDistinct(t *testing.T) {
	masked := reflect.TypeOf(setting.WalletSettingsMasked{})
	runtime := reflect.TypeOf(setting.WalletRuntimeSettings{})

	if masked == runtime {
		t.Fatalf("管理视图与运行时视图必须是不同类型")
	}
	if masked.Name() == runtime.Name() {
		t.Fatalf("管理视图与运行时视图不能同名")
	}

	maskedProfile := reflect.TypeOf(setting.WalletProviderProfileMasked{})
	runtimeProfile := reflect.TypeOf(setting.WalletProviderProfileRuntime{})
	if maskedProfile == runtimeProfile {
		t.Fatalf("Profile 管理视图与运行时视图必须是不同类型")
	}
}
