package main

import (
	"context"

	"github.com/KiClover/orzAdmin/api/finance/wallet"
	"github.com/KiClover/orzAdmin/bootstrap"
	"github.com/KiClover/orzAdmin/internal/constructor"
	"go.uber.org/fx"
)

func main() {
	bootstrap.NewApp(
		context.Background(),
		wallet.ServiceName,
		fx.Provide(constructor.NewHzService),
		fx.Invoke(wallet.NewRegisterRouter),
		fx.Invoke(constructor.RunHzService),
	)
}
