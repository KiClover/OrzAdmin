package main

import (
	"context"

	"github.com/KiClover/orzfrp/api/system/auth"
	"github.com/KiClover/orzfrp/bootstrap"
	"github.com/KiClover/orzfrp/internal/constructor"
	"go.uber.org/fx"
)

func main() {
	/*
		bootstrap.NewApp(
			context.Background(),
			"auth",
			fx.Provide(func() *server.Hertz {
				return server.New(server.WithHostPorts("localhost:8889"))
			}),
			fx.Invoke(auth.NewRegisterRouter), // 先注册路由
			fx.Invoke(func(h *server.Hertz) {
				h.Spin() // 最后启动服务器
			}),
		)
	*/

	bootstrap.NewApp(
		context.Background(),
		auth.ServiceName,
		fx.Provide(constructor.NewHzService),
		fx.Invoke(auth.NewRegisterRouter),
		fx.Invoke(constructor.RunHzService),
	)
}
