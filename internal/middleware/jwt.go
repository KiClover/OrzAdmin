package middleware

import (
	"context"

	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/pkg/jwt"
	"github.com/cloudwego/hertz/pkg/app"
)

type HzAuthMw struct {
	Cfg *conf.Config
}

func (s *HzAuthMw) AuthMw(ctx context.Context, c *app.RequestContext) {
	tokenStr := c.GetHeader("Authorization")
	if len(tokenStr) == 0 {
		c.AbortWithMsg("token is empty", 401)
	}
	if len(tokenStr) > 7 && string(tokenStr[:7]) == "Bearer " {
		tokenStr = tokenStr[7:]
	}
	claims, err := jwt.ParseJwtToken(string(tokenStr), s.Cfg.AuthConf.AccessSecret)
	if err != nil {
		c.AbortWithMsg("token is invalid", 401)
	}
	c.Set("userId", claims["userId"])
	c.Next(ctx)
}
