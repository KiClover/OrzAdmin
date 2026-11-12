package wallet

import (
	"context"

	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	settings "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting/systemsettingservice"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

// Module 提供 Commerce Wallet 的业务实现与运行时 Setting 快照服务。
func Module() fx.Option {
	return fx.Options(
		fx.Provide(NewCommerceWalletService),
		fx.Provide(newWalletRuntime),
	)
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
