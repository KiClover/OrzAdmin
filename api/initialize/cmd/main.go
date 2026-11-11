package main

import (
	"context"

	"github.com/KiClover/orzAdmin/api/initialize"
	"github.com/KiClover/orzAdmin/bootstrap"
	"github.com/KiClover/orzAdmin/internal/constructor"
	"go.uber.org/fx"
)

func main() {
	bootstrap.NewApp(
		context.Background(),
		initialize.ServiceName,
		fx.Provide(constructor.NewHzService),
		fx.Invoke(initialize.NewRegisterRouter),
		fx.Invoke(constructor.RunHzService),
	)
}
