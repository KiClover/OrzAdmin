package system

import (
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/services/system/access"
	"github.com/KiClover/orzAdmin/services/system/message"
	"github.com/KiClover/orzAdmin/services/system/setting"
	"github.com/KiClover/orzAdmin/services/system/token"
	"github.com/KiClover/orzAdmin/services/system/user"
	"go.uber.org/fx"
)

func Module() fx.Option {
	// 传递system服务的svc
	return fx.Options(
		// Creem Profile 远程校验器以接口形式提供，供 Setting 服务注入。
		fx.Provide(func(cfg *conf.Config) setting.WalletProfileValidator {
			return setting.NewCreemProfileValidator(cfg)
		}),
		fx.Provide(user.NewSystemUserService),
		fx.Provide(setting.NewSystemSettingService),
		fx.Provide(message.NewSystemMessageService),
		fx.Provide(access.NewSystemAccessService),
		fx.Provide(token.NewSystemTokenService),
	)
}
