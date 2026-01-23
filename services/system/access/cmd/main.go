package main

import (
	"context"

	"github.com/KiClover/orzAdmin/bootstrap"
	"github.com/KiClover/orzAdmin/internal/constructor"
	"github.com/KiClover/orzAdmin/kitex_gen/system/access/systemaccessservice"
	"github.com/KiClover/orzAdmin/services/system/access"
	"go.opentelemetry.io/otel"
	"go.uber.org/fx"
)

var tracer = otel.Tracer("github.com/KiClover/orzAdmin")

func main() {
	ctx, span := tracer.Start(context.Background(), "main")
	defer span.End()
	app := bootstrap.NewApp(
		ctx,
		access.ServiceName,
		fx.Invoke(constructor.RunService(systemaccessservice.NewServer)),
	)

	app.Run()
}
