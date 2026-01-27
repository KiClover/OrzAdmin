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

	init := root.Group("/init")
	init.POST("/database", svc.InitDatabase)

	token := root.Group("/token")
	token.Use(jwtMw.UseJwtMw)
	token.POST("/get", svc.GetToken)
	token.POST("/refresh", svc.RefreshToken)
}
