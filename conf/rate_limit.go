package conf

import (
	"fmt"
	"math"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// RateLimitConf 包含通用限流策略和可信代理配置。
type RateLimitConf struct {
	// TrustedProxyCIDRs 为空时不信任任何转发头，使用实际连接地址。
	TrustedProxyCIDRs []string                   `mapstructure:"trusted_proxy_cidrs"`
	Policies          map[string]RateLimitPolicy `mapstructure:"policies"`
}

type RateLimitPolicy struct {
	User RateLimitRule `mapstructure:"user"`
	IP   RateLimitRule `mapstructure:"ip"`
}

type RateLimitRule struct {
	Enabled       bool  `mapstructure:"enabled"`
	WindowSeconds int64 `mapstructure:"window_seconds"`
	MaxRequests   int64 `mapstructure:"max_requests"`
}

// Validate 拒绝非法策略名、限流数值与可信代理，供启动阶段使用。
func (c RateLimitConf) Validate() error {
	for name, policy := range c.Policies {
		if !validOwnerNamespace(name) {
			return fmt.Errorf("rate_limit.policies name must match [a-z0-9][a-z0-9._-]{0,63}: %q", name)
		}
		for _, entry := range []struct {
			name string
			rule RateLimitRule
		}{{"user", policy.User}, {"ip", policy.IP}} {
			path := fmt.Sprintf("rate_limit.policies.%s.%s", name, entry.name)
			if entry.rule.WindowSeconds < 0 || entry.rule.MaxRequests < 0 {
				return fmt.Errorf("%s window_seconds and max_requests must not be negative", path)
			}
			if entry.rule.Enabled && (entry.rule.WindowSeconds == 0 || entry.rule.MaxRequests == 0) {
				return fmt.Errorf("%s enabled rule requires window_seconds and max_requests > 0", path)
			}
			if entry.rule.WindowSeconds > math.MaxInt64/int64(time.Second) {
				return fmt.Errorf("%s.window_seconds overflows time.Duration", path)
			}
		}
	}
	_, err := c.TrustedProxies()
	return err
}

// loadRateLimitPolicies 只从原始 YAML 恢复策略名，避免 Viper 小写化、
// 点号展开和丢弃空映射。所有规则值仍从 Viper 获取，以保持默认值 → YAML → 环境变量。
func loadRateLimitPolicies(v *viper.Viper) (map[string]RateLimitPolicy, error) {
	names := map[string]any{"wallet_redemption": nil}
	if path := v.ConfigFileUsed(); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read rate limit policies: %w", err)
		}
		var raw struct {
			RateLimit struct {
				Policies map[string]any `yaml:"policies"`
			} `yaml:"rate_limit"`
		}
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("failed to decode rate limit policies: %w", err)
		}
		for name := range raw.RateLimit.Policies {
			names[name] = nil
		}
	}
	policies := make(map[string]RateLimitPolicy, len(names))
	for name := range names {
		if !validOwnerNamespace(name) {
			return nil, fmt.Errorf("rate_limit.policies name must match [a-z0-9][a-z0-9._-]{0,63}: %q", name)
		}
		var policy RateLimitPolicy
		for _, dimension := range []struct {
			name string
			rule *RateLimitRule
		}{{"user", &policy.User}, {"ip", &policy.IP}} {
			prefix := "rate_limit.policies." + name + "." + dimension.name
			if value := v.Get(prefix + ".enabled"); value != nil {
				enabled, err := strconv.ParseBool(fmt.Sprint(value))
				if err != nil {
					return nil, fmt.Errorf("%s.enabled must be a boolean", prefix)
				}
				dimension.rule.Enabled = enabled
			}
			for _, field := range []struct {
				name   string
				target *int64
			}{{"window_seconds", &dimension.rule.WindowSeconds}, {"max_requests", &dimension.rule.MaxRequests}} {
				if value := v.Get(prefix + "." + field.name); value != nil {
					// Parse before conversion: weak struct decoding truncates fractional YAML values.
					integer, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
					if err != nil {
						return nil, fmt.Errorf("%s.%s must be an integer", prefix, field.name)
					}
					*field.target = integer
				}
			}
		}
		policies[name] = policy
	}
	return policies, nil
}

// TrustedProxies 返回解析后的可信代理网段；任一非法配置都返回错误。
func (c RateLimitConf) TrustedProxies() ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(c.TrustedProxyCIDRs))
	for _, cidr := range c.TrustedProxyCIDRs {
		prefix, err := parseProxyPrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("rate_limit.trusted_proxy_cidrs contains invalid CIDR %q: %w", cidr, err)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

// parseProxyPrefix 接受 CIDR 或单个 IPv4/IPv6 地址（自动补全主机位）。
func parseProxyPrefix(raw string) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Prefix{}, fmt.Errorf("empty proxy address")
	}
	if strings.Contains(raw, "/") {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return netip.Prefix{}, err
		}
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}
