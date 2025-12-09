package auth

import "github.com/cloudwego/hertz/pkg/app/server"

func NewRegisterRouter(h *server.Hertz, svc *HzSystemAuthService) {

	root := h.Group("/auth")

	// 初始化数据库
	init := root.Group("/init")
	init.POST("/database", svc.InitDatabase)

	// 注册
	reg := root.Group("/reg")
	reg.POST("/email", svc.RegisterByEmail)

	// 登录
	login := root.Group("/login")
	login.POST("/pwd", svc.LoginByPwd)

}
