package auth

import (
	"context"

	"github.com/KiClover/orzfrp/api/model/auth_api/system"
	"github.com/KiClover/orzfrp/kitex_gen/system/user"
	"github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"github.com/cloudwego/hertz/pkg/app"
	"go.uber.org/fx"
)

type HzSystemAuthServiceP struct {
	fx.In
	UserClient systemuserservice.Client
}
type HzSystemAuthService struct {
	UserClient systemuserservice.Client
}

func NewHzSystemAuthService(p HzSystemAuthServiceP) *HzSystemAuthService {
	return &HzSystemAuthService{
		UserClient: p.UserClient,
	}
}

func (s *HzSystemAuthService) RegisterByEmail(ctx context.Context, c *app.RequestContext) {
	var req system.RegisterByEmail
	err := c.BindAndValidate(&req)
	if err != nil {
		c.JSON(400, err.Error())
		return
	}
	// todo: 邮箱验证码流程
	r, err := s.UserClient.CreateUser(ctx, &user.UserInfo{
		Email:    &req.Email,
		Password: &req.Password,
	})
	if err != nil {
		c.JSON(500, err.Error())
		return
	}
	c.JSON(200, r)
}
