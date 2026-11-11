package permission

import (
	"github.com/KiClover/orzAdmin/internal/health"
	"github.com/KiClover/orzAdmin/internal/middleware"
	"github.com/cloudwego/hertz/pkg/app/server"
)

func NewRegisterRouter(h *server.Hertz, svc *HzSystemPermissionService) {

	root := h
	root.GET("/health", health.Health)

	// 加载中间件
	jwtMw := middleware.NewJwtMw(svc.Cfg)
	casbinMw := middleware.NewCasbinMw(svc.Cfg, svc.Casbin)

	// 数据库初始化统一由 api/initialize 提供，本服务不再注册 /init/database。

	// API
	api := root.Group("/api")
	api.Use(jwtMw.UseJwtMw)
	api.Use(casbinMw.UseCasbinMw)
	api.POST("/create", svc.CreateApi)
	api.POST("/list", svc.GetApiList)
	api.POST("/update", svc.UpdateApi)
	api.POST("/delete", svc.DeleteApi)

	// Role
	role := root.Group("/role")
	role.Use(jwtMw.UseJwtMw)
	role.Use(casbinMw.UseCasbinMw)
	role.POST("/create", svc.CreateRole)
	role.POST("/list", svc.GetRoleList)
	role.POST("/update", svc.UpdateRole)
	role.POST("/delete", svc.DeleteRole)

	// Casbin
	casbin := root.Group("/casbin")
	casbin.Use(jwtMw.UseJwtMw)
	casbin.Use(casbinMw.UseCasbinMw)
	casbin.POST("/role/update", svc.UpdateRoleCasbin)
	casbin.POST("/user/update", svc.UpdateUserCabin)
	casbin.POST("/role/get", svc.GetRoleCasbinRules)
	casbin.POST("/user/roles/get", svc.GetUserRoles)
	casbin.POST("/role/users/get", svc.GetRoleUsers)
	casbin.POST("/user/permissions/get", svc.GetUserPermissions)
}
