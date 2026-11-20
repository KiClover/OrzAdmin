package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/access"
	"github.com/KiClover/orzAdmin/kitex_gen/system/access/systemaccessservice"
	userrpc "github.com/KiClover/orzAdmin/kitex_gen/system/user"
	"github.com/KiClover/orzAdmin/pkg/utils"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/kitex/client/callopt"
)

type fakeUserClient struct {
	lastCreate *userrpc.UserInfo
	lastUpdate *userrpc.UserInfo
	lastList   *userrpc.GetUserListReq
}

func (f *fakeUserClient) InitDatabase(context.Context, *userrpc.InitDatabaseReq, ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}
func (f *fakeUserClient) GetBootstrapAdmin(context.Context, *base.Empty, ...callopt.Option) (*userrpc.BootstrapAdmin, error) {
	return &userrpc.BootstrapAdmin{}, nil
}
func (f *fakeUserClient) CreateUser(_ context.Context, req *userrpc.UserInfo, _ ...callopt.Option) (*base.BaseIDInt64Resp, error) {
	f.lastCreate = req
	return &base.BaseIDInt64Resp{Id: 10, Msg: "success"}, nil
}
func (f *fakeUserClient) UpdateUser(_ context.Context, req *userrpc.UserInfo, _ ...callopt.Option) (*base.BaseResp, error) {
	f.lastUpdate = req
	return &base.BaseResp{Msg: "success"}, nil
}
func (f *fakeUserClient) DeleteUser(context.Context, *base.IDInt64Req, ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}
func (f *fakeUserClient) GetUserList(_ context.Context, req *userrpc.GetUserListReq, _ ...callopt.Option) (*userrpc.GetUserListResp, error) {
	f.lastList = req
	return &userrpc.GetUserListResp{Total: 0}, nil
}
func (f *fakeUserClient) GetUserById(context.Context, *userrpc.GetUserByIdReq, ...callopt.Option) (*userrpc.UserInfo, error) {
	return &userrpc.UserInfo{}, nil
}
func (f *fakeUserClient) GetUserByEmail(context.Context, *userrpc.GetUserByEmailReq, ...callopt.Option) (*userrpc.UserInfo, error) {
	return &userrpc.UserInfo{}, nil
}
func (f *fakeUserClient) GetUserByMobile(context.Context, *userrpc.GetUserByMobileReq, ...callopt.Option) (*userrpc.UserInfo, error) {
	return &userrpc.UserInfo{}, nil
}
func (f *fakeUserClient) GetUserIsExist(context.Context, *userrpc.GetUserIsExistReq, ...callopt.Option) (*userrpc.GetUserIsExistResp, error) {
	return &userrpc.GetUserIsExistResp{}, nil
}

type fakeAccessClient struct {
	systemaccessservice.Client
}

func (f *fakeAccessClient) GetRoleList(context.Context, *base.Empty, ...callopt.Option) (*access.RoleInfoListResp, error) {
	return &access.RoleInfoListResp{}, nil
}

func newTestUserServer(t *testing.T) (*server.Hertz, *fakeUserClient) {
	t.Helper()
	m, err := model.NewModelFromString(`
[request_definition]
r = sub, obj, act
[policy_definition]
p = sub, obj, act
[role_definition]
g = _, _
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && r.act == p.act
`)
	if err != nil {
		t.Fatalf("new casbin model: %v", err)
	}
	enforcer, err := casbin.NewEnforcer(m)
	if err != nil {
		t.Fatalf("new casbin enforcer: %v", err)
	}
	client := &fakeUserClient{}
	svc := &HzSystemUserService{
		UserClient:   client,
		Casbin:       enforcer,
		AccessClient: &fakeAccessClient{},
	}
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	h.Use(func(ctx context.Context, c *app.RequestContext) {
		c.Set("userId", uint64(1))
		c.Next(ctx)
	})
	h.POST("/user/create", svc.CreateUser)
	h.POST("/user/update", svc.UpdateUser)
	h.POST("/user/list", svc.GetUserList)
	h.POST("/user/profile/update", svc.UpdateUserInfo)
	return h, client
}

func postJSON(t *testing.T, h *server.Hertz, path string, body any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	w := ut.PerformRequest(h.Engine, http.MethodPost, path, &ut.Body{Body: bytes.NewReader(raw), Len: len(raw)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	if w.Code != http.StatusOK {
		t.Fatalf("POST %s status = %d, want 200; body=%s", path, w.Code, w.Body.String())
	}
}

func TestUserAPIHandlersNormalizeEmail(t *testing.T) {
	h, client := newTestUserServer(t)

	postJSON(t, h, "/user/create", map[string]any{
		"status":   0,
		"name":     "alice",
		"password": "pwd",
		"email":    "  Alice@Example.COM ",
		"gender":   3,
	})
	if client.lastCreate == nil || client.lastCreate.Email == nil || *client.lastCreate.Email != "alice@example.com" {
		t.Fatalf("CreateUser forwarded Email = %v, want alice@example.com", client.lastCreate.GetEmail())
	}

	postJSON(t, h, "/user/update", map[string]any{
		"id":    1,
		"email": " BOB@Example.COM ",
	})
	if client.lastUpdate == nil || client.lastUpdate.Email == nil || *client.lastUpdate.Email != "bob@example.com" {
		t.Fatalf("UpdateUser forwarded Email = %v, want bob@example.com", client.lastUpdate.GetEmail())
	}

	postJSON(t, h, "/user/list", map[string]any{
		"page":      1,
		"page_size": 10,
		"email":     " Charlie@Example.COM ",
	})
	if client.lastList == nil || client.lastList.Email == nil || *client.lastList.Email != "charlie@example.com" {
		t.Fatalf("GetUserList forwarded Email = %v, want charlie@example.com", client.lastList.GetEmail())
	}

	postJSON(t, h, "/user/profile/update", map[string]any{
		"email":  " Dave@Example.COM ",
		"gender": utils.GetPointer(int32(3)),
	})
	if client.lastUpdate == nil || client.lastUpdate.Email == nil || *client.lastUpdate.Email != "dave@example.com" {
		t.Fatalf("UpdateUserInfo forwarded Email = %v, want dave@example.com", client.lastUpdate.GetEmail())
	}
}
