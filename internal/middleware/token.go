package middleware

import (
	"context"
	"fmt"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/ent"
	tokenEnt "github.com/KiClover/orzAdmin/ent/token"
	"github.com/cloudwego/hertz/pkg/app"
)

type TokenMw struct {
	Cfg *conf.Config
	DB  *ent.Client
}

func NewTokenMw(cfg *conf.Config, db *ent.Client) *TokenMw {
	return &TokenMw{
		Cfg: cfg,
		DB:  db,
	}
}

func (s *TokenMw) UseTokenMw(ctx context.Context, c *app.RequestContext) {
	tokenStr := c.GetHeader("SecretKey")
	if len(tokenStr) == 0 {
		c.AbortWithMsg("token is empty", 401)
		return
	}
	exist, err := s.DB.Token.Query().Where(tokenEnt.Token(string(tokenStr))).Exist(ctx)
	if err != nil {
		c.AbortWithMsg(fmt.Sprintf("query token err:%v", err), 401)
		return
	}
	if !exist {
		c.AbortWithMsg("token is invalid", 401)
		return
	}
	r, err := s.DB.Token.Query().Where(tokenEnt.Token(string(tokenStr))).Only(ctx)
	if err != nil {
		c.AbortWithMsg(fmt.Sprintf("query token err:%v", err), 401)
		return
	}
	c.Set("userId", r.UserID)
	c.Next(ctx)
}
