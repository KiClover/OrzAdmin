package access

import (
	"context"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/migrate"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/access"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

const ServiceName = "system-access"

type SystemAccessService struct {
	fx.In
	Cfg   *conf.Config
	DB    *db.AccessServiceDB
	DBAll *ent.Client
}

var _ access.SystemAccessService = (*SystemAccessService)(nil)

func NewSystemAccessService(svc SystemAccessService) access.SystemAccessService {
	return &svc
}

func (s SystemAccessService) InitDatabase(ctx context.Context, req *base.Empty) (res *base.BaseResp, err error) {
	tables := []*schema.Table{migrate.ApisTable, migrate.RolesTable}
	if err := migrate.Create(ctx, s.DBAll.Schema, tables, schema.WithForeignKeys(false)); err != nil {
		logrus.Errorf("init database err: %v", err)
		return nil, err
	}
	apis := []*ent.APICreate{
		// 用户管理
		s.DB.Api.Create().SetPath("/user/get/*").SetMethod("POST").SetGroup("user").SetDescription("user get"),
		s.DB.Api.Create().SetPath("/user/create").SetMethod("POST").SetGroup("user").SetDescription("create user"),
		s.DB.Api.Create().SetPath("/user/update").SetMethod("POST").SetGroup("user").SetDescription("update user"),
		s.DB.Api.Create().SetPath("/user/delete").SetMethod("POST").SetGroup("user").SetDescription("delete user"),
		s.DB.Api.Create().SetPath("/user/list").SetMethod("POST").SetGroup("user").SetDescription("user list"),

		// API管理
		s.DB.Api.Create().SetPath("/api/create").SetMethod("POST").SetGroup("permission").SetDescription("create api"),
		s.DB.Api.Create().SetPath("/api/update").SetMethod("POST").SetGroup("permission").SetDescription("update api"),
		s.DB.Api.Create().SetPath("/api/delete").SetMethod("POST").SetGroup("permission").SetDescription("delete api"),
		s.DB.Api.Create().SetPath("/api/list").SetMethod("POST").SetGroup("permission").SetDescription("api list"),

		// 角色映射管理
		s.DB.Api.Create().SetPath("/role/create").SetMethod("POST").SetGroup("permission").SetDescription("create role"),
		s.DB.Api.Create().SetPath("/role/update").SetMethod("PUT").SetGroup("permission").SetDescription("update role"),
		s.DB.Api.Create().SetPath("/role/delete").SetMethod("DELETE").SetGroup("permission").SetDescription("delete role"),
		s.DB.Api.Create().SetPath("/role/list").SetMethod("POST").SetGroup("permission").SetDescription("role list"),

		// Casbin管理
		s.DB.Api.Create().SetPath("/casbin/role/update").SetMethod("POST").SetGroup("permission").SetDescription("update role casbin"),
		s.DB.Api.Create().SetPath("/casbin/user/update").SetMethod("POST").SetGroup("permission").SetDescription("update user casbin"),
		s.DB.Api.Create().SetPath("/casbin/role/get").SetMethod("POST").SetGroup("permission").SetDescription("get role casbin"),
		s.DB.Api.Create().SetPath("/casbin/user/roles/get").SetMethod("POST").SetGroup("permission").SetDescription("get user roles"),
		s.DB.Api.Create().SetPath("/casbin/role/users/get").SetMethod("POST").SetGroup("permission").SetDescription("get role users"),
		s.DB.Api.Create().SetPath("/casbin/user/permissions/get").SetMethod("POST").SetGroup("permission").SetDescription("get user permissions"),
	}
	_, err = s.DB.Api.CreateBulk(apis...).Save(ctx)
	if err != nil && !ent.IsConstraintError(err) {
		logrus.Errorf("init database seed apis err: %v", err)
		return nil, err
	}

	roles := []*ent.RoleCreate{
		s.DB.Role.Create().SetRoleCode("admin").SetRoleName("管理员").SetRemark("最高权限，勿动"),
		s.DB.Role.Create().SetRoleCode("user").SetRoleName("普通用户").SetRemark("默认普通用户"),
	}
	_, err = s.DB.Role.CreateBulk(roles...).Save(ctx)
	if err != nil && !ent.IsConstraintError(err) {
		logrus.Errorf("init database seed roles err: %v", err)
		return nil, err
	}

	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemAccessService) CreateApi(ctx context.Context, req *access.ApiInfo) (res *base.BaseResp, err error) {
	_, err = s.DB.Api.Create().
		SetPath(*req.Path).
		SetMethod(*req.Method).
		SetGroup(*req.Group).
		SetDescription(*req.Description).
		Save(ctx)
	if err != nil {
		logrus.Errorf("create api err: %v", err)
		return nil, err
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemAccessService) UpdateApi(ctx context.Context, req *access.ApiInfo) (res *base.BaseResp, err error) {
	_, err = s.DB.Api.UpdateOneID(*req.Id).
		SetPath(*req.Path).
		SetMethod(*req.Method).
		SetGroup(*req.Group).
		SetDescription(*req.Description).
		Save(ctx)
	if err != nil {
		logrus.Errorf("create api err: %v", err)
		return nil, err
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemAccessService) DeleteApi(ctx context.Context, req *base.IDsReq) (res *base.BaseResp, err error) {
	for _, id := range req.Ids {
		err = s.DB.Api.DeleteOneID(id).Exec(ctx)
		if err != nil {
			logrus.Errorf("delete api err: %v", err)
			return nil, err
		}
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemAccessService) GetApiList(ctx context.Context, req *base.Empty) (res *access.ApiInfoListResp, err error) {
	data, err := s.DB.Api.Query().All(ctx)
	if err != nil {
		logrus.Errorf("get api list err: %v", err)
		return nil, err
	}
	var list []*access.ApiInfo
	for _, v := range data {
		list = append(list, &access.ApiInfo{
			Id:          &v.ID,
			Path:        &v.Path,
			Method:      &v.Method,
			Group:       &v.Group,
			Description: &v.Description,
		})
	}
	return &access.ApiInfoListResp{
		List: list,
	}, nil
}

func (s SystemAccessService) CreateRole(ctx context.Context, req *access.RoleInfo) (res *base.BaseResp, err error) {
	_, err = s.DB.Role.Create().
		SetRoleCode(*req.RoleCode).
		SetRoleName(*req.RoleName).
		SetRemark(*req.Remark).
		Save(ctx)
	if err != nil {
		logrus.Errorf("create role err: %v", err)
		return nil, err
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemAccessService) UpdateRole(ctx context.Context, req *access.RoleInfo) (res *base.BaseResp, err error) {
	_, err = s.DB.Role.UpdateOneID(*req.Id).
		SetRoleName(*req.RoleName).
		SetRemark(*req.Remark).
		Save(ctx)
	// 不允许修改RoleCode
	if err != nil {
		logrus.Errorf("update role err: %v", err)
		return nil, err
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemAccessService) DeleteRole(ctx context.Context, req *base.IDsReq) (res *base.BaseResp, err error) {
	for _, id := range req.Ids {
		err = s.DB.Role.DeleteOneID(id).Exec(ctx)
		if err != nil {
			logrus.Errorf("delete role err: %v", err)
			return nil, err
		}
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemAccessService) GetRoleList(ctx context.Context, req *base.Empty) (res *access.RoleInfoListResp, err error) {
	data, err := s.DB.Role.Query().All(ctx)
	if err != nil {
		logrus.Errorf("get api list err: %v", err)
		return nil, err
	}
	var list []*access.RoleInfo
	for _, v := range data {
		list = append(list, &access.RoleInfo{
			Id:       &v.ID,
			RoleCode: &v.RoleCode,
			RoleName: &v.RoleName,
			Remark:   &v.Remark,
		})
	}
	return &access.RoleInfoListResp{
		List: list,
	}, nil
}
