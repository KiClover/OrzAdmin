package initialize

import (
	"github.com/KiClover/orzAdmin/internal/health"
	"github.com/cloudwego/hertz/pkg/app/server"
)

// NewRegisterRouter 注册初始化 API 的全部路由。
//
// 初始化 API 只接受本机或受信内网访问，不挂 JWT / Casbin，也不接受初始化密钥；
// 旧的多合一 /init/database 路由刻意不保留。
func NewRegisterRouter(h *server.Hertz, svc *HzInitializeService) {
	root := h.Group("/")
	root.GET("/health", health.Health)

	initGroup := root.Group("/init")
	initGroup.POST("/services", svc.InitAll)
	initGroup.POST("/services/:service", svc.InitService)
}
