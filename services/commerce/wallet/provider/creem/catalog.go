package creem

import (
	"context"
	"strings"
	"time"

	creemapi "github.com/KiClover/orzAdmin/internal/creem"
	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
)

// Catalog 是只读的 Product 元数据读取器。
//
// 充值创建需要 Product 自身配置的 default_success_url，但 Setting 的 Profile 只保存
// Product ID 与凭据；这里用同一个共享 Creem 客户端读取该地址，避免把跳转地址
// 变成请求体参数或被配置复制。Catalog 不缓存，也不写任何状态。
//
// 跳转地址完全由 Creem Product 决定，本服务不做域名级校验，也不会覆盖它。
type Catalog struct {
	timeout          time.Duration
	maxResponseBytes int64
	doer             creemapi.Doer
}

// CatalogOptions 是 Catalog 的依赖集合；技术限制默认取代码内常量。
type CatalogOptions struct {
	// Timeout 与 MaxResponseBytes 为零时取 walletpolicy 常量。
	Timeout          time.Duration
	MaxResponseBytes int64
	// Doer 仅供测试注入；生产使用内部 http.Client。
	Doer creemapi.Doer
}

// NewCatalog 构造只读 Product 目录读取器。
func NewCatalog(options CatalogOptions) (*Catalog, error) {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = walletpolicy.ProviderRequestTimeout
	}
	maxBytes := options.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = walletpolicy.ProviderMaxResponseBytes
	}
	return &Catalog{
		timeout:          timeout,
		maxResponseBytes: maxBytes,
		doer:             options.Doer,
	}, nil
}

// ProductSuccessURL 返回 Product 配置的 default_success_url。
//
// 只要求 Product 处于可售状态且币种为 USD；跳转地址原样返回，由 Creem 决定。
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
	// Provider 的大小写不作承诺：status/currency 按忽略大小写比较，
	// 否则 Product 返回 "USD"/"Active" 时会被误判为不可用。
	if !strings.EqualFold(product.Status, "active") || !strings.EqualFold(product.Currency, "usd") {
		return "", payment.ErrGatewayInvalidResponse
	}
	return product.DefaultSuccessURL, nil
}

// 编译期断言：Catalog 必须实现 payment.ProductCatalog。
var _ payment.ProductCatalog = (*Catalog)(nil)
