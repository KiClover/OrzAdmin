package system

import (
	"github.com/KiClover/orzfrp/api/system/auth"
	"go.uber.org/fx"
)

func Module() fx.Option {
	return fx.Module(
		"system.handler",
		fx.Provide(auth.NewHzSystemAuthService),
	)
}
