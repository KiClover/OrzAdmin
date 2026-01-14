package setting

import (
	"github.com/KiClover/orzfrp/internal/middleware"
	"github.com/cloudwego/hertz/pkg/app/server"
)

func NewRegisterRouter(h *server.Hertz, svc *HzSystemSettingService) {
	root := h

	// 加载中间件
	jwtMw := middleware.NewJwtMw(svc.Cfg)
	casbinMw := middleware.NewCasbinMw(svc.Cfg, svc.Casbin)

	// 设置
	setting := root.Group("/setting")
	setting.Use(jwtMw.UseJwtMw)
	setting.Use(casbinMw.UseCasbinMw)
	setting.POST("/register/role", svc.SetRegUserRole)
	setting.POST("/email", svc.SetEmailSetting)
	setting.POST("/mobile", svc.SetSMSSetting)
	setting.GET("/register/role", svc.GetRegUserRole)
	setting.GET("/email", svc.GetEmailSetting)
	setting.GET("/mobile", svc.GetSMSSetting)

}
