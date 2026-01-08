package user

import (
	"github.com/KiClover/orzfrp/internal/middleware"
	"github.com/cloudwego/hertz/pkg/app/server"
)

func NewRegisterRouter(h *server.Hertz, svc *HzSystemUserService) {

	root := h

	// 加载中间件
	jwtMw := middleware.NewJwtMw(svc.Cfg)
	casbinMw := middleware.NewCasbinMw(svc.Cfg, svc.Casbin)

	// 初始化数据库
	init := root.Group("/init")
	init.POST("/database", svc.InitDatabase)

	// 用户管理
	user := root.Group("/user")
	user.Use(jwtMw.UseJwtMw)
	user.Use(casbinMw.UseCasbinMw)
	user.POST("/create", svc.CreateUser)
}
