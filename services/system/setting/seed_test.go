package setting

import "testing"

// TestSettingSeedsKeysAreUnique 保证种子 key 不重复：settings 表的 key 有唯一约束。
func TestSettingSeedsKeysAreUnique(t *testing.T) {
	seen := make(map[string]struct{}, len(SettingSeeds()))
	for _, seed := range SettingSeeds() {
		if _, ok := seen[seed.Key]; ok {
			t.Fatalf("duplicate setting seed %q", seed.Key)
		}
		seen[seed.Key] = struct{}{}
	}
}

// TestSettingSeedsContainRegUserRole 保证默认注册角色仍被种下。
func TestSettingSeedsContainRegUserRole(t *testing.T) {
	for _, seed := range SettingSeeds() {
		if seed.Key == "reg_user_role" {
			if seed.Value == "" {
				t.Fatal("reg_user_role seed must not be empty")
			}
			return
		}
	}
	t.Fatal("missing reg_user_role seed")
}

// TestSettingSeedsContainDecodableWalletConfig 保证 wallet_config 种子能被服务的
// 严格解码器接受，并且是“关闭态”的默认快照。
func TestSettingSeedsContainDecodableWalletConfig(t *testing.T) {
	var raw string
	for _, seed := range SettingSeeds() {
		if seed.Key == WalletConfigKey {
			raw = seed.Value
			break
		}
	}
	if raw == "" {
		t.Fatalf("missing %s seed", WalletConfigKey)
	}
	snapshot, err := decodeWalletSnapshot([]byte(raw))
	if err != nil {
		t.Fatalf("wallet_config seed is not decodable: %v", err)
	}
	if snapshot.Revision != 0 || snapshot.Enabled || snapshot.RechargeMode != RechargeModeDisabled {
		t.Fatalf("wallet_config seed must be the closed default: %+v", snapshot)
	}
}
