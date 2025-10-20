package bootstrap

import "go.uber.org/fx"

func AppEntry() fx.Option {
	return fx.Options(
		fx.Provide(),
	)
}
