package instance

import (
	"context"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/ent"
	_ "github.com/KiClover/orzAdmin/ent/runtime"
	internallog "github.com/KiClover/orzAdmin/internal/log"
)

// dbLogger 让 ent 的 SQL 调试日志统一带 [DB] 前缀。
var dbLogger = internallog.WithModule(internallog.ModuleDB)

func NewDatabase(ctx context.Context, conf *conf.Config) *ent.Client {
	ctx, span := tracer.Start(ctx, "instance.NewDatabase")
	defer span.End()
	db := ent.NewClient(
		// logger：ent 以 fmt.Sprint 语义调用 fn，转发到带模块前缀的 logger
		ent.Log(func(args ...any) { dbLogger.Info(args...) }),
		ent.Driver(conf.DatabaseConf.NewNoCacheDriver()),
	)
	return db
}
