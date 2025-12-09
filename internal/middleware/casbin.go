package middleware

import (
	"context"
	"fmt"

	"github.com/KiClover/orzfrp/conf"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
	casbin2 "github.com/hertz-contrib/casbin"
)

func NewCasbinMw(cfg *conf.Config, enforcer *casbin.Enforcer) *casbin2.Middleware {
	mw, err := casbin2.NewCasbinMiddlewareFromEnforcer(enforcer, LookupHandler)
	if err != nil {
		panic(fmt.Errorf("failed to create casbin middleware: %v", err))
	}
	return mw
}

func LookupHandler(ctx context.Context, c *app.RequestContext) string {
	r, exist := c.Get("role")
	if !exist {
		return ""
	}
	role, ok := r.(string)
	if !ok {
		return ""
	}
	return role
}
