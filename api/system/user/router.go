package user

import (
	"github.com/KiClover/orzAdmin/internal/health"
	"github.com/KiClover/orzAdmin/internal/middleware"
	"github.com/cloudwego/hertz/pkg/app/server"
)

func NewRegisterRouter(h *server.Hertz, svc *HzSystemUserService) {

	root := h
	root.GET("/health", health.Health)

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
	user.POST("/update", svc.UpdateUser)
	user.POST("/delete", svc.DeleteUser)
	user.POST("/list", svc.GetUserList)
	user.GET("/get/:id", svc.GetUserById)

	// 用户信息管理
	userSide := root.Group("/info")
	userSide.Use(jwtMw.UseJwtMw)
	userSide.POST("/profile", svc.GetUserProfile)
	userSide.POST("/update", svc.UpdateUserInfo)
	userSide.POST("/change-pwd", svc.ChangePwd)

	baseInfo := root.Group("/base")
	baseInfo.POST("/:id", svc.GetUserBasicInfoById)
}
