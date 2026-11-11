package setting

import (
	"github.com/KiClover/orzAdmin/internal/health"
	"github.com/KiClover/orzAdmin/internal/middleware"
	"github.com/cloudwego/hertz/pkg/app/server"
)

func NewRegisterRouter(h *server.Hertz, svc *HzSystemSettingService) {
	root := h
	root.GET("/health", health.Health)

	// 加载中间件
	jwtMw := middleware.NewJwtMw(svc.Cfg)
	casbinMw := middleware.NewCasbinMw(svc.Cfg, svc.Casbin)

	// 设置
	setting := root.Group("/setting")
	setting.Use(jwtMw.UseJwtMw)
	setting.Use(casbinMw.UseCasbinMw)
	setting.POST("/register/role", svc.SetRegUserRole)
	setting.POST("/email", svc.SetEmailSetting)
	setting.POST("/mobile", svc.SetSmsSetting)
	setting.GET("/register/role", svc.GetRegUserRole)
	setting.GET("/email", svc.GetEmailSetting)
	setting.GET("/mobile", svc.GetSmsSetting)
	// 钱包配置：管理视图读写，读写权限由 Casbin 的 (/setting/wallet, GET/PUT) 独立控制。
	setting.GET("/wallet", svc.GetWalletSetting)
	setting.PUT("/wallet", svc.UpdateWalletSetting)

}
