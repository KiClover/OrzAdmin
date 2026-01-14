package main

import (
	"context"

	"github.com/KiClover/orzfrp/api/system/permission"
	"github.com/KiClover/orzfrp/bootstrap"
	"github.com/KiClover/orzfrp/internal/constructor"
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
