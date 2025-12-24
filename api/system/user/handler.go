package user

import (
	"context"

	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/kitex_gen/system/message/systemmessageservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzfrp/pkg/hresp"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

const HzServiceName = "api.system.user"

type HzSystemUserServiceP struct {
	fx.In
	UserClient    systemuserservice.Client
	Cfg           *conf.Config
	Casbin        *casbin.Enforcer
	SettingClient systemsettingservice.Client
	Redis         redis.UniversalClient
	MsgClient     systemmessageservice.Client
}
type HzSystemUserService struct {
	UserClient    systemuserservice.Client
	Cfg           *conf.Config
	Casbin        *casbin.Enforcer
	SettingClient systemsettingservice.Client
	Redis         redis.UniversalClient
	MsgClient     systemmessageservice.Client
}

func NewHzSystemUserService(p HzSystemUserServiceP) *HzSystemUserService {
	return &HzSystemUserService{
		UserClient:    p.UserClient,
		Cfg:           p.Cfg,
		Casbin:        p.Casbin,
		SettingClient: p.SettingClient,
		Redis:         p.Redis,
		MsgClient:     p.MsgClient,
	}
}

func (s *HzSystemUserService) InitDatabase(ctx context.Context, c *app.RequestContext) {
	_, err := s.Casbin.AddPolicy("admin", "/user/*", "GET")
	if err != nil {
		hresp.BaseErrMsg(c, 4, "init database failed", err)
		return
	}
	_, err = s.Casbin.AddPolicy("admin", "/user", "POST")
	if err != nil {
		hresp.BaseErrMsg(c, 4, "init database failed", err)
		return
	}
	_, err = s.Casbin.AddPolicy("admin", "/user/*", "PUT")
	if err != nil {
		hresp.BaseErrMsg(c, 4, "init database failed", err)
		return
	}
	_, err = s.Casbin.AddPolicy("admin", "/user/*", "DELETE")
	if err != nil {
		hresp.BaseErrMsg(c, 4, "init database failed", err)
		return
	}
	hresp.BaseSuccessMsg(c, "init database success")
}

func (s *HzSystemUserService) CreateUser(ctx context.Context, c *app.RequestContext) {

}
