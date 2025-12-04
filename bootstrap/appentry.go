package bootstrap

import (
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/instance"
	"go.uber.org/fx"
)

func AppEntries() fx.Option {
	return fx.Options(
		fx.Invoke(conf.NewLoadConf),
		fx.Provide(instance.NewDatabase),
	)
}
