package initialize

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

// newRouterServer 只注册路由，不关心初始化器行为。
func newRouterServer(t *testing.T) *server.Hertz {
	t.Helper()
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	NewRegisterRouter(h, &HzInitializeService{Initializer: &fakeInitializer{}})
	return h
}

// TestInitRoutesRegistered 确认两个初始化路由存在且路径参数被识别。
func TestInitRoutesRegistered(t *testing.T) {
	h := newRouterServer(t)

	cases := []struct {
		name string
		path string
		code int
	}{
		{name: "全部服务", path: "/init/services", code: 200},
		{name: "指定服务", path: "/init/services/system-user", code: 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := ut.PerformRequest(h.Engine, "POST", tc.path, nil)
			if got := w.Code; got != tc.code {
				t.Fatalf("POST %s 状态码 = %d，期望 %d", tc.path, got, tc.code)
			}
		})
	}
}

// TestHealthRouteRegistered 保留与其它 API 一致的探活路由。
func TestHealthRouteRegistered(t *testing.T) {
	h := newRouterServer(t)
	w := ut.PerformRequest(h.Engine, "GET", "/health", nil)
	if got := w.Code; got != 200 {
		t.Fatalf("GET /health 状态码 = %d，期望 200", got)
	}
}

// TestOldInitDatabaseRouteRemoved 确认旧的多合一初始化路由已经不存在。
func TestOldInitDatabaseRouteRemoved(t *testing.T) {
	h := newRouterServer(t)

	methods := []string{"GET", "POST", "PUT", "DELETE"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			w := ut.PerformRequest(h.Engine, method, "/init/database", nil)
			if got := w.Code; got != 404 {
				t.Fatalf("%s /init/database 状态码 = %d，期望 404", method, got)
			}
		})
	}
}

// TestNoOtherInitRoutes 只允许白名单内的初始化路由，避免把任意路径当作服务名。
func TestNoOtherInitRoutes(t *testing.T) {
	h := newRouterServer(t)

	for _, path := range []string{
		"/init/services/system-user/extra",
		"/init/service/system-user",
		"/init",
	} {
		w := ut.PerformRequest(h.Engine, "POST", path, nil)
		if got := w.Code; got != 404 {
			t.Fatalf("POST %s 状态码 = %d，期望 404", path, got)
		}
	}
}
