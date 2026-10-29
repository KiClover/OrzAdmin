package constructor

import (
	"context"
	"fmt"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/internal/middleware"
	internaltrace "github.com/KiClover/orzAdmin/internal/trace"
	"github.com/KiClover/orzAdmin/pkg/utils"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/app/server/registry"
	"github.com/cloudwego/hertz/pkg/common/config"
	hertzotel "github.com/hertz-contrib/obs-opentelemetry/tracing"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

type NewHzServiceFunc func(options ...config.Option) *server.Hertz
type HzServiceRunnerFunc[T any] func(ctx context.Context, lc fx.Lifecycle, serviceName string, cfg *conf.Config)

func RunHzServiceP[T any](Router T, options ...config.Option) HzServiceRunnerFunc[T] {
	return func(ctx context.Context, lc fx.Lifecycle, serviceName string, cfg *conf.Config) {
		ctx, span := tracer.Start(ctx, fmt.Sprintf("bootstrap.RunHzService:%s", serviceName))
		defer span.End()

		options = append(
			options,
			server.WithHostPorts(fmt.Sprintf("%s:%d", cfg.SystemConf.Addr, cfg.SystemConf.Port)),
		)
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				ctx, span = tracer.Start(ctx, fmt.Sprintf("start-service:%s", serviceName))
				defer span.End()
				h := server.New(options...)
				//Router(h)
				h.Spin()
				return nil
			},
		})
	}
}

func NewHzService(ctx context.Context, serviceName string, reg registry.Registry, cfg *conf.Config, options ...config.Option) *server.Hertz {
	ctx, span := tracer.Start(ctx, fmt.Sprintf("bootstrap.RunHzService:%s", serviceName))
	defer span.End()

	ip, err := utils.GetOutboundIP()
	if err != nil {
		logrus.WithContext(ctx).WithError(err).Panic("get local ip failed")
	}
	registryInfo := &registry.Info{
		ServiceName: serviceName,
		Addr:        utils.NewNetAddr("tcp", fmt.Sprintf("%s:%d", ip, cfg.SystemConf.Port)),
		Weight:      10,
		Tags:        nil,
	}
	options = append(
		options,
		server.WithRegistry(reg, registryInfo),
		server.WithHostPorts(fmt.Sprintf("%s:%d", cfg.SystemConf.Addr, cfg.SystemConf.Port)),
	)

	// 官方中间件要通过 server.WithTracer 注入 TraceCarrier，缺了它会静默不出 Span。
	tracerOpt, tracingCfg := hertzotel.NewServerTracer(
		hertzotel.WithCustomResponseHandler(internaltrace.ResponseTraceID),
	)
	options = append(options, tracerOpt)

	h := server.New(options...)
	h.Use(middleware.NewCORSMw())
	h.Use(hertzotel.ServerMiddleware(tracingCfg))
	return h
}

func RunHzService(h *server.Hertz) {
	h.Spin()
}
