package conf

import (
	"fmt"
	"net/url"
	"strings"
)

// WalletConf 只承载随部署变化的钱包参数。
//
// 技术性约束（超时、响应体大小、刷新周期、分页上限等）属于代码常量，
// 见 internal/walletpolicy 包，不在此处暴露。
type WalletConf struct {
	// OwnerNamespace 是本钱包实例代表调用方派生的固定业务命名空间。
	// 设计 §12 要求它来自服务端启动配置（专属 listener 身份），
	// 请求体只能携带该命名空间内的业务键，不能覆盖它。
	OwnerNamespace string `mapstructure:"owner_namespace"`
	// ProductSuccessURLAllowedOrigins 是允许作为 Creem Product
	// default_success_url 的 Origin 白名单（scheme://host[:port]）。
	// 为空表示没有任何 Origin 被允许，充值功能将无法启用。
	ProductSuccessURLAllowedOrigins []string `mapstructure:"product_success_url_allowed_origins"`
}

// Validate 校验部署配置的格式。
func (c WalletConf) Validate() error {
	if !validOwnerNamespace(c.OwnerNamespace) {
		return fmt.Errorf("wallet.owner_namespace must match [a-z0-9][a-z0-9._-]{0,63}: %q", c.OwnerNamespace)
	}
	for _, origin := range c.ProductSuccessURLAllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil {
			return fmt.Errorf("wallet.product_success_url_allowed_origins contains invalid URL %q: %w", origin, err)
		}
		if parsed.Scheme != "https" {
			return fmt.Errorf("wallet.product_success_url_allowed_origins must use https: %q", origin)
		}
		if parsed.Host == "" {
			return fmt.Errorf("wallet.product_success_url_allowed_origins must contain a host: %q", origin)
		}
		if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
			(parsed.Path != "" && parsed.Path != "/") {
			return fmt.Errorf("wallet.product_success_url_allowed_origins must be an origin without path, query or credentials: %q", origin)
		}
	}
	return nil
}

// OriginAllowed 判断给定 URL 的 Origin 是否命中白名单。
//
// 只比较 scheme+host（含端口），不比较路径、查询串与凭证；
// 非 HTTPS、无法解析或白名单为空时一律拒绝（fail closed）。
func (c WalletConf) OriginAllowed(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	origin := originOf(parsed.Scheme, parsed.Host)
	for _, allowed := range c.ProductSuccessURLAllowedOrigins {
		allowedParsed, err := url.Parse(allowed)
		if err != nil || allowedParsed.Host == "" {
			continue
		}
		if originOf(allowedParsed.Scheme, allowedParsed.Host) == origin {
			return true
		}
	}
	return false
}

// originOf 规范化 Origin 字符串（统一小写）。
func originOf(scheme, host string) string {
	return strings.ToLower(scheme) + "://" + strings.ToLower(host)
}

// validOwnerNamespace 限定命名空间为小写字母/数字开头、最多 64 字符的安全 token，
// 避免把空白或控制字符写进不可变业务键。
func validOwnerNamespace(ns string) bool {
	if len(ns) == 0 || len(ns) > 64 {
		return false
	}
	for i, r := range ns {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
