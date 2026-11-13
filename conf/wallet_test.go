package conf

import (
	"strings"
	"testing"
)

func TestWalletConfValidate(t *testing.T) {
	if err := (WalletConf{OwnerNamespace: "commerce"}).Validate(); err != nil {
		t.Fatalf("合法配置被拒绝: %v", err)
	}

	invalidNamespaces := []string{"", " ", "-commerce", ".commerce", "Commerce", "comm erce", "a/b", strings.Repeat("a", 65)}
	for _, ns := range invalidNamespaces {
		if err := (WalletConf{OwnerNamespace: ns}).Validate(); err == nil {
			t.Fatalf("非法 owner_namespace %q 应被拒绝", ns)
		}
	}
	for _, ns := range []string{"commerce", "a", "wallet-1", "shop.prod", "a_b"} {
		if err := (WalletConf{OwnerNamespace: ns}).Validate(); err != nil {
			t.Fatalf("合法 owner_namespace %q 被拒绝: %v", ns, err)
		}
	}
}

func TestLoadConfigWalletDefaultsAndEnv(t *testing.T) {
	cfg := loadConfig("etc/does-not-exist.yaml")
	if cfg.Wallet.OwnerNamespace != "commerce" {
		t.Fatalf("默认 owner_namespace 应为 commerce: %q", cfg.Wallet.OwnerNamespace)
	}

	t.Setenv("WALLET_OWNER_NAMESPACE", "shop")
	cfg = loadConfig("etc/does-not-exist.yaml")
	if cfg.Wallet.OwnerNamespace != "shop" {
		t.Fatalf("WALLET_OWNER_NAMESPACE 未正确解析: %q", cfg.Wallet.OwnerNamespace)
	}
}
