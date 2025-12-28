package permission

import (
	"github.com/KiClover/orzfrp/internal/middleware"
	"github.com/cloudwego/hertz/pkg/app/server"
)

func NewRegisterRouter(h *server.Hertz, svc *HzSystemPermissionService) {

	root := h

	// 加载中间件
	jwtMw := middleware.NewJwtMw(svc.Cfg)
	casbinMw := middleware.NewCasbinMw(svc.Cfg, svc.Casbin)
	root.Use(jwtMw.UseJwtMw)

	// 初始化数据库
	init := root.Group("/init")
	init.POST("/database")

	// API
	api := root.Group("/api")
	api.Use(casbinMw.UseCasbinMw)
	api.POST("")
	api.GET("/:id")
	api.PUT("")
	api.DELETE("/:id")
}
