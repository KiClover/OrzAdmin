package bootstrap

import (
	"github.com/KiClover/orzAdmin/api"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/instance"
	"github.com/KiClover/orzAdmin/internal/client"
	"github.com/KiClover/orzAdmin/internal/discovery"
	internallog "github.com/KiClover/orzAdmin/internal/log"
	"github.com/KiClover/orzAdmin/services"
	"go.uber.org/fx"
)

func AppEntries() fx.Option {
	return fx.Options(
		fx.Provide(conf.NewLoadConf),
		// 统一日志：消费 *conf.Config 产出标准 Logger，并接管 Fx 事件
		internallog.Module(),
		fx.WithLogger(internallog.NewFxEventLogger),
		instance.Module(),
		discovery.Module(),
		db.Module(),
		// 微服务的svc
		services.Module(),
		// 微服务的Client
		client.Module(),

		api.Module(),
	)
}
