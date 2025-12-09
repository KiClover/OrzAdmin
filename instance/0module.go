package instance

import "go.uber.org/fx"

func Module() fx.Option {
	return fx.Module(
		"instance",
		fx.Provide(NewDatabase),
		fx.Provide(NewCasbin),
	)
}
