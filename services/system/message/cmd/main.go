package main

import (
	"context"

	"github.com/KiClover/orzfrp/bootstrap"
	"github.com/KiClover/orzfrp/internal/constructor"
	"github.com/KiClover/orzfrp/kitex_gen/system/message/systemmessageservice"
	"github.com/KiClover/orzfrp/services/system/message"
	"go.opentelemetry.io/otel"
	"go.uber.org/fx"
)

var tracer = otel.Tracer("github.com/KiClover/orzfrp")

func main() {
	ctx, span := tracer.Start(context.Background(), "main")
	defer span.End()
	app := bootstrap.NewApp(
		ctx,
		message.ServiceName,
		fx.Invoke(constructor.RunService(systemmessageservice.NewServer)),
	)

	app.Run()
}
