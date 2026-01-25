package system

import (
	"go.uber.org/fx"
)

func Module() fx.Option {
	return fx.Options(
		fx.Provide(NewUserServiceClient),
		fx.Provide(NewSettingServiceClient),
		fx.Provide(NewMessageServiceClient),
		fx.Provide(NewAccessServiceClient),
		fx.Provide(NewClientServiceClient),
	)
}
