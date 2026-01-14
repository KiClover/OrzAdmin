package setting

import (
	"context"

	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/kitex_gen/system/setting/systemsettingservice"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

const ServiceName = "api.system.setting"

type HzSystemSettingServiceP struct {
	fx.In
	Cfg           *conf.Config
	SettingClient systemsettingservice.Client
	Casbin        *casbin.Enforcer
	Redis         redis.UniversalClient
}
type HzSystemSettingService struct {
	Cfg           *conf.Config
	SettingClient systemsettingservice.Client
	Casbin        *casbin.Enforcer
	Redis         redis.UniversalClient
}

func NewHzSystemSettingService(p HzSystemSettingServiceP) *HzSystemSettingService {
	return &HzSystemSettingService{
		Cfg:           p.Cfg,
		SettingClient: p.SettingClient,
		Redis:         p.Redis,
		Casbin:        p.Casbin,
	}
}

// todo: Complete
func (s *HzSystemSettingService) SetRegUserRole(ctx context.Context, c *app.RequestContext) {

}

func (s *HzSystemSettingService) GetRegUserRole(ctx context.Context, c *app.RequestContext) {

}

func (s *HzSystemSettingService) SetEmailSetting(ctx context.Context, c *app.RequestContext) {

}

func (s *HzSystemSettingService) GetEmailSetting(ctx context.Context, c *app.RequestContext) {

}

func (s *HzSystemSettingService) SetSmsSetting(ctx context.Context, c *app.RequestContext) {

}

func (s *HzSystemSettingService) GetSmsSetting(ctx context.Context, c *app.RequestContext) {

}
