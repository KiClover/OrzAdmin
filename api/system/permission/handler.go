package permission

import (
	"context"
	"strconv"

	"github.com/KiClover/orzAdmin/api/model/system/permission"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/access"
	"github.com/KiClover/orzAdmin/kitex_gen/system/access/systemaccessservice"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzAdmin/kitex_gen/system/user"
	"github.com/KiClover/orzAdmin/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzAdmin/pkg/hresp"
	"github.com/KiClover/orzAdmin/pkg/utils"
	access1 "github.com/KiClover/orzAdmin/services/system/access"
	user1 "github.com/KiClover/orzAdmin/services/system/user"
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

	// 为Editor赋权
	_, err = s.Casbin.AddPolicy("admin", "/web/create", "POST")
	if err != nil {
		hresp.BaseErrMsg(c, 1, "add policy failed", err)
		return
	}
	_, err = s.Casbin.AddPolicy("admin", "/web/delete", "POST")
	if err != nil {
		hresp.BaseErrMsg(c, 1, "add policy failed", err)
		return
	}

	u, err := s.UserClient.CreateUser(ctx, &user.UserInfo{
		Status:      utils.GetPointer(uint32(0)),
		Name:        utils.GetPointer("admin"),
		Password:    utils.GetPointer("123456789orzfrp"),
		Description: utils.GetPointer("administrator"),
		Mobile:      utils.GetPointer("10000000000"),
		Email:       utils.GetPointer("kiclover@email.cn"),
		Avatar:      nil,
		Gender:      nil,
	})
	if err != nil {
		hresp.KitexCallErr(c, user1.ServiceName, err)
		return
	}
	_, err = s.Casbin.AddRoleForUser(strconv.FormatInt(u.Id, 10), "admin")
	if err != nil {

		hresp.BaseErrMsg(c, 1, "add role for user failed", err)
		return
	}
	hresp.BaseSuccessMsg(c, "init database success")
	return
}

func (s *HzSystemPermissionService) CreateApi(ctx context.Context, c *app.RequestContext) {
	var req permission.ApiInfo
	if err := c.BindAndValidate(&req); err != nil {
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
	if err := c.BindAndValidate(&req); err != nil {
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
	if err := c.BindAndValidate(&req); err != nil {
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
	if err := c.BindAndValidate(&req); err != nil {
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
	if err := c.BindAndValidate(&req); err != nil {
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

func (s *HzSystemPermissionService) UpdateRoleCasbin(ctx context.Context, c *app.RequestContext) {
	var req permission.UpdateRoleCasbinReq
	if err := c.BindAndValidate(&req); err != nil {
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
	hresp.BaseSuccessMsg(c, "update role casbin success")
	return
}

func (s *HzSystemPermissionService) UpdateUserCabin(ctx context.Context, c *app.RequestContext) {
	var req permission.UpdateUserCasbinReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.BaseErrMsg(c, 4, "bind and validate failed", err)
	}
	roles, err := s.Casbin.GetRolesForUser(strconv.FormatInt(*req.ID, 10))
	if err != nil {
		hresp.BaseErrMsg(c, 1, "get roles for user failed", err)
	}
	for _, r := range roles {
		_, err = s.Casbin.DeleteRoleForUser(strconv.FormatInt(*req.ID, 10), r)
		if err != nil {
			hresp.BaseErrMsg(c, 1, "remove roles failed", err)
		}
	}
	for _, r := range req.RoleCodes {
		_, err = s.Casbin.AddRoleForUser(strconv.FormatInt(*req.ID, 10), r)
		if err != nil {
			hresp.BaseErrMsg(c, 1, "add roles failed", err)
		}
	}
	hresp.BaseSuccessMsg(c, "update user casbin success")
	return
}

func (s *HzSystemPermissionService) GetRoleCasbinRules(ctx context.Context, c *app.RequestContext) {
	var req permission.GetRoleCasbinRulesReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	if req.RoleCode == nil || len(*req.RoleCode) == 0 {
		hresp.BaseErrMsg(c, 4, "role_code is empty", nil)
		return
	}

	policies, err := s.Casbin.GetFilteredPolicy(0, *req.RoleCode)
	if err != nil {
		hresp.BaseErrMsg(c, 1, "get role policies failed", err)
		return
	}

	rules := make([]*permission.CasbinRule, 0, len(policies))
	for _, p := range policies {
		if len(p) < 3 {
			continue
		}
		path := p[1]
		method := p[2]
		rules = append(rules, &permission.CasbinRule{
			Path:   &path,
			Method: &method,
		})
	}

	resp := &permission.GetRoleCasbinRulesResp{
		RoleCode: req.RoleCode,
		Rules:    rules,
	}
	hresp.BaseSuccessMsgWithBody(c, "success", resp)
}

func (s *HzSystemPermissionService) GetUserRoles(ctx context.Context, c *app.RequestContext) {
	var req permission.GetUserRolesReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	if req.ID == nil || *req.ID <= 0 {
		hresp.BaseErrMsg(c, 4, "id is invalid", nil)
		return
	}

	userKey := strconv.FormatInt(*req.ID, 10)
	roles, err := s.Casbin.GetRolesForUser(userKey)
	if err != nil {
		hresp.BaseErrMsg(c, 1, "get user roles failed", err)
		return
	}

	resp := &permission.GetUserRolesResp{
		ID:        req.ID,
		RoleCodes: roles,
	}
	hresp.BaseSuccessMsgWithBody(c, "success", resp)
}

func (s *HzSystemPermissionService) GetRoleUsers(ctx context.Context, c *app.RequestContext) {
	var req permission.GetRoleUsersReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	if req.RoleCode == nil || len(*req.RoleCode) == 0 {
		hresp.BaseErrMsg(c, 4, "role_code is empty", nil)
		return
	}

	users, err := s.Casbin.GetUsersForRole(*req.RoleCode)
	if err != nil {
		hresp.BaseErrMsg(c, 1, "get users for role failed", err)
		return
	}
	userList := make([]int64, 0, len(users))
	for _, u := range users {
		userId, err := strconv.ParseInt(u, 10, 64)
		if err != nil {
			hresp.BaseErrMsg(c, 1, "parse user id failed", err)
			return
		}
		userList = append(userList, userId)
	}

	resp := &permission.GetRoleUsersResp{
		RoleCode: req.RoleCode,
		UserIds:  userList,
	}
	hresp.BaseSuccessMsgWithBody(c, "success", resp)
}

func (s *HzSystemPermissionService) GetUserPermissions(ctx context.Context, c *app.RequestContext) {
	var req permission.GetUserPermissionsReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	if req.ID == nil || *req.ID <= 0 {
		hresp.BaseErrMsg(c, 4, "id is invalid", nil)
		return
	}

	userKey := strconv.FormatInt(*req.ID, 10)
	perms, err := s.Casbin.GetImplicitPermissionsForUser(userKey)
	if err != nil {
		hresp.BaseErrMsg(c, 1, "get user permissions failed", err)
		return
	}

	rules := make([]*permission.CasbinRule, 0, len(perms))
	for _, p := range perms {
		if len(p) < 3 {
			continue
		}
		path := p[1]
		method := p[2]
		rules = append(rules, &permission.CasbinRule{
			Path:   &path,
			Method: &method,
		})
	}

	resp := &permission.GetUserPermissionsResp{
		ID:    req.ID,
		Rules: rules,
	}
	hresp.BaseSuccessMsgWithBody(c, "success", resp)
}
