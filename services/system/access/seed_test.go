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

// walletAdminRoutes 是任务 9 的管理员钱包路由，必须与
// api/finance/wallet/router.go 注册的路径和方法逐条一致。
func walletAdminRoutes() [][2]string {
	return [][2]string{
		{"POST", "/admin/wallets/list"},
		{"POST", "/admin/wallets/get"},
		{"POST", "/admin/transactions/list"},
		{"POST", "/admin/recharges/list"},
		{"GET", "/admin/recharges/:rechargeId"},
		{"POST", "/admin/recharges/:rechargeId/refresh"},
		{"POST", "/admin/recharges/:rechargeId/refund"},
		{"POST", "/admin/redemption-codes"},
		{"POST", "/admin/redemption-codes/list"},
		{"POST", "/admin/redemption-codes/:codeId/status"},
	}
}

// TestAPISeedsCoverWalletAdminRoutes 保证管理员钱包路由全部登记。
//
// 缺少任一条目时，首次初始化无法为 admin 角色生成策略，路由会一直 403。
func TestAPISeedsCoverWalletAdminRoutes(t *testing.T) {
	seen := make(map[string]APISeed, len(APISeeds()))
	for _, seed := range APISeeds() {
		seen[seed.Method+" "+seed.Path] = seed
	}

	for _, route := range walletAdminRoutes() {
		seed, ok := seen[route[0]+" "+route[1]]
		if !ok {
			t.Fatalf("missing api seed %s %s", route[0], route[1])
		}
		if seed.Group != "wallet" {
			t.Fatalf("api seed %s %s has unexpected group %q", route[0], route[1], seed.Group)
		}
		if seed.Description == "" {
			t.Fatalf("api seed %s %s has empty description", route[0], route[1])
		}
	}
}

// TestAPISeedsDoNotRegisterPlaintextObject 保证明文读取能力不会被登记成路由。
//
// wallet.redemption-code 是独立策略对象，不是 HTTP 路径；一旦混入 API 种子，
// 首次授权会把明文读取白送给普通管理员。
func TestAPISeedsDoNotRegisterPlaintextObject(t *testing.T) {
	for _, seed := range APISeeds() {
		if seed.Path == "wallet.redemption-code" {
			t.Fatalf("plaintext policy object must not be registered as an api seed")
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
