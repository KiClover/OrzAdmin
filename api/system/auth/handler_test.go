package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/message"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/KiClover/orzAdmin/kitex_gen/system/user"
	"github.com/KiClover/orzAdmin/pkg/hresp"
	"github.com/KiClover/orzAdmin/pkg/utils"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/kitex/client/callopt"
	"github.com/redis/go-redis/v9"
)

type memRedisHook struct {
	mu   sync.Mutex
	data map[string]string
}

func (h *memRedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, errors.New("unexpected dial in in-memory redis hook")
	}
}

func (h *memRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.data == nil {
			h.data = make(map[string]string)
		}
		args := cmd.Args()
		switch cmd.Name() {
		case "set":
			if len(args) >= 3 {
				key, _ := args[1].(string)
				val, _ := args[2].(string)
				h.data[key] = val
			}
			if c, ok := cmd.(*redis.StatusCmd); ok {
				c.SetVal("OK")
			}
			return nil
		case "get":
			key, _ := args[1].(string)
			val, ok := h.data[key]
			c, _ := cmd.(*redis.StringCmd)
			if !ok {
				c.SetErr(redis.Nil)
				return redis.Nil
			}
			c.SetVal(val)
			return nil
		case "del":
			var deleted int64
			for _, raw := range args[1:] {
				key, _ := raw.(string)
				if _, ok := h.data[key]; ok {
					delete(h.data, key)
					deleted++
				}
			}
			if c, ok := cmd.(*redis.IntCmd); ok {
				c.SetVal(deleted)
			}
			return nil
		default:
			return next(ctx, cmd)
		}
	}
}

func (h *memRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func newTestRedisClient() (*redis.Client, *memRedisHook) {
	hook := &memRedisHook{data: make(map[string]string)}
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	client.AddHook(hook)
	return client, hook
}

// fakeAuthUserClient 故意按精确字符串匹配（区分大小写）存储与查询邮箱，
// 用来验证 handler 层必须把邮箱归一化后再传给下游。
type fakeAuthUserClient struct {
	mu      sync.Mutex
	nextID  int64
	byID    map[int64]*user.UserInfo
	byEmail map[string]*user.UserInfo
}

func newFakeAuthUserClient() *fakeAuthUserClient {
	return &fakeAuthUserClient{
		nextID:  1,
		byID:    make(map[int64]*user.UserInfo),
		byEmail: make(map[string]*user.UserInfo),
	}
}

func (f *fakeAuthUserClient) InitDatabase(context.Context, *user.InitDatabaseReq, ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}

func (f *fakeAuthUserClient) GetBootstrapAdmin(context.Context, *base.Empty, ...callopt.Option) (*user.BootstrapAdmin, error) {
	return &user.BootstrapAdmin{}, nil
}

func (f *fakeAuthUserClient) CreateUser(_ context.Context, req *user.UserInfo, _ ...callopt.Option) (*base.BaseIDInt64Resp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextID
	f.nextID++
	hash, err := utils.HashPassword(*req.Password)
	if err != nil {
		return nil, err
	}
	status := uint32(0)
	if req.Status != nil {
		status = *req.Status
	}
	copyInfo := &user.UserInfo{
		Id:       &id,
		Name:     req.Name,
		Email:    req.Email,
		Password: &hash,
		Status:   &status,
	}
	f.byID[id] = copyInfo
	if req.Email != nil {
		f.byEmail[*req.Email] = copyInfo
	}
	return &base.BaseIDInt64Resp{Id: id, Msg: "success"}, nil
}

func (f *fakeAuthUserClient) UpdateUser(_ context.Context, req *user.UserInfo, _ ...callopt.Option) (*base.BaseResp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[*req.Id]
	if !ok {
		return nil, errors.New("not found")
	}
	if req.Password != nil {
		hash, err := utils.HashPassword(*req.Password)
		if err != nil {
			return nil, err
		}
		u.Password = &hash
	}
	if req.Email != nil {
		if u.Email != nil {
			delete(f.byEmail, *u.Email)
		}
		u.Email = req.Email
		f.byEmail[*req.Email] = u
	}
	return &base.BaseResp{Msg: "success"}, nil
}

func (f *fakeAuthUserClient) DeleteUser(context.Context, *base.IDInt64Req, ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}

func (f *fakeAuthUserClient) GetUserList(context.Context, *user.GetUserListReq, ...callopt.Option) (*user.GetUserListResp, error) {
	return &user.GetUserListResp{}, nil
}

func (f *fakeAuthUserClient) GetUserById(_ context.Context, req *user.GetUserByIdReq, _ ...callopt.Option) (*user.UserInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[*req.Id]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}

func (f *fakeAuthUserClient) GetUserByEmail(_ context.Context, req *user.GetUserByEmailReq, _ ...callopt.Option) (*user.UserInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byEmail[*req.Email]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}

func (f *fakeAuthUserClient) GetUserByMobile(context.Context, *user.GetUserByMobileReq, ...callopt.Option) (*user.UserInfo, error) {
	return nil, errors.New("not found")
}

func (f *fakeAuthUserClient) GetUserIsExist(_ context.Context, req *user.GetUserIsExistReq, _ ...callopt.Option) (*user.GetUserIsExistResp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if req.Email != nil {
		_, ok := f.byEmail[*req.Email]
		return &user.GetUserIsExistResp{IsExist: ok}, nil
	}
	return &user.GetUserIsExistResp{IsExist: false}, nil
}

type fakeAuthSettingClient struct{}

func (f *fakeAuthSettingClient) InitDatabase(context.Context, *base.Empty, ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}
func (f *fakeAuthSettingClient) ChangeRegUserRole(context.Context, *setting.ChangeRegUserRoleReq, ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}
func (f *fakeAuthSettingClient) GetRegUserRole(context.Context, *base.Empty, ...callopt.Option) (*setting.GetRegUserRoleResp, error) {
	return &setting.GetRegUserRoleResp{Role: "user"}, nil
}
func (f *fakeAuthSettingClient) SetEmailProvider(context.Context, *setting.EmailProviderInfo, ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}
func (f *fakeAuthSettingClient) GetEmailProvider(context.Context, *base.Empty, ...callopt.Option) (*setting.EmailProviderInfo, error) {
	return &setting.EmailProviderInfo{}, nil
}
func (f *fakeAuthSettingClient) SetSmsProvider(context.Context, *setting.SmsProviderInfo, ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}
func (f *fakeAuthSettingClient) GetSmsProvider(context.Context, *base.Empty, ...callopt.Option) (*setting.SmsProviderInfo, error) {
	return &setting.SmsProviderInfo{}, nil
}
func (f *fakeAuthSettingClient) GetWalletSettingsMasked(context.Context, *base.Empty, ...callopt.Option) (*setting.WalletSettingsMasked, error) {
	return &setting.WalletSettingsMasked{}, nil
}
func (f *fakeAuthSettingClient) UpdateWalletSettings(context.Context, *setting.UpdateWalletSettingsReq, ...callopt.Option) (*setting.WalletSettingsMasked, error) {
	return &setting.WalletSettingsMasked{}, nil
}
func (f *fakeAuthSettingClient) GetWalletRuntimeSettings(context.Context, *base.Empty, ...callopt.Option) (*setting.WalletRuntimeSettings, error) {
	return &setting.WalletRuntimeSettings{}, nil
}

type fakeAuthMsgClient struct {
	code   string
	lastTo string
	calls  int
}

func (f *fakeAuthMsgClient) InitDatabase(context.Context, *base.Empty, ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}
func (f *fakeAuthMsgClient) SendEmailCode(_ context.Context, req *message.SendCodeReq, _ ...callopt.Option) (*message.SendCodeResp, error) {
	f.calls++
	f.lastTo = req.To
	return &message.SendCodeResp{Code: f.code}, nil
}
func (f *fakeAuthMsgClient) SendSmsCode(context.Context, *message.SendCodeReq, ...callopt.Option) (*message.SendCodeResp, error) {
	return &message.SendCodeResp{Code: f.code}, nil
}

func newTestCasbinEnforcer(t *testing.T) *casbin.Enforcer {
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
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		t.Fatalf("new casbin enforcer: %v", err)
	}
	return e
}

func newAuthTestServer(t *testing.T) (*server.Hertz, *fakeAuthUserClient, *fakeAuthMsgClient, *memRedisHook) {
	t.Helper()
	redisClient, hook := newTestRedisClient()
	userClient := newFakeAuthUserClient()
	msgClient := &fakeAuthMsgClient{code: "654321"}

	svc := &HzSystemAuthService{
		UserClient: userClient,
		Cfg: &conf.Config{AuthConf: conf.AuthConf{
			AccessSecret: "test-secret",
			AccessExpire: 3600,
		}},
		Casbin:        newTestCasbinEnforcer(t),
		SettingClient: &fakeAuthSettingClient{},
		Redis:         redisClient,
		MsgClient:     msgClient,
	}
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	NewRegisterRouter(h, svc)
	return h, userClient, msgClient, hook
}

func performJSON(t *testing.T, h *server.Hertz, method, path string, payload any) hresp.Resp {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	w := ut.PerformRequest(h.Engine, method, path, &ut.Body{Body: bytes.NewReader(raw), Len: len(raw)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	if w.Code != http.StatusOK {
		t.Fatalf("%s %s http status = %d, want 200; body=%s", method, path, w.Code, w.Body.String())
	}
	var resp hresp.Resp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", w.Body.String(), err)
	}
	return resp
}

func seedCaptcha(hook *memRedisHook, id, answer string) {
	hook.mu.Lock()
	defer hook.mu.Unlock()
	hook.data[captchaPrefix+id] = answer
}

func TestGetVerifyCodeAndRegisterEmailCaseInsensitive(t *testing.T) {
	h, userClient, msgClient, hook := newAuthTestServer(t)

	// 1. 使用混合大小写 + 首尾空格发送验证码，应归一化后存入 Redis 并发送邮件。
	seedCaptcha(hook, "cap1", "ABCD")
	resp := performJSON(t, h, http.MethodPost, "/verify/code", map[string]any{
		"type":       1,
		"account":    "  Alice.Smith@Example.COM ",
		"captcha_id": "cap1",
		"captcha":    "ABCD",
	})
	if resp.Code != 0 {
		t.Fatalf("GetVerifyCode failed: %+v", resp)
	}
	if msgClient.lastTo != "alice.smith@example.com" {
		t.Fatalf("SendEmailCode To = %q, want %q", msgClient.lastTo, "alice.smith@example.com")
	}

	// 2. 换全大写再次发送，必须命中同一份 sent 限流 Key。
	seedCaptcha(hook, "cap2", "EFGH")
	resp = performJSON(t, h, http.MethodPost, "/verify/code", map[string]any{
		"type":       1,
		"account":    "ALICE.SMITH@EXAMPLE.COM",
		"captcha_id": "cap2",
		"captcha":    "EFGH",
	})
	if resp.Code == 0 || resp.Msg != "验证码已发送，请勿频繁发送" {
		t.Fatalf("expected rate limit hit across different cases, got %+v", resp)
	}

	// 3. 用全小写提交注册，应命中同一个验证码并把小写邮箱写入用户存储。
	resp = performJSON(t, h, http.MethodPost, "/reg/email", map[string]any{
		"email":     "alice.smith@example.com",
		"code":      "654321",
		"password":  "Secret123!",
		"user_name": "alice",
	})
	if resp.Code != 0 {
		t.Fatalf("RegisterByEmail failed: %+v", resp)
	}
	if _, ok := userClient.byEmail["alice.smith@example.com"]; !ok {
		t.Fatalf("expected user stored under normalized lowercase email, got keys=%v", userClient.byEmail)
	}

	// 4. 换大写再次注册同一邮箱，必须提示已注册。
	resp = performJSON(t, h, http.MethodPost, "/reg/email", map[string]any{
		"email":     "ALICE.SMITH@EXAMPLE.COM",
		"code":      "654321",
		"password":  "Secret123!",
		"user_name": "alice2",
	})
	if resp.Code == 0 || resp.Msg != "该邮箱已注册" {
		t.Fatalf("expected duplicate email rejection across different cases, got %+v", resp)
	}
}

func TestLoginAndFindPwdEmailCaseInsensitive(t *testing.T) {
	h, userClient, _, hook := newAuthTestServer(t)

	normalized := "alice@example.com"
	initialPwd := "Initial123!"
	if _, err := userClient.CreateUser(context.Background(), &user.UserInfo{
		Name:     utils.GetPointer("alice"),
		Email:    &normalized,
		Password: &initialPwd,
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// 1. 密码登录：输入混合大小写 + 空格应成功。
	resp := performJSON(t, h, http.MethodPost, "/login/pwd", map[string]any{
		"type":     1,
		"account":  "  AlIcE@Example.COM ",
		"password": initialPwd,
	})
	if resp.Code != 0 {
		t.Fatalf("LoginByPwd failed: %+v", resp)
	}

	// 2. 验证码登录：大写发送验证码，小写提交登录应成功。
	seedCaptcha(hook, "cap3", "1234")
	resp = performJSON(t, h, http.MethodPost, "/verify/code", map[string]any{
		"type":       1,
		"account":    "ALICE@EXAMPLE.COM",
		"captcha_id": "cap3",
		"captcha":    "1234",
	})
	if resp.Code != 0 {
		t.Fatalf("GetVerifyCode for login failed: %+v", resp)
	}
	resp = performJSON(t, h, http.MethodPost, "/login/code", map[string]any{
		"type":    1,
		"account": "alice@example.com",
		"code":    "654321",
	})
	if resp.Code != 0 {
		t.Fatalf("LoginByCode failed: %+v", resp)
	}

	// 清理频次限制 Key 后测找回密码。
	hook.mu.Lock()
	delete(hook.data, SentPrefix+normalized)
	hook.mu.Unlock()

	// 3. 找回密码：小写发码，大写改密，再用混合大小写+新密码登录。
	seedCaptcha(hook, "cap4", "5678")
	resp = performJSON(t, h, http.MethodPost, "/verify/code", map[string]any{
		"type":       1,
		"account":    "alice@example.com",
		"captcha_id": "cap4",
		"captcha":    "5678",
	})
	if resp.Code != 0 {
		t.Fatalf("GetVerifyCode for find pwd failed: %+v", resp)
	}
	resp = performJSON(t, h, http.MethodPost, "/login/find", map[string]any{
		"type":     1,
		"account":  "  ALICE@EXAMPLE.COM ",
		"code":     "654321",
		"password": "Updated456!",
	})
	if resp.Code != 0 {
		t.Fatalf("FindPwd failed: %+v", resp)
	}

	resp = performJSON(t, h, http.MethodPost, "/login/pwd", map[string]any{
		"type":     1,
		"account":  "Alice@Example.Com",
		"password": "Updated456!",
	})
	if resp.Code != 0 {
		t.Fatalf("LoginByPwd with new password failed: %+v", resp)
	}
}
