package creem

import (
	"context"
	"net/url"
	"strings"
	"time"

	creemapi "github.com/KiClover/orzAdmin/internal/creem"
	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
)

// Catalog 是只读的 Product 元数据读取器。
//
// 充值创建需要 Product 已校验的 default_success_url，但 Setting 的 Profile 只保存
// Product ID 与凭据；这里用同一个共享 Creem 客户端读取该地址，避免把跳转地址
// 变成请求体参数或被配置复制。Catalog 不缓存，也不写任何状态。
type Catalog struct {
	allowed          []string
	timeout          time.Duration
	maxResponseBytes int64
	doer             creemapi.Doer
}

// CatalogOptions 是 Catalog 的依赖集合；技术限制默认取代码内常量。
type CatalogOptions struct {
	// AllowedOrigins 是部署配置允许的成功跳转 Origin（scheme+host）。
	AllowedOrigins []string
	// Timeout 与 MaxResponseBytes 为零时取 walletpolicy 常量。
	Timeout          time.Duration
	MaxResponseBytes int64
	// Doer 仅供测试注入；生产使用内部 http.Client。
	Doer creemapi.Doer
}

// NewCatalog 构造只读 Product 目录读取器。
func NewCatalog(options CatalogOptions) (*Catalog, error) {
	allowed := make([]string, 0, len(options.AllowedOrigins))
	for _, origin := range options.AllowedOrigins {
		normalized := strings.ToLower(strings.TrimRight(strings.TrimSpace(origin), "/"))
		if normalized == "" {
			continue
		}
		allowed = append(allowed, normalized)
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = walletpolicy.ProviderRequestTimeout
	}
	maxBytes := options.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = walletpolicy.ProviderMaxResponseBytes
	}
	return &Catalog{
		allowed:          allowed,
		timeout:          timeout,
		maxResponseBytes: maxBytes,
		doer:             options.Doer,
	}, nil
}

// ProductSuccessURL 返回 Product 的 default_success_url，并校验其 Origin 在部署白名单内。
func (c *Catalog) ProductSuccessURL(
	ctx context.Context,
	credentials payment.GatewayCredentials,
) (string, error) {
	environment, _, err := environmentOf(credentials.Environment)
	if err != nil {
		return "", payment.ErrGatewayInvalidResponse
	}
	client, err := creemapi.NewClient(creemapi.Options{
		Environment:      environment,
		APIKey:           credentials.APIKey,
		Timeout:          c.timeout,
		MaxResponseBytes: c.maxResponseBytes,
		Doer:             c.doer,
	})
	if err != nil {
		return "", payment.ErrGatewayInvalidResponse
	}
	product, err := client.GetProduct(ctx, credentials.ProductID)
	if err != nil {
		return "", mapError("get product", err)
	}
	if product.Status != "active" || product.Currency != "usd" {
		return "", payment.ErrGatewayInvalidResponse
	}
	parsed, err := url.Parse(product.DefaultSuccessURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", payment.ErrGatewayInvalidResponse
	}
	origin := strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	if !c.originAllowed(origin) {
		return "", payment.ErrGatewayInvalidResponse
	}
	return parsed.String(), nil
}

// originAllowed 与 conf.WalletConf 的白名单语义一致：空白名单 fail closed。
func (c *Catalog) originAllowed(origin string) bool {
	for _, allowed := range c.allowed {
		if allowed == origin {
			return true
		}
	}
	return false
}

// 编译期断言：Catalog 必须实现 payment.ProductCatalog。
var _ payment.ProductCatalog = (*Catalog)(nil)
