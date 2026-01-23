package main

import (
	"context"

	"github.com/KiClover/orzAdmin/bootstrap"
	"github.com/KiClover/orzAdmin/internal/constructor"
	user1 "github.com/KiClover/orzAdmin/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzAdmin/services/system/user"
	"go.opentelemetry.io/otel"
	"go.uber.org/fx"
)

var tracer = otel.Tracer("github.com/KiClover/orzAdmin")

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
