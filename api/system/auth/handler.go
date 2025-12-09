package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/KiClover/orzfrp/api/model/auth_api/system"
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/kitex_gen/base"
	"github.com/KiClover/orzfrp/kitex_gen/system/user"
	"github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzfrp/pkg/hresp"
	"github.com/KiClover/orzfrp/pkg/jwt"
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

func (s *HzSystemAuthService) LoginByPwd(ctx context.Context, c *app.RequestContext) {
	var req system.LoginByPwdReq
	err := c.BindAndValidate(&req)
	if err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	var userInfo *user.UserInfo

	if req.Type == "email" {
		e, err := s.UserClient.GetUserIsExist(ctx,
			&user.GetUserIsExistReq{
				Email: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
		if !e.IsExist {
			hresp.BaseErrMsg(c, 3, "该邮箱未注册")
			return
		}
		userInfo, err = s.UserClient.GetUserByEmail(ctx,
			&user.GetUserByEmailReq{
				Email: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
	} else if req.Type == "mobile" {
		m, err := s.UserClient.GetUserIsExist(ctx,
			&user.GetUserIsExistReq{
				Mobile: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
		if !m.IsExist {
			hresp.BaseErrMsg(c, 3, "该手机号未注册")
			return
		}
		userInfo, err = s.UserClient.GetUserByMobile(ctx,
			&user.GetUserByMobileReq{
				Mobile: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
	} else {
		hresp.BaseErrMsg(c, 3, "登录方式错误")
		return
	}

	v := utils.CheckPasswordHash(req.Password, *userInfo.Password)
	if v != nil {
		hresp.BaseErrMsg(c, 3, "密码错误")
		return
	}

	token, err := jwt.NewJwtToken(s.Cfg.AuthConf.AccessSecret, time.Now().Unix(), s.Cfg.AuthConf.AccessExpire,
		jwt.WithOption("userId", userInfo.Id),
		jwt.WithOption("roleId", 1),
	)
	if err != nil {
		hresp.BaseErrMsg(c, 4, "token签发失败")
		return
	}
	hresp.BaseSuccessMsgWithBody(c, "登录成功！", system.LoginResp{
		Token:        token,
		RefreshToken: "",
	})

}
