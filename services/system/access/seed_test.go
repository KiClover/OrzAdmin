package access

import "testing"

// TestAPISeedsAreUnique 保证种子里的 (path, method) 不重复：
// 唯一索引一旦冲突，初始化会直接失败，重跑也无法补齐。
func TestAPISeedsAreUnique(t *testing.T) {
	seen := make(map[string]struct{}, len(APISeeds()))
	for _, seed := range APISeeds() {
		key := seed.Method + " " + seed.Path
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate api seed %q", key)
		}
		seen[key] = struct{}{}
	}
}

// TestAPISeedsCoverWalletSettingRoutes 保证钱包设置的读写权限有对应的路由记录，
// 否则 Casbin 无法为管理员策略提供对象。
func TestAPISeedsCoverWalletSettingRoutes(t *testing.T) {
	seen := make(map[string]APISeed, len(APISeeds()))
	for _, seed := range APISeeds() {
		seen[seed.Method+" "+seed.Path] = seed
	}
	for _, method := range []string{"GET", "PUT"} {
		seed, ok := seen[method+" /setting/wallet"]
		if !ok {
			t.Fatalf("missing api seed %s /setting/wallet", method)
		}
		if seed.Group == "" || seed.Description == "" {
			t.Fatalf("api seed %s /setting/wallet has empty group or description", method)
		}
	}
}

// TestRoleSeedsContainAdminAndUser 保证默认角色齐全，首次授权依赖 admin 角色存在。
func TestRoleSeedsContainAdminAndUser(t *testing.T) {
	codes := make(map[string]struct{}, len(RoleSeeds()))
	for _, seed := range RoleSeeds() {
		codes[seed.Code] = struct{}{}
	}
	for _, code := range []string{"admin", "user"} {
		if _, ok := codes[code]; !ok {
			t.Fatalf("missing role seed %q", code)
		}
	}
}
