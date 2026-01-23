package main

import (
	"context"

	"github.com/KiClover/orzAdmin/api/system/permission"
	"github.com/KiClover/orzAdmin/bootstrap"
	"github.com/KiClover/orzAdmin/internal/constructor"
	"go.uber.org/fx"
)

func main() {
	bootstrap.NewApp(
		context.Background(),
		permission.ServiceName,
		fx.Provide(constructor.NewHzService),
		fx.Invoke(permission.NewRegisterRouter),
		fx.Invoke(constructor.RunHzService),
	)
}
