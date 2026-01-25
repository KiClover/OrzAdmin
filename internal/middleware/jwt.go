package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/pkg/jwt"
	"github.com/cloudwego/hertz/pkg/app"
)

func anyToUint64(v interface{}) (uint64, bool) {
	switch t := v.(type) {
	case uint64:
		return t, true
	case uint:
		return uint64(t), true
	case int:
		if t < 0 {
			return 0, false
		}
		return uint64(t), true
	case int64:
		if t < 0 {
			return 0, false
		}
		return uint64(t), true
	case float64:
		if t < 0 {
			return 0, false
		}
		return uint64(t), true
	case json.Number:
		i, err := t.Int64()
		if err != nil || i < 0 {
			return 0, false
		}
		return uint64(i), true
	case string:
		u, err := strconv.ParseUint(t, 10, 64)
		if err != nil {
			return 0, false
		}
		return u, true
	default:
		return 0, false
	}
}

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
	uid, ok := anyToUint64(claims["userId"])
	if !ok {
		c.AbortWithMsg("invalid user id", 401)
		return
	}
	c.Set("userId", uid)
	c.Next(ctx)
}
