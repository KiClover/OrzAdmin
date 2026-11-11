package token

import (
	"github.com/KiClover/orzAdmin/internal/health"
	"github.com/KiClover/orzAdmin/internal/middleware"
	"github.com/cloudwego/hertz/pkg/app/server"
)

func NewRegisterRouter(h *server.Hertz, svc *HzSystemTokenService) {
	root := h
	root.GET("/health", health.Health)

	jwtMw := middleware.NewJwtMw(svc.Cfg)

	// 数据库初始化统一由 api/initialize 提供，本服务不再注册 /init/database。

	token := root.Group("/token")
	token.Use(jwtMw.UseJwtMw)
	token.POST("/get", svc.GetToken)
	token.POST("/refresh", svc.RefreshToken)
}
