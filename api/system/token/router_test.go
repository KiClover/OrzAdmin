package token

import (
	"net/http"
	"testing"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

// 旧 /init/database 入口必须消失：数据库初始化只保留 api/initialize 的 POST /init/services[:service]。
func TestLegacyInitDatabaseRouteRemoved(t *testing.T) {
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	NewRegisterRouter(h, &HzSystemTokenService{Cfg: &conf.Config{}})

	w := ut.PerformRequest(h.Engine, http.MethodPost, "/init/database", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("POST /init/database 状态码 = %d，期望 404", w.Code)
	}

	health := ut.PerformRequest(h.Engine, http.MethodGet, "/health", nil)
	if health.Code != http.StatusOK {
		t.Fatalf("GET /health 状态码 = %d，期望 200", health.Code)
	}
}
