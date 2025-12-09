package auth

import (
	"context"
	"fmt"

	"github.com/KiClover/orzfrp/api/model/auth_api/system"
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/kitex_gen/base"
	"github.com/KiClover/orzfrp/kitex_gen/system/user"
	"github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzfrp/pkg/hresp"
	"github.com/KiClover/orzfrp/pkg/utils"
	user2 "github.com/KiClover/orzfrp/services/system/user"
	"github.com/cloudwego/hertz/pkg/app"
	"go.uber.org/fx"
)

const HzServiceName = "api.system.auth"

type HzSystemAuthServiceP struct {
	fx.In
	UserClient systemuserservice.Client
	Cfg        *conf.Config
}
type HzSystemAuthService struct {
	UserClient systemuserservice.Client
	Cfg        *conf.Config
}

func NewHzSystemAuthService(p HzSystemAuthServiceP) *HzSystemAuthService {
	return &HzSystemAuthService{
		UserClient: p.UserClient,
		Cfg:        p.Cfg,
	}
}

func (s *HzSystemAuthService) InitDatabase(ctx context.Context, c *app.RequestContext) {
	_, err := s.UserClient.InitDatabase(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}
	hresp.BaseSuccessMsg(c, "初始化数据库成功！")
	return
}

func (s *HzSystemAuthService) RegisterByEmail(ctx context.Context, c *app.RequestContext) {
	var req system.RegisterByEmail
	err := c.BindAndValidate(&req)
	if err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	// 检查是否已注册
	exist, err := s.UserClient.GetUserIsExist(ctx, &user.GetUserIsExistReq{
		Email: utils.GetPointer(req.Email),
	})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}
	if exist.IsExist {
		hresp.BaseErrMsg(c, 3, "该邮箱已注册")
		return
	}
	// todo: 邮箱验证码流程
	_, err = s.UserClient.CreateUser(ctx, &user.UserInfo{
		Email:    &req.Email,
		Password: &req.Password,
		Name:     &req.UserName,
	})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}
	hresp.BaseSuccessMsg(c, fmt.Sprintf("用户 %s 注册成功！", req.UserName))
	return
}

func (s *HzSystemAuthService) LoginByEmail(ctx context.Context, c *app.RequestContext) {
	var req system.LoginByEmailReq
	err := c.BindAndValidate(&req)
	if err != nil {
		hresp.ReqBindErr(c, err)
		return
	}

}
