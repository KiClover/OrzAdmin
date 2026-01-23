package bootstrap

import (
	system2 "github.com/KiClover/orzAdmin/api/system"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/instance"
	"github.com/KiClover/orzAdmin/internal/client"
	"github.com/KiClover/orzAdmin/internal/etcd"
	"github.com/KiClover/orzAdmin/services"
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
