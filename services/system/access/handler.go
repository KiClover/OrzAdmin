package access

import (
	"context"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	orzinit "github.com/KiClover/orzAdmin/internal/initialize"
	internallog "github.com/KiClover/orzAdmin/internal/log"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/access"
	"go.uber.org/fx"
)

const ServiceName = "system-access"

// logger 让 access 服务的日志统一带 [ACCESS] 前缀。
var logger = internallog.WithModule(internallog.ModuleAccess)

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
	// 委托共享初始化器：建表 + 按 (path, method) 幂等补齐 API、按 role_code 补齐角色、
	// 只追加 Casbin 策略（不会重置既有授权，也不会重建管理员）。
	initializer := orzinit.NewInitializer(s.Cfg)
	defer initializer.Close()
	if _, err := initializer.InitializeService(ctx, orzinit.ServiceAccess); err != nil {
		logger.Errorf("init database err: %v", err)
		return nil, err
	}
	return &base.BaseResp{Msg: "success"}, nil
}

func (s SystemAccessService) CreateApi(ctx context.Context, req *access.ApiInfo) (res *base.BaseResp, err error) {
	_, err = s.DB.Api.Create().
		SetPath(*req.Path).
		SetMethod(*req.Method).
		SetGroup(*req.Group).
		SetDescription(*req.Description).
		Save(ctx)
	if err != nil {
		logger.Errorf("create api err: %v", err)
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
		logger.Errorf("create api err: %v", err)
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
			logger.Errorf("delete api err: %v", err)
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
		logger.Errorf("get api list err: %v", err)
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
		logger.Errorf("create role err: %v", err)
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
		logger.Errorf("update role err: %v", err)
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
			logger.Errorf("delete role err: %v", err)
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
		logger.Errorf("get api list err: %v", err)
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
