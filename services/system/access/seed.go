package access

import (
	"context"
	"github.com/KiClover/orzAdmin/ent"
	apiEnt "github.com/KiClover/orzAdmin/ent/api"
	roleEnt "github.com/KiClover/orzAdmin/ent/role"
)

// APISeed 描述一条注册到 access 服务的 HTTP 路由。
type APISeed struct {
	Path        string
	Method      string
	Group       string
	Description string
}

// RoleSeed 描述一个默认角色。
type RoleSeed struct {
	Code   string
	Name   string
	Remark string
}

// APISeeds 返回需要注册的路由。已存在的 (path, method) 不会被覆盖。
func APISeeds() []APISeed {
	return []APISeed{
		// 用户管理
		{Path: "/user/get/*", Method: "POST", Group: "user", Description: "user get"},
		{Path: "/user/create", Method: "POST", Group: "user", Description: "create user"},
		{Path: "/user/update", Method: "POST", Group: "user", Description: "update user"},
		{Path: "/user/delete", Method: "POST", Group: "user", Description: "delete user"},
		{Path: "/user/list", Method: "POST", Group: "user", Description: "user list"},

		// API 管理
		{Path: "/api/create", Method: "POST", Group: "permission", Description: "create api"},
		{Path: "/api/update", Method: "POST", Group: "permission", Description: "update api"},
		{Path: "/api/delete", Method: "POST", Group: "permission", Description: "delete api"},
		{Path: "/api/list", Method: "POST", Group: "permission", Description: "api list"},

		// 角色映射管理
		{Path: "/role/create", Method: "POST", Group: "permission", Description: "create role"},
		{Path: "/role/update", Method: "PUT", Group: "permission", Description: "update role"},
		{Path: "/role/delete", Method: "DELETE", Group: "permission", Description: "delete role"},
		{Path: "/role/list", Method: "POST", Group: "permission", Description: "role list"},

		// Casbin 管理
		{Path: "/casbin/role/update", Method: "POST", Group: "permission", Description: "update role casbin"},
		{Path: "/casbin/user/update", Method: "POST", Group: "permission", Description: "update user casbin"},
		{Path: "/casbin/role/get", Method: "POST", Group: "permission", Description: "get role casbin"},
		{Path: "/casbin/user/roles/get", Method: "POST", Group: "permission", Description: "get user roles"},
		{Path: "/casbin/role/users/get", Method: "POST", Group: "permission", Description: "get role users"},
		{Path: "/casbin/user/permissions/get", Method: "POST", Group: "permission", Description: "get user permissions"},

		// 钱包设置（任务 2 冻结；钱包管理员路由由任务 9 追加）
		{Path: "/setting/wallet", Method: "GET", Group: "setting", Description: "get wallet settings"},
		{Path: "/setting/wallet", Method: "PUT", Group: "setting", Description: "update wallet settings"},
	}
}

// RoleSeeds 返回默认角色。
func RoleSeeds() []RoleSeed {
	return []RoleSeed{
		{Code: "admin", Name: "管理员", Remark: "最高权限，勿动"},
		{Code: "user", Name: "普通用户", Remark: "默认普通用户"},
	}
}

// seedAccess 幂等补齐 API 与角色：只补缺失项，已存在的记录保持原值。
func (s SystemAccessService) seedAccess(ctx context.Context) error {
	for _, seed := range APISeeds() {
		exists, err := s.DB.Api.Query().
			Where(apiEnt.Path(seed.Path), apiEnt.Method(seed.Method)).
			Exist(ctx)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		_, err = s.DB.Api.Create().
			SetPath(seed.Path).
			SetMethod(seed.Method).
			SetGroup(seed.Group).
			SetDescription(seed.Description).
			Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				continue
			}
			return err
		}
	}

	for _, seed := range RoleSeeds() {
		exists, err := s.DB.Role.Query().Where(roleEnt.RoleCode(seed.Code)).Exist(ctx)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		_, err = s.DB.Role.Create().
			SetRoleCode(seed.Code).
			SetRoleName(seed.Name).
			SetRemark(seed.Remark).
			Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				continue
			}
			return err
		}
	}
	return nil
}
