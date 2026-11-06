package main

import (
	"context"

	"github.com/KiClover/orzAdmin/api/system/setting"
	"github.com/KiClover/orzAdmin/bootstrap"
	"github.com/KiClover/orzAdmin/internal/constructor"
	"go.uber.org/fx"
)

func main() {
	bootstrap.NewApp(
		context.Background(),
		setting.ServiceName,
		fx.Provide(constructor.NewHzService),
		fx.Invoke(setting.NewRegisterRouter),
		fx.Invoke(constructor.RunHzService),
	)
}
