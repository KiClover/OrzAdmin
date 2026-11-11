// 初始化 API 的进程入口。
//
// 与其它 API 不同，这里刻意不加载 bootstrap.AppEntries：初始化服务启动时
// 不应连接服务发现、不应提前创建任何业务表，只有真正调用 /init/services 时
// 才按需建立数据库连接。
package main

import (
	"context"

	"github.com/KiClover/orzAdmin/api/initialize"
	"github.com/KiClover/orzAdmin/conf"
	internallog "github.com/KiClover/orzAdmin/internal/log"
	internaltrace "github.com/KiClover/orzAdmin/internal/trace"
	"github.com/cloudwego/hertz/pkg/app/server"
	"go.uber.org/fx"
)

func main() {
	app := fx.New(
		fx.Provide(conf.NewLoadConf),
		internallog.Module(),
		fx.WithLogger(internallog.NewFxEventLogger),
		internaltrace.Module(),
		initialize.Module(),
		fx.Invoke(initialize.NewRegisterRouter),
		fx.Invoke(initialize.CloseInitializer),
		fx.Invoke(runHzService),
	)
	app.Run()
}

// runHzService 启动 Hertz 并把优雅关停挂到 Fx 生命周期上。
func runHzService(lc fx.Lifecycle, h *server.Hertz) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go h.Spin()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return h.Shutdown(ctx)
		},
	})
}
