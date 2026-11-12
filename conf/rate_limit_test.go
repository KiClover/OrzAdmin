package conf

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func rateLimitConfigFile(t *testing.T, yaml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rate-limit.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigRateLimitDefaults(t *testing.T) {
	cfg := loadConfig(rateLimitConfigFile(t, "logger:\n  level: info\n"))
	want := RateLimitPolicy{
		User: RateLimitRule{Enabled: true, WindowSeconds: 600, MaxRequests: 3},
		IP:   RateLimitRule{Enabled: false, WindowSeconds: 60, MaxRequests: 30},
	}
	if got := cfg.RateLimit.Policies["wallet_redemption"]; got != want {
		t.Fatalf("default wallet_redemption = %+v, want %+v", got, want)
	}
	if len(cfg.RateLimit.TrustedProxyCIDRs) != 0 {
		t.Fatalf("default trusted proxies = %v, want empty", cfg.RateLimit.TrustedProxyCIDRs)
	}
}

func TestLoadConfigRateLimitYAMLOverridesIndependentRules(t *testing.T) {
	cfg := loadConfig(rateLimitConfigFile(t, `rate_limit:
  policies:
    wallet_redemption:
      user: {enabled: false, window_seconds: 120, max_requests: 7}
      ip: {enabled: true, window_seconds: 15, max_requests: 20}
    checkout.v2:
      user: {enabled: true, window_seconds: 8, max_requests: 4}
`))
	want := RateLimitPolicy{
		User: RateLimitRule{Enabled: false, WindowSeconds: 120, MaxRequests: 7},
		IP:   RateLimitRule{Enabled: true, WindowSeconds: 15, MaxRequests: 20},
	}
	if got := cfg.RateLimit.Policies["wallet_redemption"]; got != want {
		t.Fatalf("wallet_redemption = %+v, want %+v", got, want)
	}
	if got := cfg.RateLimit.Policies["checkout.v2"].User; got != (RateLimitRule{Enabled: true, WindowSeconds: 8, MaxRequests: 4}) {
		t.Fatalf("dotted policy name lost: %+v", cfg.RateLimit.Policies)
	}
}

func TestLoadConfigRateLimitCustomPolicyEnvOverrides(t *testing.T) {
	t.Setenv("ORZFRP_RATE_LIMIT_POLICIES_CHECKOUT_USER_MAX_REQUESTS", "9")
	t.Setenv("ORZFRP_RATE_LIMIT_POLICIES_CHECKOUT_IP_ENABLED", "false")
	cfg := loadConfig(rateLimitConfigFile(t, `rate_limit:
  policies:
    checkout:
      user: {enabled: true, window_seconds: 60, max_requests: 2}
      ip: {enabled: true, window_seconds: 20, max_requests: 8}
`))
	got := cfg.RateLimit.Policies["checkout"]
	if got.User.MaxRequests != 9 || got.IP.Enabled {
		t.Fatalf("environment overrides lost: %+v", got)
	}
}

func TestLoadConfigRateLimitPartialOverrideAndDisabledZeros(t *testing.T) {
	cfg := loadConfig(rateLimitConfigFile(t, `rate_limit:
  policies:
    wallet_redemption:
      user: {enabled: false, window_seconds: 0, max_requests: 0}
    zero-rule:
      ip: {enabled: false, window_seconds: 0, max_requests: 0}
    duration_boundary:
      user: {enabled: true, window_seconds: 9223372036, max_requests: 1}
`))
	policy := cfg.RateLimit.Policies["wallet_redemption"]
	if policy.User != (RateLimitRule{}) {
		t.Fatalf("disabled zero user = %+v, want zeros", policy.User)
	}
	if policy.IP != (RateLimitRule{WindowSeconds: 60, MaxRequests: 30}) {
		t.Fatalf("omitted IP rule lost defaults: %+v", policy.IP)
	}
}

func TestLoadConfigRateLimitTrustedProxiesEnv(t *testing.T) {
	t.Setenv("RATE_LIMIT_TRUSTED_PROXY_CIDRS", "10.0.0.7/24, 192.0.2.4,2001:db8::1")
	t.Setenv("WALLET_TRUSTED_PROXY_CIDRS", "bad-legacy-proxy")
	cfg := loadConfig(rateLimitConfigFile(t, "rate_limit:\n  trusted_proxy_cidrs: ['203.0.113.0/24']\nwallet:\n  trusted_proxy_cidrs: ['bad-legacy-proxy']\n"))
	got, err := cfg.RateLimit.TrustedProxies()
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.4/32"),
		netip.MustParsePrefix("2001:db8::1/128"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trusted proxies = %v, want %v", got, want)
	}
}

func TestRateLimitConfValidationAndTrustedProxies(t *testing.T) {
	for _, name := range []string{"a", "0", "wallet_redemption", "a.b-c_d", strings.Repeat("a", 64)} {
		if err := (RateLimitConf{Policies: map[string]RateLimitPolicy{name: {}}}).Validate(); err != nil {
			t.Fatalf("valid policy name %q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"", "UPPER", "a/b", "a b", ".a", "-a", "_a", strings.Repeat("a", 65)} {
		if err := (RateLimitConf{Policies: map[string]RateLimitPolicy{name: {}}}).Validate(); err == nil {
			t.Fatalf("invalid policy name %q accepted", name)
		}
	}
	for _, proxy := range []string{"", "not-an-ip", "10.0.0.1/33", "[::1]:80"} {
		cfg := RateLimitConf{TrustedProxyCIDRs: []string{proxy}}
		if _, err := cfg.TrustedProxies(); err == nil {
			t.Fatalf("invalid proxy %q accepted", proxy)
		}
	}
}

func TestLoadConfigRateLimitRejectsInvalidSettings(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"policy name", "rate_limit:\n  policies:\n    BadName:\n      user: {enabled: false}\n", "rate_limit.policies"},
		{"empty policy name", "rate_limit:\n  policies:\n    '': {}\n", "rate_limit.policies"},
		{"leading punctuation", "rate_limit:\n  policies:\n    -bad: {}\n", "rate_limit.policies"},
		{"long policy name", "rate_limit:\n  policies:\n    " + strings.Repeat("a", 65) + ": {}\n", "rate_limit.policies"},
		{"fractional window", "rate_limit:\n  policies:\n    custom:\n      user: {enabled: true, window_seconds: 1.5, max_requests: 3}\n", "window_seconds"},
		{"fractional max", "rate_limit:\n  policies:\n    wallet_redemption:\n      user: {enabled: true, window_seconds: 60, max_requests: 1.5}\n", "max_requests"},
		{"enabled zero window", "rate_limit:\n  policies:\n    custom:\n      user: {enabled: true, window_seconds: 0, max_requests: 3}\n", "user"},
		{"enabled zero max", "rate_limit:\n  policies:\n    custom:\n      ip: {enabled: true, window_seconds: 1, max_requests: 0}\n", "ip"},
		{"disabled negative window", "rate_limit:\n  policies:\n    custom:\n      user: {enabled: false, window_seconds: -1, max_requests: 0}\n", "user"},
		{"disabled negative max", "rate_limit:\n  policies:\n    custom:\n      ip: {enabled: false, window_seconds: 0, max_requests: -1}\n", "ip"},
		{"duration overflow", "rate_limit:\n  policies:\n    custom:\n      user: {enabled: true, window_seconds: 9223372037, max_requests: 1}\n", "window_seconds"},
		{"disabled duration overflow", "rate_limit:\n  policies:\n    custom:\n      ip: {enabled: false, window_seconds: 9223372037, max_requests: 0}\n", "window_seconds"},
		{"invalid proxy", "rate_limit:\n  trusted_proxy_cidrs: ['bad-proxy']\n", "rate_limit.trusted_proxy_cidrs"},
		{"empty proxy", "rate_limit:\n  trusted_proxy_cidrs: ['']\n", "rate_limit.trusted_proxy_cidrs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := rateLimitConfigFile(t, tc.yaml)
			defer func() {
				value := recover()
				if value == nil {
					t.Fatal("loadConfig accepted invalid rate limit settings")
				}
				if !strings.Contains(fmt.Sprint(value), tc.want) {
					t.Fatalf("panic = %v, want context %q", value, tc.want)
				}
			}()
			loadConfig(path)
		})
	}
}
