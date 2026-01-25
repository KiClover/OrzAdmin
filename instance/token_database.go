package instance

import (
	"context"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/sirupsen/logrus"
)

func NewTokenDatabase(ctx context.Context, conf *conf.Config) *ent.TokenClient {
	ctx, span := tracer.Start(ctx, "instance.NewDatabase")
	defer span.End()
	db := ent.NewClient(
		// logger
		ent.Log(logrus.Info),
		ent.Driver(conf.DatabaseConf.NewNoCacheDriver()),
	)
	return db.Token
}
