package main

import (
	"context"

	"github.com/KiClover/orzfrp/api/system/user"
	"github.com/KiClover/orzfrp/bootstrap"
	"github.com/KiClover/orzfrp/internal/constructor"
	"go.uber.org/fx"
)

func main() {
	bootstrap.NewApp(
		context.Background(),
		user.ServiceName,
		fx.Provide(constructor.NewHzService),
		fx.Invoke(user.NewRegisterRouter),
		fx.Invoke(constructor.RunHzService),
	)
}
