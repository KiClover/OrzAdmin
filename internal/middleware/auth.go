package middleware

import (
	"context"
	"fmt"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/pkg/jwt"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
)

type HzAuthMw struct {
	Cfg      *conf.Config
	Enforcer *casbin.Enforcer
}

func NewAuthMw(cfg *conf.Config, enforcer *casbin.Enforcer) *HzAuthMw {
	return &HzAuthMw{
		Cfg:      cfg,
		Enforcer: enforcer,
	}
}

func (s *HzAuthMw) AuthMw(ctx context.Context, c *app.RequestContext) {
	// JWT校验
	tokenStr := c.GetHeader("Authorization")
	if len(tokenStr) == 0 {
		c.AbortWithMsg("token is empty", 401)
	}
	if len(tokenStr) > 7 && string(tokenStr[:7]) == "Bearer " {
		tokenStr = tokenStr[7:]
	}
	claims, err := jwt.ParseJwtToken(s.Cfg.AuthConf.AccessSecret, string(tokenStr))
	if err != nil {
		c.AbortWithMsg("token is invalid", 401)
	}
	uid, ok := anyToUint64(claims["userId"])
	if !ok {
		c.AbortWithMsg("invalid user id", 401)
		return
	}
	c.Set("userId", uid)

	path := c.Path()
	method := c.Request.Method
	uidStr := fmt.Sprintf("%d", uid)

	ok, err = s.Enforcer.Enforce(uidStr, path, method)
	if err != nil {
		c.AbortWithStatusJSON(500, map[string]string{"error": "权限验证失败"})
		return
	}

	if !ok {
		c.AbortWithStatusJSON(403, map[string]string{"error": "无权限访问"})
		return
	}
	c.Next(ctx)
}
