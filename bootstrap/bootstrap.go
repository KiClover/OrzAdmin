package bootstrap

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.uber.org/fx"

	internaltrace "github.com/KiClover/orzAdmin/internal/trace"
)

var tracer = otel.Tracer("github.com/KiClover/orzAdmin")

func NewApp(ctx context.Context, serviceName string, options ...fx.Option) *fx.App {
	ctx, span := tracer.Start(ctx, "bootstrap.NewApp")
	defer span.End()

	opts := []fx.Option{
		fx.Supply(fx.Annotate(ctx, fx.As(new(context.Context)))),
		fx.Supply(serviceName),
		fx.Supply(internaltrace.ServiceName(serviceName)),
	}
	opts = append(opts, AppEntries())
	opts = append(opts, options...)

	app := fx.New(opts...)

	return app
}
