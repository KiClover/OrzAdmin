package main

import (
	"context"

	"github.com/KiClover/orzfrp/bootstrap"
	"github.com/KiClover/orzfrp/internal/constructor"
	user1 "github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzfrp/services/system/user"
	"go.opentelemetry.io/otel"
	"go.uber.org/fx"
)

var tracer = otel.Tracer("github.com/KiClover/orzfrp")

func main() {
	ctx, span := tracer.Start(context.Background(), "main")
	defer span.End()
	app := bootstrap.NewApp(
		ctx,
		user.ServiceName,
		fx.Invoke(constructor.RunService(user1.NewServer)),
	)

	app.Run()
}
