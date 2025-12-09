package bootstrap

import (
	system2 "github.com/KiClover/orzfrp/api/system"
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/db"
	"github.com/KiClover/orzfrp/instance"
	"github.com/KiClover/orzfrp/internal/client"
	"github.com/KiClover/orzfrp/internal/etcd"
	"github.com/KiClover/orzfrp/services"
	"go.uber.org/fx"
)

func AppEntries() fx.Option {
	return fx.Options(
		fx.Provide(conf.NewLoadConf),
		instance.Module(),
		etcd.Module(),
		db.Module(),
		// 微服务的svc
		services.Module(),
		// 微服务的Client
		client.Module(),

		system2.Module(),
	)
}
