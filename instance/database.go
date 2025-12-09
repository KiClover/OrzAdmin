package instance

import (
	"context"

	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/ent"
	_ "github.com/KiClover/orzfrp/ent/runtime"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("github.com/KiClover/orzfrp")

func NewDatabase(ctx context.Context, conf *conf.Config) *ent.Client {
	ctx, span := tracer.Start(ctx, "instance.NewDatabase")
	defer span.End()
	db := ent.NewClient(
		// logger
		ent.Log(logrus.Info),
		ent.Driver(conf.DatabaseConf.NewNoCacheDriver()),
	)
	return db
}
