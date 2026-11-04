package log

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	testTraceIDHex = "0102030405060708090a0b0c0d0e0f10"
	testSpanIDHex  = "0102030405060708"
)

// ctxWithSpan 构造携带有效 SpanContext 的 ctx（不需要 SDK Provider）。
func ctxWithSpan(t *testing.T) context.Context {
	t.Helper()
	tid, err := oteltrace.TraceIDFromHex(testTraceIDHex)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := oteltrace.SpanIDFromHex(testSpanIDHex)
	if err != nil {
		t.Fatal(err)
	}
	sc := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: oteltrace.FlagsSampled,
	})
	return oteltrace.ContextWithSpanContext(context.Background(), sc)
}

func TestTraceHookInjectsTraceAndSpanID(t *testing.T) {
	entry := newEntry(logrus.Fields{}, "rpc failed").WithContext(ctxWithSpan(t))
	if err := NewTraceHook().Fire(entry); err != nil {
		t.Fatal(err)
	}
	if got := entry.Data[FieldTraceID]; got != testTraceIDHex {
		t.Fatalf("trace_id = %v, want %s", got, testTraceIDHex)
	}
	if got := entry.Data[FieldSpanID]; got != testSpanIDHex {
		t.Fatalf("span_id = %v, want %s", got, testSpanIDHex)
	}
}

func TestTraceHookWithoutContextKeepsEntryUntouched(t *testing.T) {
	entry := newEntry(logrus.Fields{"module": ModuleApp}, "startup")
	if err := NewTraceHook().Fire(entry); err != nil {
		t.Fatal(err)
	}
	if _, ok := entry.Data[FieldTraceID]; ok {
		t.Fatal("无 Context 的日志不应注入 trace_id")
	}
	if _, ok := entry.Data[FieldSpanID]; ok {
		t.Fatal("无 Context 的日志不应注入 span_id")
	}
}

func TestTraceHookIgnoresInvalidSpanContext(t *testing.T) {
	// context.Background() 上的 SpanContext 全零（追踪未启用/未采样），不应产生假关联。
	entry := newEntry(logrus.Fields{}, "no span").WithContext(context.Background())
	if err := NewTraceHook().Fire(entry); err != nil {
		t.Fatal(err)
	}
	if _, ok := entry.Data[FieldTraceID]; ok {
		t.Fatal("无效 SpanContext 不应注入 trace_id")
	}
	if _, ok := entry.Data[FieldSpanID]; ok {
		t.Fatal("无效 SpanContext 不应注入 span_id")
	}
}

func TestTraceHookDoesNotOverwriteCallerFields(t *testing.T) {
	// RequestLogFields 等白名单已写入的 trace_id 必须保留，Hook 只补缺。
	entry := newEntry(logrus.Fields{FieldTraceID: "caller-provided"}, "dup").WithContext(ctxWithSpan(t))
	if err := NewTraceHook().Fire(entry); err != nil {
		t.Fatal(err)
	}
	if got := entry.Data[FieldTraceID]; got != "caller-provided" {
		t.Fatalf("trace_id = %v, want caller-provided（Hook 不应覆盖调用方字段）", got)
	}
	if got := entry.Data[FieldSpanID]; got != testSpanIDHex {
		t.Fatalf("span_id = %v, want %s（缺失字段仍应补齐）", got, testSpanIDHex)
	}
}

func TestTraceHookInjectsServiceIdentity(t *testing.T) {
	SetServiceIdentity("svc-a", "dev", "1.2.3")
	t.Cleanup(func() { SetServiceIdentity("", "", "") })

	entry := newEntry(logrus.Fields{}, "startup")
	if err := NewTraceHook().Fire(entry); err != nil {
		t.Fatal(err)
	}
	if got := entry.Data[FieldService]; got != "svc-a" {
		t.Fatalf("service = %v, want svc-a", got)
	}
	if got := entry.Data[FieldEnvironment]; got != "dev" {
		t.Fatalf("environment = %v, want dev", got)
	}
	if got := entry.Data[FieldVersion]; got != "1.2.3" {
		t.Fatalf("version = %v, want 1.2.3", got)
	}
}

func TestTraceHookSkipsEmptyIdentityAndKeepsCallerValue(t *testing.T) {
	SetServiceIdentity("", "", "")
	entry := newEntry(logrus.Fields{FieldService: "custom"}, "startup")
	if err := NewTraceHook().Fire(entry); err != nil {
		t.Fatal(err)
	}
	if got := entry.Data[FieldService]; got != "custom" {
		t.Fatalf("service = %v, want custom（空身份不应覆盖调用方字段）", got)
	}
	if _, ok := entry.Data[FieldEnvironment]; ok {
		t.Fatal("空 environment 不应注入")
	}
	if _, ok := entry.Data[FieldVersion]; ok {
		t.Fatal("空 version 不应注入")
	}
}

func TestInstallTraceHookIsIdempotent(t *testing.T) {
	logger := logrus.New()
	installTraceHook(logger)
	installTraceHook(logger)
	// logrus 按级别存 Hook（AllLevels=7），同一实例会出现 7 次，按指针去重计数。
	seen := map[*TraceHook]bool{}
	for _, hooks := range logger.Hooks {
		for _, h := range hooks {
			if th, ok := h.(*TraceHook); ok {
				seen[th] = true
			}
		}
	}
	if len(seen) != 1 {
		t.Fatalf("TraceHook 安装了 %d 个实例, want 1（重复构造不能叠加 Hook）", len(seen))
	}
}

// TestTraceHookEndToEnd 走完整 Logger 输出路径：WithContext + JSONFormatter，
// 断言落盘文本里带 trace_id/span_id，证明"日志可跳转 Trace"真实成立。
func TestTraceHookEndToEnd(t *testing.T) {
	logger := logrus.New()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})
	installTraceHook(logger)

	logger.WithContext(ctxWithSpan(t)).Error("db query failed")

	out := buf.String()
	if !strings.Contains(out, `"trace_id":"`+testTraceIDHex+`"`) {
		t.Fatalf("输出缺少 trace_id: %s", out)
	}
	if !strings.Contains(out, `"span_id":"`+testSpanIDHex+`"`) {
		t.Fatalf("输出缺少 span_id: %s", out)
	}
}

// TestWithContextReturnsContextualEntry 保证统一入口与 logrus.WithContext 行为一致，
// TraceHook 能从中取到 SpanContext。
func TestWithContextReturnsContextualEntry(t *testing.T) {
	entry := WithContext(ctxWithSpan(t))
	if entry.Context == nil {
		t.Fatal("WithContext 返回的 Entry 必须携带 Context")
	}
	if err := NewTraceHook().Fire(entry); err != nil {
		t.Fatal(err)
	}
	if got := entry.Data[FieldTraceID]; got != testTraceIDHex {
		t.Fatalf("trace_id = %v, want %s", got, testTraceIDHex)
	}
}
