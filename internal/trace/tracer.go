package trace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/internal/log"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
)

// ServiceName 是 Fx 容器内注入的服务名（避免与其它 string 依赖冲突）。
type ServiceName string

const (
	// errLogInterval 是同一错误的最小日志间隔，防止后端不可达时刷屏。
	errLogInterval = time.Minute
	// defaultShutdownTimeout 是配置缺省时的关闭超时。
	defaultShutdownTimeout = 5 * time.Second
)

// enabledFlag 记录开关状态：只给装配期与测试断言用，不在请求路径上判断。
var enabledFlag atomic.Bool

// Enabled 报告全局链路追踪是否已启用（装配期与测试用；请求路径不读它）。
func Enabled() bool { return enabledFlag.Load() }

// Tracer 持有已安装的 SDK TracerProvider；未启用时 provider 为 nil。
type Tracer struct {
	provider        *sdktrace.TracerProvider
	shutdownTimeout time.Duration
}

// NewTracer 安装全局 TracerProvider / W3C Propagator / ErrorHandler。
// trace.enabled=false 时什么都不做，仅返回可注入的空壳。
func NewTracer(lc fx.Lifecycle, cfg *conf.Config, serviceName ServiceName) (*Tracer, error) {
	c := cfg.Trace
	if !c.Enabled {
		logrus.WithField("module", log.ModuleTrace).Info("trace disabled (trace.enabled=false)")
		return &Tracer{}, nil
	}
	if c.SampleRatio < 0 || c.SampleRatio > 1 {
		return nil, fmt.Errorf("trace.sample_ratio=%v 超出 [0,1]", c.SampleRatio)
	}

	shutdownTimeout := c.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultShutdownTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), exportTimeout(c))
	defer cancel()

	exp, err := newOTLPExporter(ctx, c)
	if err != nil {
		return nil, err
	}
	provider, err := newTracerProvider(c, string(serviceName), exp)
	if err != nil {
		return nil, err
	}

	t := &Tracer{provider: provider, shutdownTimeout: shutdownTimeout}
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetErrorHandler(newDedupHandler(errLogInterval))
	enabledFlag.Store(true)

	lc.Append(fx.Hook{OnStop: func(context.Context) error { return t.shutdown() }})

	logrus.WithFields(logrus.Fields{
		"module":       log.ModuleTrace,
		"service":      string(serviceName),
		"endpoint":     c.Endpoint,
		"sample_ratio": c.SampleRatio,
	}).Info("trace exporter installed")
	return t, nil
}

// exportTimeout 返回导出超时，缺省回落到 defaultExportTimeout。
func exportTimeout(c conf.TraceConf) time.Duration {
	if c.ExportTimeout > 0 {
		return c.ExportTimeout
	}
	return defaultExportTimeout
}

// shutdown 先 Flush 队列再关闭 provider，两步都用 shutdownTimeout 兜底。
func (t *Tracer) shutdown() error {
	if t.provider == nil {
		return nil
	}
	enabledFlag.Store(false)

	flushCtx, cancel := context.WithTimeout(context.Background(), t.shutdownTimeout)
	defer cancel()
	if err := t.provider.ForceFlush(flushCtx); err != nil {
		logrus.WithError(err).WithField("module", log.ModuleTrace).Warn("trace force flush failed, some spans may be dropped")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), t.shutdownTimeout)
	defer cancelShutdown()
	if err := t.provider.Shutdown(shutdownCtx); err != nil {
		logrus.WithError(err).WithField("module", log.ModuleTrace).Warn("trace provider shutdown failed")
		return err
	}
	return nil
}

// dedupHandler 把 OTel 内部错误转日志，并对同一消息限流。
type dedupHandler struct {
	mu       sync.Mutex
	interval time.Duration
	last     map[string]time.Time
}

func newDedupHandler(interval time.Duration) *dedupHandler {
	return &dedupHandler{interval: interval, last: make(map[string]time.Time)}
}

// Handle 实现 otel.ErrorHandler。
func (h *dedupHandler) Handle(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	now := time.Now()
	h.mu.Lock()
	if last, ok := h.last[msg]; ok && now.Sub(last) < h.interval {
		h.mu.Unlock()
		return
	}
	h.last[msg] = now
	if len(h.last) > 64 {
		for k, v := range h.last {
			if now.Sub(v) >= h.interval {
				delete(h.last, k)
			}
		}
	}
	h.mu.Unlock()
	logrus.WithError(err).WithField("module", log.ModuleTrace).Warn("opentelemetry internal error")
}
