package instance

import (
	"context"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/ent"
	_ "github.com/KiClover/orzAdmin/ent/runtime"
	"github.com/sirupsen/logrus"
)

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
