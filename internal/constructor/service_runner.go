package constructor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/pkg/utils"
	"github.com/cloudwego/kitex/pkg/kerrors"
	"github.com/cloudwego/kitex/pkg/limit"
	"github.com/cloudwego/kitex/pkg/registry"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	"github.com/kitex-contrib/obs-opentelemetry/tracing"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.uber.org/fx"
)

var tracer = otel.Tracer("github.com/KiClover/orzAdmin")

type NewServiceFunc[T any] func(handler T, opts ...server.Option) server.Server
type ServiceRunnerFunc[T any] func(ctx context.Context, handler T, lc fx.Lifecycle, serviceName string, reg registry.Registry, cfg *conf.Config)

func RunService[T any](serviceFunc NewServiceFunc[T], options ...server.Option) ServiceRunnerFunc[T] {
	return func(ctx context.Context, handler T, lc fx.Lifecycle, serviceName string, reg registry.Registry, cfg *conf.Config) {
		ctx, span := tracer.Start(ctx, fmt.Sprintf("bootstrap.RunService:%s", serviceName))
		defer span.End()

		ip, err := utils.GetLocalIP()
		if err != nil {
			logrus.WithContext(ctx).WithError(err).Panic("get local ip failed")
		}
		registryInfo := &registry.Info{
			ServiceName: serviceName,
			Addr:        utils.NewNetAddr(cfg.SystemConf.Network, ip),
			Weight:      10,
			StartTime:   time.Now(),
			WarmUp:      10 * time.Second,
		}

		options = append(
			options,
			server.WithRegistry(reg),
			server.WithRegistryInfo(registryInfo),
			server.WithSuite(tracing.NewServerSuite()),
			server.WithServiceAddr(utils.NewNetAddr(cfg.SystemConf.Network, fmt.Sprintf("%s:%d", cfg.SystemConf.Addr, cfg.SystemConf.Port))),
			// Windows不支持NetPoll Windows条件下需要注释该参数
			//server.WithMuxTransport(),
			server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
			server.WithLimit(&limit.Option{MaxConnections: 10000, MaxQPS: 5000}),
			server.WithErrorHandler(newServerErrorHandler(serviceName)),
		)

		svr := serviceFunc(handler, options...)

		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				ctx, span := tracer.Start(ctx, fmt.Sprintf("start-service:%s", serviceName))
				defer span.End()

				go func() {
					if err := svr.Run(); err != nil {
						logrus.WithContext(ctx).WithError(err).Panicf("run %s server failed", serviceName)
						return
					}
				}()

				return nil
			},
			OnStop: func(ctx context.Context) error {
				ctx, span := tracer.Start(ctx, fmt.Sprintf("stop-service:%s", serviceName))
				defer span.End()

				if err := svr.Stop(); err != nil {
					span.RecordError(err)
					logrus.WithContext(ctx).WithError(err).Errorf("stop %s server failed", serviceName)
					return err
				}

				return nil
			},
		})
	}
}

// newServerErrorHandler 构造 Kitex 服务端错误处理器。
// 解包后的根因只用于 Span 记录，对外仍返回原始错误，以保留 Kitex 错误分类与业务状态码。
func newServerErrorHandler(serviceName string) func(ctx context.Context, err error) error {
	return func(ctx context.Context, err error) error {
		_, span := tracer.Start(ctx, fmt.Sprintf("ServerError:%s", serviceName))
		defer span.End()

		span.RecordError(unwrapKitexError(err))
		return err
	}
}

// unwrapKitexError 逐层剥离 Kitex DetailedError 包装，返回用于诊断的根因错误。
// 循环每轮都必须向根因推进一步，否则立即终止，避免错误处理器被单个请求永久挂起：
// 非 Kitex 错误、非目标错误类型、cause 为空或 errors.As 失败都返回原始错误。
func unwrapKitexError(err error) error {
	current := err
	for kerrors.IsKitexError(current) {
		var de *kerrors.DetailedError
		if !errors.As(current, &de) {
			return err
		}

		switch de.ErrorType() {
		case kerrors.ErrBiz, kerrors.ErrRemoteOrNetwork:
			cause := de.Unwrap()
			if cause == nil {
				return err
			}
			current = cause
		default:
			return err
		}
	}

	return current
}
