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
	// ProductSuccessURLAllowedOrigins 是允许作为 Creem Product
	// default_success_url 的 Origin 白名单（scheme://host[:port]）。
	// 为空表示没有任何 Origin 被允许，充值功能将无法启用。
	ProductSuccessURLAllowedOrigins []string `mapstructure:"product_success_url_allowed_origins"`
}

// Validate 校验部署配置的格式。
func (c WalletConf) Validate() error {
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
