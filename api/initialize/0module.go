package initialize

import (
	"go.uber.org/fx"
)

// Module 提供初始化 API 的 handler 与其依赖。
//
// 服务客户端、数据库与日志等基础设施与其它 API 完全一致，由 bootstrap.AppEntries 提供：
// 初始化 API 不自己维护服务发现，也不自己建立连接。
func Module() fx.Option {
	return fx.Module(
		"api.initialize",
		fx.Provide(NewRPCInitializer),
		fx.Provide(NewHzInitializeService),
	)
}
