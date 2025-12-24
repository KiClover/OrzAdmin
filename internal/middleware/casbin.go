package middleware

import (
	"context"
	"fmt"

	"github.com/KiClover/orzfrp/conf"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/sirupsen/logrus"
)

type CasbinMw struct {
	Cfg      *conf.Config
	Enforcer *casbin.Enforcer
}

func NewCasbinMw(cfg *conf.Config, enforcer *casbin.Enforcer) *CasbinMw {
	return &CasbinMw{
		Cfg:      cfg,
		Enforcer: enforcer,
	}
}

func (s *CasbinMw) UseCasbinMw(ctx context.Context, c *app.RequestContext) {
	path := string(c.Path())
	method := string(c.Request.Method())
	val, ok := c.Get("userId")
	if !ok {
		logrus.Errorf("get user id from context err:%v", c.Request)
		c.AbortWithStatusJSON(401, map[string]string{"error": "无法获取用户信息"})
		return
	}
	userIdFloat, ok := val.(float64)
	if !ok {
		c.AbortWithStatusJSON(401, map[string]string{"error": "用户ID格式错误"})
		return
	}
	userId := fmt.Sprintf("%.0f", userIdFloat)

	ok, err := s.Enforcer.Enforce(userId, path, method)
	if err != nil {
		logrus.Errorf("casbin enforce err:%v", err)
		c.AbortWithStatusJSON(500, map[string]string{"error": "权限验证失败"})
		return
	}

	if !ok {
		c.AbortWithStatusJSON(403, map[string]string{"error": "无权限访问"})
		return
	}
}
