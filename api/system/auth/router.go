package auth

import (
	"github.com/KiClover/orzAdmin/internal/health"
	"github.com/cloudwego/hertz/pkg/app/server"
)

func NewRegisterRouter(h *server.Hertz, svc *HzSystemAuthService) {

	root := h
	root.GET("/health", health.Health)

	// 数据库初始化统一由 api/initialize 提供（POST /init/services[/:service]），
	// 各业务 API 不再保留自己的 /init/database 入口。

	// 注册
	reg := root.Group("/reg")
	reg.POST("/email", svc.RegisterByEmail)
	reg.POST("/mobile", svc.RegisterByMobile)

	// 登录
	login := root.Group("/login")
	login.POST("/pwd", svc.LoginByPwd)
	login.POST("/code", svc.LoginByCode)
	login.POST("/find", svc.FindPwd)

	// 验证
	verify := root.Group("/verify")
	verify.POST("/code", svc.GetVerifyCode)
	verify.GET("/captcha", svc.GetCaptcha)
}
