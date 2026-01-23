package main

import (
	"context"

	"github.com/KiClover/orzAdmin/bootstrap"
	"github.com/KiClover/orzAdmin/internal/constructor"
	"github.com/KiClover/orzAdmin/kitex_gen/system/message/systemmessageservice"
	"github.com/KiClover/orzAdmin/services/system/message"
	"go.opentelemetry.io/otel"
	"go.uber.org/fx"
)

var tracer = otel.Tracer("github.com/KiClover/orzAdmin")

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
