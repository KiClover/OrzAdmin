package initialize

import (
	"fmt"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/internal/health"
	"github.com/KiClover/orzAdmin/internal/middleware"
	internaltrace "github.com/KiClover/orzAdmin/internal/trace"
	"github.com/cloudwego/hertz/pkg/app/server"
	hertzconfig "github.com/cloudwego/hertz/pkg/common/config"
	hertzotel "github.com/hertz-contrib/obs-opentelemetry/tracing"
)

// NewRegisterRouter 注册初始化 API 的全部路由。
//
// 初始化 API 只接受本机或受信内网访问，不挂 JWT / Casbin，也不接受初始化密钥；
// 旧的多合一 /init/database 路由刻意不保留。
func NewRegisterRouter(h *server.Hertz, svc *HzInitializeService) {
	root := h.Group("/")
	root.GET("/health", health.Health)

	initGroup := root.Group("/init")
	initGroup.POST("/services", svc.InitAll)
	initGroup.POST("/services/:service", svc.InitService)
}

// newHertzServer 构造初始化 API 的 Hertz 实例：监听地址来自配置，
// 不注册服务发现，因此不会加载 etcd / nacos。
func newHertzServer(cfg *conf.Config) *server.Hertz {
	tracerOpt, tracingCfg := hertzotel.NewServerTracer(
		hertzotel.WithCustomResponseHandler(internaltrace.ResponseTraceID),
	)
	options := []hertzconfig.Option{
		server.WithHostPorts(fmt.Sprintf("%s:%d", cfg.SystemConf.Addr, cfg.SystemConf.Port)),
		tracerOpt,
	}
	h := server.New(options...)
	h.Use(middleware.NewCORSMw())
	h.Use(hertzotel.ServerMiddleware(tracingCfg))
	return h
}
