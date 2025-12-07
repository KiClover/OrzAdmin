package auth

import "github.com/cloudwego/hertz/pkg/app/server"

func NewRegisterRouter(h *server.Hertz, svc *HzSystemAuthService) {
	r := h.Group("/auth")
	r.POST("/register/email", svc.RegisterByEmail)
}
