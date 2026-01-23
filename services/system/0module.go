package system

import (
	"github.com/KiClover/orzAdmin/services/system/access"
	"github.com/KiClover/orzAdmin/services/system/message"
	"github.com/KiClover/orzAdmin/services/system/setting"
	"github.com/KiClover/orzAdmin/services/system/user"
	"go.uber.org/fx"
)

func Module() fx.Option {
	// 传递system服务的svc
	return fx.Options(
		fx.Provide(user.NewSystemUserService),
		fx.Provide(setting.NewSystemSettingService),
		fx.Provide(message.NewSystemMessageService),
		fx.Provide(access.NewSystemAccessService),
	)
}
