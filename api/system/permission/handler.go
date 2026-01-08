package permission

import (
	"context"

	"github.com/KiClover/orzfrp/api/model/system/permission"
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/kitex_gen/base"
	"github.com/KiClover/orzfrp/kitex_gen/system/access"
	"github.com/KiClover/orzfrp/kitex_gen/system/access/systemaccessservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzfrp/pkg/hresp"
	"github.com/KiClover/orzfrp/pkg/utils"
	access1 "github.com/KiClover/orzfrp/services/system/access"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

const ServiceName = "api.system.permission"

type HzSystemPermissionServiceP struct {
	fx.In
	UserClient    systemuserservice.Client
	Cfg           *conf.Config
	Casbin        *casbin.Enforcer
	SettingClient systemsettingservice.Client
	Redis         redis.UniversalClient
	AccessClient  systemaccessservice.Client
}
type HzSystemPermissionService struct {
	UserClient    systemuserservice.Client
	Cfg           *conf.Config
	Casbin        *casbin.Enforcer
	SettingClient systemsettingservice.Client
	Redis         redis.UniversalClient
	AccessClient  systemaccessservice.Client
}

func NewHzSystemPermissionService(p HzSystemPermissionServiceP) *HzSystemPermissionService {
	return &HzSystemPermissionService{
		UserClient:    p.UserClient,
		Cfg:           p.Cfg,
		Casbin:        p.Casbin,
		SettingClient: p.SettingClient,
		Redis:         p.Redis,
		AccessClient:  p.AccessClient,
	}
}

func (s *HzSystemPermissionService) InitDatabase(ctx context.Context, c *app.RequestContext) {
	_, err := s.AccessClient.InitDatabase(ctx, &base.Empty{})
	if err != nil {
		hresp.BaseErrMsg(c, 4, "init database failed", err)
		return
	}
	apis, err := s.AccessClient.GetApiList(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, access1.ServiceName, err)
	}
	for _, r := range apis.List {
		_, err = s.Casbin.AddPolicy("admin", *r.Path, *r.Method)
		if err != nil {
			hresp.BaseErrMsg(c, 1, "add policy failed", err)
			return
		}
	}
	hresp.BaseSuccessMsg(c, "init database success")
	return
}

func (s *HzSystemPermissionService) CreateApi(ctx context.Context, c *app.RequestContext) {
	var req permission.ApiInfo
	if err := c.BindAndValidate(req); err != nil {
		hresp.BaseErrMsg(c, 4, "bind and validate failed", err)
		return
	}
	_, err := s.AccessClient.CreateApi(ctx, &access.ApiInfo{
		Path:        req.Path,
		Description: req.Description,
		Group:       req.Group,
		Method:      req.Method,
	})
	if err != nil {
		hresp.KitexCallErr(c, access1.ServiceName, err)
	}

	hresp.BaseSuccessMsg(c, "create api success")
	return
}
func (s *HzSystemPermissionService) GetApiList(ctx context.Context, c *app.RequestContext) {
	list, err := s.AccessClient.GetApiList(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, access1.ServiceName, err)
	}
	hresp.BaseSuccessMsgWithBody(c, "success", list)
	return
}

func (s *HzSystemPermissionService) UpdateApi(ctx context.Context, c *app.RequestContext) {
	var req permission.ApiInfo
	if err := c.BindAndValidate(req); err != nil {
		hresp.BaseErrMsg(c, 4, "bind and validate failed", err)
	}
	_, err := s.AccessClient.UpdateApi(ctx, &access.ApiInfo{
		Path:        req.Path,
		Description: req.Description,
		Group:       req.Group,
		Method:      req.Method,
	})
	if err != nil {
		hresp.KitexCallErr(c, access1.ServiceName, err)
	}
	hresp.BaseSuccessMsg(c, "update api success")
	return
}

func (s *HzSystemPermissionService) DeleteApi(ctx context.Context, c *app.RequestContext) {
	var req base.IDsReq
	if err := c.BindAndValidate(req); err != nil {
		hresp.BaseErrMsg(c, 4, "bind and validate failed", err)
	}
	_, err := s.AccessClient.DeleteApi(ctx, &req)
	if err != nil {
		hresp.KitexCallErr(c, access1.ServiceName, err)
	}

	hresp.BaseSuccessMsg(c, "delete api success")
	return
}
func (s *HzSystemPermissionService) CreateRole(ctx context.Context, c *app.RequestContext) {
	var req permission.RoleInfo
	if err := c.BindAndValidate(req); err != nil {
	}
	_, err := s.AccessClient.CreateRole(ctx, &access.RoleInfo{
		RoleCode: req.RoleCode,
		RoleName: req.RoleName,
		Remark:   req.Remark,
	})
	if err != nil {
		hresp.KitexCallErr(c, access1.ServiceName, err)
	}

	hresp.BaseSuccessMsg(c, "create role success")
	return
}

func (s *HzSystemPermissionService) UpdateRole(ctx context.Context, c *app.RequestContext) {
	var req permission.RoleInfo
	if err := c.BindAndValidate(req); err != nil {
	}
	_, err := s.AccessClient.UpdateRole(ctx, &access.RoleInfo{
		Id:       utils.GetPointer(uint64(*req.ID)),
		RoleName: req.RoleName,
		Remark:   req.Remark,
	})
	if err != nil {
		hresp.KitexCallErr(c, access1.ServiceName, err)
	}

	hresp.BaseSuccessMsg(c, "update role success")
	return
}

func (s *HzSystemPermissionService) DeleteRole(ctx context.Context, c *app.RequestContext) {
	var req base.IDsReq
	if err := c.BindAndValidate(req); err != nil {
		hresp.BaseErrMsg(c, 4, "bind and validate failed", err)
	}
	_, err := s.AccessClient.DeleteRole(ctx, &req)
	if err != nil {
		hresp.KitexCallErr(c, access1.ServiceName, err)
	}
	hresp.BaseSuccessMsg(c, "delete role success")
	return
}

func (s *HzSystemPermissionService) GetRoleList(ctx context.Context, c *app.RequestContext) {
	list, err := s.AccessClient.GetRoleList(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, access1.ServiceName, err)
	}
	hresp.BaseSuccessMsgWithBody(c, "success", list)
	return
}

func (s *HzSystemPermissionService) UpdateCasbin(ctx context.Context, c *app.RequestContext) {
	var req permission.UpdateCasbinReq
	if err := c.BindAndValidate(req); err != nil {
		hresp.BaseErrMsg(c, 4, "bind and validate failed", err)
	}

	_, err := s.Casbin.RemoveFilteredPolicy(0, *req.RoleCode)
	if err != nil {
		hresp.BaseErrMsg(c, 1, "remove policy failed", err)
	}

	for _, v := range req.Rules {
		_, err = s.Casbin.AddPolicy(req.RoleCode, *v.Path, *v.Method)
		if err != nil {
			hresp.BaseErrMsg(c, 1, "add policy failed", err)
		}
	}
}
