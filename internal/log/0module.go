package log

import "go.uber.org/fx"

func Module() fx.Option {
	return fx.Module(
		"internal.logger",
		fx.Provide(NewLogger),
	)
}
