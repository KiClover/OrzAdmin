package log

import (
	"context"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// 日志与 Trace 关联使用的标准字段名（LoggingAuditReport §3.8）。
const (
	FieldTraceID     = "trace_id"
	FieldSpanID      = "span_id"
	FieldService     = "service"
	FieldEnvironment = "environment"
	FieldVersion     = "version"
)

// serviceIdentity 是进程级静态身份，由 NewTracer 在装配期写入一次。
// 用 atomic.Value 而不是包级变量直写：Hook 在请求路径上并发读，装配期只写一次。
type serviceIdentity struct {
	service     string
	environment string
	version     string
}

var identity atomic.Value // 存 serviceIdentity；未写入时 Load 返回 nil

// SetServiceIdentity 设置进程级服务身份，TraceHook 会把它注入每条日志。
// 空字符串字段会被跳过；测试结束后用空值调用即可还原。
func SetServiceIdentity(service, environment, version string) {
	identity.Store(serviceIdentity{service: service, environment: environment, version: version})
}

// TraceHook 把日志与调用链关联起来（LoggingAuditReport §3.8）：
//  1. Entry 携带的 Context 里有有效 SpanContext 时，注入 trace_id / span_id——
//     从错误日志可以跳到同一条 Trace，从 Trace 也能反查该 Span 下的业务日志；
//  2. 每条日志注入进程级静态身份 service / environment / version，集中检索时可直接聚合。
//
// Hook 不覆盖调用方已显式设置的同名字段（例如 RequestLogFields 已写入的 trace_id）。
// 无状态（身份只读），与 SanitizeHook 一样可安全并发。
type TraceHook struct{}

// NewTraceHook 创建关联 Hook，供 NewLogger 安装到全局 Logger。
func NewTraceHook() *TraceHook { return &TraceHook{} }

// Levels 对所有级别生效：关联能力与级别无关。
func (h *TraceHook) Levels() []logrus.Level { return logrus.AllLevels }

// Fire 在 Entry 落到 Formatter/Writer 之前注入关联字段。
func (h *TraceHook) Fire(entry *logrus.Entry) error {
	if entry == nil {
		return nil
	}
	if entry.Data == nil {
		entry.Data = logrus.Fields{}
	}

	if id, ok := identity.Load().(serviceIdentity); ok {
		setFieldIfAbsent(entry, FieldService, id.service)
		setFieldIfAbsent(entry, FieldEnvironment, id.environment)
		setFieldIfAbsent(entry, FieldVersion, id.version)
	}

	// 只有调用方用 WithContext 携带了请求上下文时才可能有 SpanContext；
	// 无效/未采样的 SpanContext（全零 ID）不注入，避免产出无意义的假关联。
	if entry.Context != nil {
		sc := oteltrace.SpanFromContext(entry.Context).SpanContext()
		if sc.HasTraceID() {
			setFieldIfAbsent(entry, FieldTraceID, sc.TraceID().String())
		}
		if sc.HasSpanID() {
			setFieldIfAbsent(entry, FieldSpanID, sc.SpanID().String())
		}
	}
	return nil
}

// setFieldIfAbsent 只在值非空且调用方未设置同名字段时写入。
func setFieldIfAbsent(entry *logrus.Entry, key, value string) {
	if value == "" {
		return
	}
	if _, exists := entry.Data[key]; exists {
		return
	}
	entry.Data[key] = value
}

// installTraceHook 幂等安装：同一 Logger 重复构造不会叠加多个 Hook。
func installTraceHook(logger *logrus.Logger) {
	if logger == nil {
		return
	}
	for _, hooks := range logger.Hooks {
		for _, hook := range hooks {
			if _, ok := hook.(*TraceHook); ok {
				return
			}
		}
	}
	logger.AddHook(NewTraceHook())
}

// WithContext 是业务代码记录请求/RPC 相关日志的统一入口：
// 返回携带 ctx 的 Entry，TraceHook 会从中提取有效 SpanContext 并注入 trace_id/span_id。
// 与请求无关的日志（启动、定时任务）直接用包级 logrus.* 或 WithModule 即可。
// 与 WithModule / WithError 链式兼容：log.WithModule(log.ModuleX).WithContext(ctx)。
func WithContext(ctx context.Context) *logrus.Entry {
	return logrus.WithContext(ctx)
}
