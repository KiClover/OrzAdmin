package bootstrap

import (
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/db"
	"github.com/KiClover/orzfrp/instance"
	"github.com/KiClover/orzfrp/internal/etcd"
	"github.com/KiClover/orzfrp/services"
	"go.uber.org/fx"
)

func AppEntries() fx.Option {
	return fx.Options(
		fx.Provide(conf.NewLoadConf),
		fx.Provide(instance.NewDatabase),
		etcd.Module(),
		db.Module(),
		// 微服务的svc
		services.Module(),
	)
}
