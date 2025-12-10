package instance

import (
	"go.opentelemetry.io/otel"
	"go.uber.org/fx"
)

var tracer = otel.Tracer("github.com/KiClover/orzfrp")

func Module() fx.Option {
	return fx.Module(
		"instance",
		fx.Provide(NewDatabase),
		fx.Provide(NewCasbin),
		fx.Provide(NewRedis),
	)
}
