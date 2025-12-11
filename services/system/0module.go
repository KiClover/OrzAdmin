package system

import (
	"github.com/KiClover/orzfrp/services/system/setting"
	"github.com/KiClover/orzfrp/services/system/user"
	"go.uber.org/fx"
)

func Module() fx.Option {
	// 传递system服务的svc
	return fx.Options(
		fx.Provide(user.NewSystemUserService),
		fx.Provide(setting.NewSystemSettingService),
	)
}
