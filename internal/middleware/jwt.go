package middleware

import (
	"context"
	"fmt"

	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/pkg/jwt"
	"github.com/cloudwego/hertz/pkg/app"
)

type HzJwtMw struct {
	Cfg *conf.Config
}

func NewJwtMw(cfg *conf.Config) *HzJwtMw {
	return &HzJwtMw{
		Cfg: cfg,
	}
}

func (s *HzJwtMw) UseJwtMw(ctx context.Context, c *app.RequestContext) {
	tokenStr := c.GetHeader("Authorization")
	if len(tokenStr) == 0 {
		c.AbortWithMsg("token is empty", 401)
		return
	}
	if len(tokenStr) > 7 && string(tokenStr[:7]) == "Bearer " {
		tokenStr = tokenStr[7:]
	}
	claims, err := jwt.ParseJwtToken(s.Cfg.AuthConf.AccessSecret, string(tokenStr))
	if err != nil {
		c.AbortWithMsg(fmt.Sprintf("parse token err:%v", err), 401)
		return
	}
	c.Set("userId", claims["userId"])
	c.Next(ctx)
}
