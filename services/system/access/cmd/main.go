package main

import (
	"context"

	"github.com/KiClover/orzfrp/bootstrap"
	"github.com/KiClover/orzfrp/internal/constructor"
	"github.com/KiClover/orzfrp/kitex_gen/system/access/systemaccessservice"
	"github.com/KiClover/orzfrp/services/system/access"
	"go.opentelemetry.io/otel"
	"go.uber.org/fx"
)

var tracer = otel.Tracer("github.com/KiClover/orzfrp")

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
