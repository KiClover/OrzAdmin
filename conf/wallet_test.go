package conf

import (
	"reflect"
	"strings"
	"testing"
)

func TestWalletConfValidate(t *testing.T) {
	valid := WalletConf{
		OwnerNamespace:                  "commerce",
		ProductSuccessURLAllowedOrigins: []string{"https://pay.example.com", "https://EXAMPLE.com:8443"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("合法配置被拒绝: %v", err)
	}

	emptyOrigin := WalletConf{OwnerNamespace: "commerce"}
	if err := emptyOrigin.Validate(); err != nil {
		t.Fatalf("空白名单应合法（由校验阶段 fail closed）: %v", err)
	}

	invalid := [][]string{
		{"http://pay.example.com"},
		{"pay.example.com"},
		{"https://pay.example.com/callback"},
		{"https://pay.example.com?x=1"},
		{"https://user:pass@pay.example.com"},
		{"https://pay.example.com#frag"},
	}
	for _, entries := range invalid {
		conf := WalletConf{OwnerNamespace: "commerce", ProductSuccessURLAllowedOrigins: entries}
		if err := conf.Validate(); err == nil {
			t.Fatalf("非法 Origin %v 应被拒绝", entries)
		}
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

func TestWalletConfOriginAllowed(t *testing.T) {
	conf := WalletConf{
		OwnerNamespace:                  "commerce",
		ProductSuccessURLAllowedOrigins: []string{"https://pay.example.com", "https://Pay.Example.com:8443/"},
	}

	allowed := []string{
		"https://pay.example.com",
		"https://pay.example.com/",
		"https://pay.example.com/success?recharge=1",
		"https://PAY.EXAMPLE.COM/checkout/return",
		"https://pay.example.com:8443/x",
	}
	for _, raw := range allowed {
		if !conf.OriginAllowed(raw) {
			t.Fatalf("%q 应被允许", raw)
		}
	}

	denied := []string{
		"http://pay.example.com/success",
		"https://evil.example.com/success",
		"https://sub.pay.example.com/success",
		"https://pay.example.com:9443/success",
		"https://pay.example.com@evil.example.com/success",
		"not a url",
		"",
	}
	for _, raw := range denied {
		if conf.OriginAllowed(raw) {
			t.Fatalf("%q 不应被允许", raw)
		}
	}

	empty := WalletConf{OwnerNamespace: "commerce"}
	if empty.OriginAllowed("https://pay.example.com/success") {
		t.Fatalf("空白名单必须拒绝所有 Origin")
	}
}

func TestLoadConfigWalletDefaultsAndEnv(t *testing.T) {
	cfg := loadConfig("etc/does-not-exist.yaml")
	if len(cfg.Wallet.ProductSuccessURLAllowedOrigins) != 0 {
		t.Fatalf("默认白名单应为空: %v", cfg.Wallet.ProductSuccessURLAllowedOrigins)
	}
	if cfg.Wallet.OwnerNamespace != "commerce" {
		t.Fatalf("默认 owner_namespace 应为 commerce: %q", cfg.Wallet.OwnerNamespace)
	}

	t.Setenv("WALLET_OWNER_NAMESPACE", "shop")
	t.Setenv("WALLET_PRODUCT_SUCCESS_URL_ALLOWED_ORIGINS", "https://a.example.com,https://b.example.com:8443")
	cfg = loadConfig("etc/does-not-exist.yaml")
	if cfg.Wallet.OwnerNamespace != "shop" {
		t.Fatalf("WALLET_OWNER_NAMESPACE 未正确解析: %q", cfg.Wallet.OwnerNamespace)
	}
	want := []string{"https://a.example.com", "https://b.example.com:8443"}
	if !reflect.DeepEqual(cfg.Wallet.ProductSuccessURLAllowedOrigins, want) {
		t.Fatalf("环境变量未正确解析: %v，期望 %v", cfg.Wallet.ProductSuccessURLAllowedOrigins, want)
	}
}
