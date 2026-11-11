package initialize_test

import (
	"testing"

	"github.com/KiClover/orzAdmin/api"
	"github.com/KiClover/orzAdmin/api/initialize"
	"github.com/KiClover/orzAdmin/conf"
	accesssvc "github.com/KiClover/orzAdmin/kitex_gen/system/access/systemaccessservice"
	messagesvc "github.com/KiClover/orzAdmin/kitex_gen/system/message/systemmessageservice"
	settingsvc "github.com/KiClover/orzAdmin/kitex_gen/system/setting/systemsettingservice"
	tokensvc "github.com/KiClover/orzAdmin/kitex_gen/system/token/systemtokenservice"
	usersvc "github.com/KiClover/orzAdmin/kitex_gen/system/user/systemuserservice"
	"github.com/casbin/casbin/v2"
	"go.uber.org/fx"
)

func TestSharedAPIModuleProvidesInitializationHandler(t *testing.T) {
	err := fx.ValidateApp(
		fx.NopLogger,
		api.Module(),
		fx.Supply(&conf.Config{}),
		fx.Provide(
			func() usersvc.Client { return nil },
			func() accesssvc.Client { return nil },
			func() settingsvc.Client { return nil },
			func() messagesvc.Client { return nil },
			func() tokensvc.Client { return nil },
			func() *casbin.Enforcer { return nil },
		),
		fx.Invoke(func(*initialize.HzInitializeService) {}),
	)
	if err != nil {
		t.Fatalf("initialization API is missing from shared wiring: %v", err)
	}
}
