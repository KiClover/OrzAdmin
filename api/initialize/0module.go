package initialize

import (
	orzinit "github.com/KiClover/orzAdmin/internal/initialize"
	"go.uber.org/fx"
)

// Module 提供初始化 API 的 handler 与其依赖。
//
// 刻意只提供 handler 与初始化器：不加载服务发现、业务服务与业务客户端，
// 保证初始化 API 启动时不连接注册中心、不提前创建任何业务表。
func Module() fx.Option {
	return fx.Module(
		"api.initialize",
		fx.Provide(orzinit.NewInitializer),
		fx.Provide(ProvideInitializer),
		fx.Provide(NewHzInitializeService),
		fx.Provide(NewService),
	)
}
