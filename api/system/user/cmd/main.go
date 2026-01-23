package main

import (
	"context"

	"github.com/KiClover/orzAdmin/api/system/user"
	"github.com/KiClover/orzAdmin/bootstrap"
	"github.com/KiClover/orzAdmin/internal/constructor"
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
