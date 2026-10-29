package trace

import "go.uber.org/fx"

// Module 提供全局 Tracer，并用 fx.Invoke 强制其在 services / internal.client 之前完成安装。
func Module() fx.Option {
	return fx.Module("internal.trace",
		fx.Provide(NewTracer),
		fx.Invoke(func(*Tracer) {}),
	)
}
