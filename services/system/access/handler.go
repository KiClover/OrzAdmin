package access

import (
	"context"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/db"
	"github.com/KiClover/orzfrp/ent"
	"github.com/KiClover/orzfrp/ent/migrate"
	"github.com/KiClover/orzfrp/kitex_gen/base"
	"github.com/KiClover/orzfrp/kitex_gen/system/access"
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
		s.DB.Api.Create().SetPath("/user/*").SetMethod("GET").SetGroup("user").SetDescription("user get"),
		s.DB.Api.Create().SetPath("/user").SetMethod("POST").SetGroup("user").SetDescription("create user"),
		s.DB.Api.Create().SetPath("/user/*").SetMethod("PUT").SetGroup("user").SetDescription("update user"),
		s.DB.Api.Create().SetPath("/user/*").SetMethod("DELETE").SetGroup("user").SetDescription("delete user"),
		s.DB.Api.Create().SetPath("/user/list").SetMethod("POST").SetGroup("user").SetDescription("user list"),
		// 角色映射管理
		s.DB.Api.Create().SetPath("/role/*").SetMethod("GET").SetGroup("access").SetDescription("role get"),
		s.DB.Api.Create().SetPath("/role").SetMethod("POST").SetGroup("access").SetDescription("create role"),
		s.DB.Api.Create().SetPath("/role/*").SetMethod("PUT").SetGroup("access").SetDescription("update role"),
		s.DB.Api.Create().SetPath("/role/*").SetMethod("DELETE").SetGroup("access").SetDescription("delete role"),
		s.DB.Api.Create().SetPath("/role/list").SetMethod("POST").SetGroup("access").SetDescription("role list"),
	}
	_, err = s.DB.Api.CreateBulk(apis...).Save(ctx)
	if err != nil && !ent.IsConstraintError(err) {
		logrus.Errorf("init database seed apis err: %v", err)
		return nil, err
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemAccessService) CreateApi(ctx context.Context, req *access.ApiInfo) (res *base.BaseResp, err error) {
	//TODO implement me
	panic("implement me")
}

func (s SystemAccessService) UpdateApi(ctx context.Context, req *access.ApiInfo) (res *base.BaseResp, err error) {
	//TODO implement me
	panic("implement me")
}

func (s SystemAccessService) DeleteApi(ctx context.Context, req *base.IDsReq) (res *base.BaseResp, err error) {
	//TODO implement me
	panic("implement me")
}

func (s SystemAccessService) GetApiList(ctx context.Context, req *base.Empty) (res *access.ApiInfoListResp, err error) {
	//TODO implement me
	panic("implement me")
}

func (s SystemAccessService) CreateRole(ctx context.Context, req *access.RoleInfo) (res *base.BaseResp, err error) {
	//TODO implement me
	panic("implement me")
}

func (s SystemAccessService) UpdateRole(ctx context.Context, req *access.RoleInfo) (res *base.BaseResp, err error) {
	//TODO implement me
	panic("implement me")
}

func (s SystemAccessService) DeleteRole(ctx context.Context, req *base.IDsReq) (res *base.BaseResp, err error) {
	//TODO implement me
	panic("implement me")
}

func (s SystemAccessService) GetRoleList(ctx context.Context, req *base.Empty) (res *access.RoleInfoListResp, err error) {
	//TODO implement me
	panic("implement me")
}
