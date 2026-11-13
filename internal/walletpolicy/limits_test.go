package walletpolicy

import "testing"

// TestWalletRPCTimeoutOutlastsProviderRequest 固定"上层预算大于下层预算"这条约束。
//
// 若 RPC 超时小于渠道超时，钱包在下层仍可能成功建单，而上层已经超时返回；
// 用户重试会生成第二笔订单，本地又看不到第一笔的 checkout。
func TestWalletRPCTimeoutOutlastsProviderRequest(t *testing.T) {
	if WalletRPCTimeout <= ProviderRequestTimeout {
		t.Fatalf("WalletRPCTimeout (%s) must exceed ProviderRequestTimeout (%s)", WalletRPCTimeout, ProviderRequestTimeout)
	}
	if WalletRPCTimeout <= ProviderConnectTimeout {
		t.Fatalf("WalletRPCTimeout (%s) must exceed ProviderConnectTimeout (%s)", WalletRPCTimeout, ProviderConnectTimeout)
	}
}