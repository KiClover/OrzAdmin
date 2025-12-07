package main

import (
	"context"

	"github.com/KiClover/orzfrp/bootstrap"
	"github.com/KiClover/orzfrp/internal/constructor"
	user1 "github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"go.opentelemetry.io/otel"
	"go.uber.org/fx"
)

var tracer = otel.Tracer("github.com/KiClover/orzfrp")

const ServiceName = "system-user"

func main() {
	ctx, span := tracer.Start(context.Background(), "main")
	defer span.End()
	app := bootstrap.NewApp(
		ctx,
		ServiceName,
		fx.Invoke(constructor.RunService(user1.NewServer)),
	)

	app.Run()
}
