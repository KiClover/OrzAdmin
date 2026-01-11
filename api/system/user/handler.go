package user

import (
	"context"
	"strconv"

	userapi "github.com/KiClover/orzfrp/api/model/system/user"
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/kitex_gen/system/message/systemmessageservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/user"
	"github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzfrp/pkg/hresp"
	"github.com/KiClover/orzfrp/pkg/utils"
	user2 "github.com/KiClover/orzfrp/services/system/user"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

const ServiceName = "api.system.user"

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
	hresp.BaseSuccessMsg(c, "init database")
}

func (s *HzSystemUserService) CreateUser(ctx context.Context, c *app.RequestContext) {
	var req userapi.UserInfoReq
	if err := c.BindAndValidate(req); err != nil {
		hresp.BaseErrMsg(c, 4, "bind and validate failed", err)
	}
	re, err := s.UserClient.CreateUser(ctx, &user.UserInfo{
		Status:      utils.GetPointer(uint32(*req.Status)),
		Name:        req.Name,
		Password:    req.Password,
		Description: req.Description,
		Mobile:      req.Mobile,
		Email:       req.Email,
		Avatar:      req.Avatar,
		Gender:      utils.GetPointer(uint32(*req.Gender)),
	})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
	}
	// 赋权
	for _, r := range req.RoleCodes {
		_, err = s.Casbin.AddRoleForUser(strconv.FormatInt(re.Id, 10), r)
		if err != nil {
			hresp.BaseErrMsg(c, 1, "add roles failed", err)
		}
	}
	hresp.BaseSuccessMsgWithBody(c, "create user success", map[string]interface{}{
		"id": re.Id,
	})
}
