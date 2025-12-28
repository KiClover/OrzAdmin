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
