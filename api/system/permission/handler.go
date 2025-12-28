package permission

import (
	"context"

	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/kitex_gen/base"
	"github.com/KiClover/orzfrp/kitex_gen/system/access/systemaccessservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzfrp/pkg/hresp"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

const ServiceName = "api.system.permission"

type HzSystemPermissionServiceP struct {
	fx.In
	UserClient    systemuserservice.Client
	Cfg           *conf.Config
	Casbin        *casbin.Enforcer
	SettingClient systemsettingservice.Client
	Redis         redis.UniversalClient
	AccessClient  systemaccessservice.Client
}
type HzSystemPermissionService struct {
	UserClient    systemuserservice.Client
	Cfg           *conf.Config
	Casbin        *casbin.Enforcer
	SettingClient systemsettingservice.Client
	Redis         redis.UniversalClient
	AccessClient  systemaccessservice.Client
}

func NewHzSystemPermissionService(p HzSystemPermissionServiceP) *HzSystemPermissionService {
	return &HzSystemPermissionService{
		UserClient:    p.UserClient,
		Cfg:           p.Cfg,
		Casbin:        p.Casbin,
		SettingClient: p.SettingClient,
		Redis:         p.Redis,
	}
}

func (s *HzSystemPermissionService) InitDatabase(ctx context.Context, c *app.RequestContext) {
	_, err := s.AccessClient.InitDatabase(ctx, &base.Empty{})
	if err != nil {
		hresp.BaseErrMsg(c, 4, "init database failed", err)
		return
	}
}
