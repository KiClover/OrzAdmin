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
	reg.POST("/mobile", svc.RegisterByMobile)

	// 登录
	login := root.Group("/login")
	login.POST("/pwd", svc.LoginByPwd)
	login.POST("/code", svc.LoginByCode)

	// 获取验证码
	// Todo 写MessageService
	//getCode := root.Group("/code")
	//getCode.POST("/email", svc.SendEmailCode)
	//getCode.POST("/mobile", svc.SendMobileCode)
}
