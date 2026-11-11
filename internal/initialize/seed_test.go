package initialize

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAPISeedsCoverWalletSettingRoutes 确认钱包设置路由已经进入种子，
// 且每条种子都带有分组与描述，便于后台按组展示。
func TestAPISeedsCoverWalletSettingRoutes(t *testing.T) {
	hasWalletGet, hasWalletPut := false, false
	seen := map[[2]string]bool{}
	for _, seed := range APISeeds() {
		key := [2]string{seed.Path, seed.Method}
		if seen[key] {
			t.Fatalf("API 种子重复: %v", key)
		}
		seen[key] = true
		if seed.Path == "" || seed.Method == "" || seed.Group == "" || seed.Description == "" {
			t.Fatalf("API 种子字段不完整: %#v", seed)
		}
		switch {
		case seed.Path == "/setting/wallet" && seed.Method == "GET":
			hasWalletGet = true
		case seed.Path == "/setting/wallet" && seed.Method == "PUT":
			hasWalletPut = true
		}
	}
	if !hasWalletGet || !hasWalletPut {
		t.Fatalf("缺少 /setting/wallet 的 GET/PUT 种子: get=%v put=%v", hasWalletGet, hasWalletPut)
	}
}

// TestRoleSeedsContainAdminAndUser 确认两个内置角色仍在种子里。
func TestRoleSeedsContainAdminAndUser(t *testing.T) {
	codes := map[string]bool{}
	for _, role := range RoleSeeds() {
		if role.Code == "" || role.Name == "" {
			t.Fatalf("角色种子字段不完整: %#v", role)
		}
		codes[role.Code] = true
	}
	for _, want := range []string{"admin", "user"} {
		if !codes[want] {
			t.Fatalf("缺少角色种子 %q", want)
		}
	}
}

// TestSettingSeedsIncludeWalletConfig 确认 wallet_config 是默认设置的一部分，
// 且键唯一；种子只描述"缺失时补什么"，不负责覆盖已有值。
func TestSettingSeedsIncludeWalletConfig(t *testing.T) {
	seen := map[string]bool{}
	found := false
	for _, seed := range SettingSeeds() {
		if seed.Key == "" {
			t.Fatalf("设置种子键为空: %#v", seed)
		}
		if seen[seed.Key] {
			t.Fatalf("设置键重复: %q", seed.Key)
		}
		seen[seed.Key] = true
		if seed.Key == "wallet_config" {
			found = true
			if !strings.HasPrefix(seed.Value, "{") {
				t.Fatalf("wallet_config 种子应是 JSON 对象: %q", seed.Value)
			}
		}
	}
	if !found {
		t.Fatalf("设置种子缺少 wallet_config")
	}
	if !seen["reg_user_role"] {
		t.Fatalf("设置种子缺少历史键 reg_user_role")
	}
}

// TestDefaultPoliciesCoverEveryAPI 确认默认策略覆盖每一条种子 API：
// 只追加、不删除，管理员始终能访问历史路由。
func TestDefaultPoliciesCoverEveryAPI(t *testing.T) {
	policies := DefaultPolicies()
	for _, seed := range APISeeds() {
		want := [3]string{"admin", seed.Path, seed.Method}
		found := false
		for _, p := range policies {
			if p == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("默认策略缺少 %v（来自 API 种子 %v）", want, seed)
		}
	}
	for _, p := range EditorPolicy {
		want := [3]string{"editor", p[0], p[1]}
		found := false
		for _, got := range policies {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("默认策略缺少 editor 规则 %v", want)
		}
	}
}

// TestAdminUserSeedIsStable 确认默认管理员的名字与占位密码不会被随意改动。
func TestAdminUserSeedIsStable(t *testing.T) {
	seed := AdminUserSeed()
	if seed.Name != "admin" {
		t.Fatalf("默认管理员名 = %q", seed.Name)
	}
	if seed.Password != "password" {
		t.Fatalf("默认管理员密码占位符被改动: %q", seed.Password)
	}
	if seed.Email == "" || seed.Mobile == "" {
		t.Fatalf("默认管理员缺少邮箱或手机号: %#v", seed)
	}
}

// TestWalletDefaultConfigJSONIsClosedAndSecretFree 校验内置默认配置：
// 关闭态、revision 0、无任何秘密字段，且可被严格的 JSON 解析接受。
func TestWalletDefaultConfigJSONIsClosedAndSecretFree(t *testing.T) {
	raw := WalletDefaultConfigJSON()
	if len(raw) == 0 {
		t.Fatal("默认配置为空")
	}
	if len(raw) > 64<<10 {
		t.Fatalf("默认配置超过 64KiB: %d", len(raw))
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("默认配置不是合法 JSON: %v", err)
	}

	// 所有键都应是快照契约的一部分，且秘密只能出现在 profiles 中。
	allowed := map[string]bool{
		"revision": true, "wallet_currency": true, "provider_currency": true,
		"cny_per_usd_rate": true, "enabled": true, "recharge_mode": true,
		"redemption_enabled": true, "redemption_rate_limit": true,
		"redemption_rate_window_seconds": true, "profiles": true,
		"updated_by": true, "updated_at": true,
	}
	for key := range decoded {
		if !allowed[key] {
			t.Fatalf("默认配置包含未预期的键 %q", key)
		}
	}

	if decoded["revision"] != float64(0) {
		t.Fatalf("revision = %v，期望 0", decoded["revision"])
	}
	if decoded["enabled"] != false {
		t.Fatalf("enabled = %v，期望 false", decoded["enabled"])
	}
	if decoded["redemption_enabled"] != false {
		t.Fatalf("redemption_enabled = %v，期望 false", decoded["redemption_enabled"])
	}
	if decoded["recharge_mode"] != "DISABLED" {
		t.Fatalf("recharge_mode = %v，期望 DISABLED", decoded["recharge_mode"])
	}
	if decoded["wallet_currency"] != "CNY" || decoded["provider_currency"] != "USD" {
		t.Fatalf("币种不匹配: %v/%v", decoded["wallet_currency"], decoded["provider_currency"])
	}
	if profiles, ok := decoded["profiles"].(map[string]any); !ok || len(profiles) != 0 {
		t.Fatalf("默认配置不应预置 Provider Profile: %v", decoded["profiles"])
	}
	if strings.Contains(string(raw), "whsec_") || strings.Contains(string(raw), "api_key") {
		t.Fatalf("默认配置不应包含任何秘密: %s", raw)
	}
}

// TestSettingSeedValuesAreUsable 逐条确认种子值可用于直接写入设置表：
// 历史键允许空字符串（例如尚未配置的邮件主机），钱包配置必须是 JSON。
func TestSettingSeedValuesAreUsable(t *testing.T) {
	for _, seed := range SettingSeeds() {
		if strings.ContainsAny(seed.Key, " \t\n") {
			t.Fatalf("设置键包含空白字符: %q", seed.Key)
		}
		if seed.Key == "wallet_config" && !json.Valid([]byte(seed.Value)) {
			t.Fatalf("wallet_config 不是合法 JSON: %q", seed.Value)
		}
	}
}
