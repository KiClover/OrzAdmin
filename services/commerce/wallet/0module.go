package wallet

import (
	"context"

	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	settings "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
	creemprovider "github.com/KiClover/orzAdmin/services/commerce/wallet/provider/creem"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

// Module 提供 Commerce Wallet 的业务实现、运行时 Setting 快照服务、支付网关工厂与 Product 目录。
func Module() fx.Option {
	return fx.Options(
		fx.Provide(NewCommerceWalletService),
		fx.Provide(newWalletRuntime),
		fx.Provide(NewPaymentGatewayFactory),
		fx.Provide(NewProductCatalog),
	)
}

// gatewayFactory 按 Profile 凭据构造 Creem 网关，并统一套用代码内的技术限制。
//
// 超时与响应上限不进入 YAML：它们是无条件的安全边界，必须对所有环境一致。
type gatewayFactory struct{}

// NewPaymentGatewayFactory 返回支付网关工厂，供充值流程按订单冻结的 Profile 构造网关。
func NewPaymentGatewayFactory() payment.PaymentGatewayFactory {
	return gatewayFactory{}
}

// NewGateway 实现 payment.PaymentGatewayFactory。
func (gatewayFactory) NewGateway(credentials payment.GatewayCredentials) (payment.PaymentGateway, error) {
	return creemprovider.New(creemprovider.Options{
		Credentials:      credentials,
		Timeout:          walletpolicy.ProviderRequestTimeout,
		MaxResponseBytes: walletpolicy.ProviderMaxResponseBytes,
	})
}

// 编译期断言：工厂必须实现 payment.PaymentGatewayFactory。
var _ payment.PaymentGatewayFactory = gatewayFactory{}

// NewProductCatalog 构造 Product 目录查询器。
//
// 成功跳转地址完全来自 Creem Product 自身配置，本服务不维护域名白名单。
func NewProductCatalog() (payment.ProductCatalog, error) {
	return creemprovider.NewCatalog(creemprovider.CatalogOptions{
		Timeout:          walletpolicy.ProviderRequestTimeout,
		MaxResponseBytes: walletpolicy.ProviderMaxResponseBytes,
	})
}

// settingSource 把 system-setting 的内部 RPC 客户端适配成 Wallet 的窄接口。
// 生成客户端带 variadic callopt，适配器固定本地调用形态，业务代码不感知传输细节。
type settingSource struct {
	client systemsettingservice.Client
}

func (s settingSource) GetWalletRuntimeSettings(ctx context.Context, req *base.Empty) (*settings.WalletRuntimeSettings, error) {
	return s.client.GetWalletRuntimeSettings(ctx, req)
}

// newWalletRuntime 构造运行时快照服务并挂到 fx 生命周期：
// 启动时立即加载一次并每 walletpolicy.SettingRefreshInterval 刷新，停止时取消后台循环。
func newWalletRuntime(
	lc fx.Lifecycle,
	logger *logrus.Logger,
	store *db.WalletServiceDB,
	client systemsettingservice.Client,
) (*Service, error) {
	runtime, err := NewService(Options{
		Source:   settingSource{client: client},
		Presence: store,
		Logger:   logger,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			runtime.Start(ctx)
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})
	return runtime, nil
}
