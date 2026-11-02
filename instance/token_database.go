package instance

import (
	"context"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/ent"
)

func NewTokenDatabase(ctx context.Context, conf *conf.Config) *ent.TokenClient {
	ctx, span := tracer.Start(ctx, "instance.NewDatabase")
	defer span.End()
	db := ent.NewClient(
		// logger：同 dbLogger，SQL 调试日志带 [DB] 前缀
		ent.Log(func(args ...any) { dbLogger.Info(args...) }),
		ent.Driver(conf.DatabaseConf.NewNoCacheDriver()),
	)
	return db.Token
}
