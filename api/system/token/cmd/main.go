package main

import (
	"context"

	"github.com/KiClover/orzAdmin/api/system/token"
	"github.com/KiClover/orzAdmin/bootstrap"
	"github.com/KiClover/orzAdmin/internal/constructor"
	"go.uber.org/fx"
)

func main() {
	bootstrap.NewApp(
		context.Background(),
		token.ServiceName,
		fx.Provide(constructor.NewHzService),
		fx.Invoke(token.NewRegisterRouter),
		fx.Invoke(constructor.RunHzService),
	)
}
