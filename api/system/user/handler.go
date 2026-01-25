package user

import (
	"context"
	"strconv"

	userapi "github.com/KiClover/orzAdmin/api/model/system/user"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/access/systemaccessservice"
	"github.com/KiClover/orzAdmin/kitex_gen/system/message/systemmessageservice"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzAdmin/kitex_gen/system/user"
	"github.com/KiClover/orzAdmin/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzAdmin/pkg/hresp"
	"github.com/KiClover/orzAdmin/pkg/utils"
	"github.com/KiClover/orzAdmin/services/system/access"
	user2 "github.com/KiClover/orzAdmin/services/system/user"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

const ServiceName = "api.system.user"

type HzSystemUserServiceP struct {
	fx.In
	UserClient    systemuserservice.Client
	Cfg           *conf.Config
	Casbin        *casbin.Enforcer
	SettingClient systemsettingservice.Client
	Redis         redis.UniversalClient
	MsgClient     systemmessageservice.Client
	AccessClient  systemaccessservice.Client
}
type HzSystemUserService struct {
	UserClient    systemuserservice.Client
	Cfg           *conf.Config
	Casbin        *casbin.Enforcer
	SettingClient systemsettingservice.Client
	Redis         redis.UniversalClient
	MsgClient     systemmessageservice.Client
	AccessClient  systemaccessservice.Client
}

func NewHzSystemUserService(p HzSystemUserServiceP) *HzSystemUserService {
	return &HzSystemUserService{
		UserClient:    p.UserClient,
		Cfg:           p.Cfg,
		Casbin:        p.Casbin,
		SettingClient: p.SettingClient,
		Redis:         p.Redis,
		MsgClient:     p.MsgClient,
		AccessClient:  p.AccessClient,
	}
}

func (s *HzSystemUserService) InitDatabase(ctx context.Context, c *app.RequestContext) {
	hresp.BaseSuccessMsg(c, "init database")
}

func (s *HzSystemUserService) CreateUser(ctx context.Context, c *app.RequestContext) {
	var req userapi.UserInfoReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	re, err := s.UserClient.CreateUser(ctx, &user.UserInfo{
		Status:      utils.GetPointer(uint32(*req.Status)),
		Name:        req.Name,
		Password:    req.Password,
		Description: req.Description,
		Mobile:      req.Mobile,
		Email:       req.Email,
		Avatar:      req.Avatar,
		Gender:      utils.GetPointer(uint32(*req.Gender)),
	})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}
	// 赋权
	for _, r := range req.RoleCodes {
		_, err = s.Casbin.AddRoleForUser(strconv.FormatInt(re.Id, 10), r)
		if err != nil {
			hresp.BaseErrMsg(c, 1, "add roles failed", err)
			return
		}
	}
	hresp.BaseSuccessMsgWithBody(c, "create user success", map[string]interface{}{
		"id": re.Id,
	})
}

func (s *HzSystemUserService) UpdateUser(ctx context.Context, c *app.RequestContext) {
	var req userapi.UserInfoReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	var status *uint32
	if req.Status != nil {
		status = utils.GetPointer(uint32(*req.Status))
	} else {
		status = nil
	}
	var gender *uint32
	if req.Gender != nil {
		gender = utils.GetPointer(uint32(*req.Gender))
	} else {
		gender = nil
	}
	_, err := s.UserClient.UpdateUser(ctx, &user.UserInfo{
		Id:          req.ID,
		Status:      status,
		Name:        req.Name,
		Password:    req.Password,
		Description: req.Description,
		Mobile:      req.Mobile,
		Email:       req.Email,
		Avatar:      req.Avatar,
		Gender:      gender,
	})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}
	hresp.BaseSuccessMsg(c, "update user success")
}

func (s *HzSystemUserService) DeleteUser(ctx context.Context, c *app.RequestContext) {
	var req userapi.UserInfoReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.BaseErrMsg(c, 4, "bind and validate failed", err)
		return
	}
	_, err := s.UserClient.DeleteUser(ctx, &base.IDInt64Req{Id: *req.ID})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}
	hresp.BaseSuccessMsg(c, "delete user success")
}

func (s *HzSystemUserService) GetUserList(ctx context.Context, c *app.RequestContext) {
	var req userapi.GetUserListReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	var status *uint32
	if req.Status != nil {
		status = utils.GetPointer(uint32(*req.Status))
	} else {
		status = nil
	}
	r, err := s.UserClient.GetUserList(ctx, &user.GetUserListReq{
		Page:     req.GetPage(),
		PageSize: req.GetPageSize(),
		Status:   status,
		Name:     req.Name,
		Mobile:   req.Mobile,
		Email:    req.Email,
	})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}

	// 写入role map
	roleResp, err := s.AccessClient.GetRoleList(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, access.ServiceName, err)
		return
	}
	roleMap := make(map[string]string, len(roleResp.List))
	for _, v := range roleResp.List {
		roleMap[v.GetRoleCode()] = v.GetRoleName()
	}

	var data []*userapi.UserInfo
	for _, v := range r.Data {
		role, err := s.Casbin.GetRolesForUser(strconv.FormatInt(*v.Id, 10))
		if err != nil {
			hresp.BaseErrMsg(c, 1, "get roles failed", err)
			return
		}
		var roleName []string
		for _, v1 := range role {
			roleName = append(roleName, roleMap[v1])
		}
		data = append(data, &userapi.UserInfo{
			ID:          v.Id,
			CreatedAt:   v.CreatedAt,
			UpdatedAt:   v.UpdatedAt,
			Name:        v.Name,
			Description: v.Description,
			Mobile:      v.Mobile,
			Email:       v.Email,
			Avatar:      v.Avatar,
			Gender:      utils.GetPointer(int32(*v.Gender)),
			Status:      utils.GetPointer(int32(*v.Status)),
			RoleCode:    role,
			RoleName:    roleName,
		})
	}
	hresp.BaseSuccessMsgWithBody(c, "get user list success", userapi.GetUserListResp{
		Total: r.Total,
		Data:  data,
	})
}

func (s *HzSystemUserService) GetUserById(ctx context.Context, c *app.RequestContext) {
	var req userapi.GetUserById
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	r, err := s.UserClient.GetUserById(ctx, &user.GetUserByIdReq{Id: &req.ID})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
	}

	// 写入role map
	roleCode, err := s.Casbin.GetRolesForUser(strconv.FormatInt(*r.Id, 10))
	roleResp, err := s.AccessClient.GetRoleList(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, access.ServiceName, err)
		return
	}
	roleMap := make(map[string]string, len(roleResp.List))
	for _, v := range roleResp.List {
		roleMap[v.GetRoleCode()] = v.GetRoleName()
	}
	roleName := make([]string, len(roleCode))
	for i, v := range roleCode {
		roleName[i] = roleMap[v]
	}

	hresp.BaseSuccessMsgWithBody(c, "get user by id success", &userapi.UserInfo{
		ID:          r.Id,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		Name:        r.Name,
		Description: r.Description,
		Mobile:      r.Mobile,
		Email:       r.Email,
		Avatar:      r.Avatar,
		Gender:      utils.GetPointer(int32(*r.Gender)),
		Status:      utils.GetPointer(int32(*r.Status)),
		RoleCode:    roleCode,
		RoleName:    roleName,
	})
}

func (s *HzSystemUserService) GetUserProfile(ctx context.Context, c *app.RequestContext) {
	userId, isExist := c.Get("userId")
	if !isExist {
		hresp.BaseErrMsg(c, 1, "get user id failed", nil)
		return
	}
	r, err := s.UserClient.GetUserById(ctx, &user.GetUserByIdReq{Id: utils.GetPointer(int64(userId.(uint64)))})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}
	roleCode, err := s.Casbin.GetRolesForUser(strconv.FormatUint(userId.(uint64), 10))
	if err != nil {
		hresp.BaseErrMsg(c, 1, "get roles failed", err)
		return
	}

	// 写入role map
	roleResp, err := s.AccessClient.GetRoleList(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, access.ServiceName, err)
		return
	}
	roleMap := make(map[string]string, len(roleResp.List))
	for _, v := range roleResp.List {
		roleMap[v.GetRoleCode()] = v.GetRoleName()
	}
	roleName := make([]string, len(roleCode))
	for i, v := range roleCode {
		roleName[i] = roleMap[v]
	}

	hresp.BaseSuccessMsgWithBody(c, "get user profile success", &userapi.GetUserProfileResp{
		ID:          r.Id,
		Status:      utils.GetPointer(int32(*r.Status)),
		Name:        r.Name,
		Description: r.Description,
		Mobile:      r.Mobile,
		Email:       r.Email,
		Avatar:      r.Avatar,
		RoleName:    roleName,
		Gender:      utils.GetPointer(int32(*r.Gender)),
	})
}

func (s *HzSystemUserService) UpdateUserInfo(ctx context.Context, c *app.RequestContext) {
	userId, isExist := c.Get("userId")
	if !isExist {
		hresp.BaseErrMsg(c, 1, "get user id failed", nil)
		return
	}
	var req userapi.UpdateUserProfileReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	_, err := s.UserClient.UpdateUser(ctx, &user.UserInfo{
		Id:          utils.GetPointer(int64(userId.(uint64))),
		Name:        req.Name,
		Description: req.Description,
		Mobile:      req.Mobile,
		Email:       req.Email,
		Avatar:      req.Avatar,
		Gender:      utils.GetPointer(uint32(*req.Gender)),
	})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}
	hresp.BaseSuccessMsg(c, "更新用户信息成功")
}

func (s *HzSystemUserService) ChangePwd(ctx context.Context, c *app.RequestContext) {
	userId, isExist := c.Get("userId")
	if !isExist {
		hresp.BaseErrMsg(c, 1, "get user id failed", nil)
		return
	}
	var req userapi.ChangeUserPwdReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	userInfo, err := s.UserClient.GetUserById(ctx, &user.GetUserByIdReq{Id: utils.GetPointer(int64(userId.(uint64)))})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
	}
	v := utils.CheckPasswordHash(*req.OldPassword, *userInfo.Password)
	if v != nil {
		hresp.BaseErrMsg(c, 3, "密码错误", nil)
		return
	}
	_, err = s.UserClient.UpdateUser(ctx, &user.UserInfo{Id: userInfo.Id, Password: req.NewPassword})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
	}
	hresp.BaseSuccessMsg(c, "修改密码成功")
}

func (s *HzSystemUserService) GetUserBasicInfoById(ctx context.Context, c *app.RequestContext) {
	var req userapi.GetUserById
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	r, err := s.UserClient.GetUserById(ctx, &user.GetUserByIdReq{Id: &req.ID})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
	}

	hresp.BaseSuccessMsgWithBody(c, "get user basic info by id success", &userapi.UserBasicInfo{
		ID:          r.Id,
		Name:        r.Name,
		Description: r.Description,
		Avatar:      r.Avatar,
	})
}
