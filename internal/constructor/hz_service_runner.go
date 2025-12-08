package constructor

import (
	"context"
	"fmt"

	"github.com/KiClover/orzfrp/conf"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/config"
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
				Router(h)
				h.Spin()
				return nil
			},
		})
	}
}

func NewHzService(ctx context.Context, serviceName string, cfg *conf.Config, options ...config.Option) *server.Hertz {
	ctx, span := tracer.Start(ctx, fmt.Sprintf("bootstrap.RunHzService:%s", serviceName))
	defer span.End()

	options = append(
		options,
		server.WithHostPorts(fmt.Sprintf("%s:%d", cfg.SystemConf.Addr, cfg.SystemConf.Port)),
	)
	h := server.New(options...)
	return h
}

func RunHzService(h *server.Hertz) {
	h.Spin()
}
