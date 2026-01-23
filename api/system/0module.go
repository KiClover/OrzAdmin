package system

import (
	"github.com/KiClover/orzAdmin/api/system/auth"
	"github.com/KiClover/orzAdmin/api/system/permission"
	"github.com/KiClover/orzAdmin/api/system/setting"
	"github.com/KiClover/orzAdmin/api/system/user"
	"go.uber.org/fx"
)

func Module() fx.Option {
	return fx.Module(
		"system.handler",
		fx.Provide(auth.NewHzSystemAuthService),
		fx.Provide(user.NewHzSystemUserService),
		fx.Provide(permission.NewHzSystemPermissionService),
		fx.Provide(setting.NewHzSystemSettingService),
	)
}
