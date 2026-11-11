package setting

import (
	"net/http"
	"testing"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

// newRouterTestEngine 用真实路由表构造测试引擎（JWT 中间件会拦截未认证请求）。
func newRouterTestEngine(t *testing.T) *server.Hertz {
	t.Helper()
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	svc := &HzSystemSettingService{
		Cfg:           &conf.Config{},
		SettingClient: &fakeSettingClient{},
	}
	NewRegisterRouter(h, svc)
	return h
}

func TestWalletSettingRoutesAreRegistered(t *testing.T) {
	h := newRouterTestEngine(t)

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		w := ut.PerformRequest(h.Engine, method, "/setting/wallet", nil)
		// 未携带 Authorization：路由存在时应被 JWT 中间件拦截为 401，而不是 404。
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s /setting/wallet 状态码 = %d，期望 401（body=%s）", method, w.Code, w.Body.String())
		}
	}
}

func TestWalletRoutesDoNotExposeRuntimeSettings(t *testing.T) {
	h := newRouterTestEngine(t)

	// 运行时快照只走内部 RPC，不注册任何 HTTP 路径。
	for _, path := range []string{
		"/setting/wallet/runtime",
		"/setting/wallet/runtime-settings",
		"/setting/wallet/secrets",
	} {
		w := ut.PerformRequest(h.Engine, http.MethodGet, path, nil)
		if w.Code != http.StatusNotFound && w.Code != http.StatusUnauthorized {
			t.Fatalf("%s 状态码 = %d", path, w.Code)
		}
	}
}

func TestExistingSettingRoutesRemain(t *testing.T) {
	h := newRouterTestEngine(t)

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/setting/register/role"},
		{http.MethodGet, "/setting/email"},
		{http.MethodGet, "/setting/mobile"},
	} {
		w := ut.PerformRequest(h.Engine, route.method, route.path, nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s 状态码 = %d，期望 401", route.method, route.path, w.Code)
		}
	}
}

func TestHealthRouteRegistered(t *testing.T) {
	h := newRouterTestEngine(t)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /health 状态码 = %d，期望 200", w.Code)
	}
}
