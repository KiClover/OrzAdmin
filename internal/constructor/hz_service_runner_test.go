package constructor

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/cloudwego/hertz/pkg/app"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// ut.PerformRequest 不经过 Hertz 的 tracer 生命周期（stats 为 LevelDisabled，
// 中间件直接放行、既不出 Span 也不跑响应回调），所以这里用真实监听验证接线。
func TestNewHzServiceEmitsServerSpan(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exp))))
	t.Cleanup(func() { otel.SetTracerProvider(tracenoop.NewTracerProvider()) })

	port := freePort(t)
	cfg := &conf.Config{}
	cfg.SystemConf.Addr = "127.0.0.1"
	cfg.SystemConf.Port = port

	h := NewHzService(context.Background(), "test.service", nil, cfg)
	h.GET("/ping", func(ctx context.Context, c *app.RequestContext) { c.String(200, "pong") })

	go h.Spin()

	url := fmt.Sprintf("http://127.0.0.1:%d/ping", port)
	var resp *http.Response
	var err error
	for i := 0; i < 100; i++ {
		if resp, err = http.Get(url); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	var serverSpan *tracetest.SpanStub
	// Span 在 Finish 阶段才 End/导出，可能晚于响应到达，这里轮询等待。
	for i := 0; i < 100 && serverSpan == nil; i++ {
		spans := exp.GetSpans()
		for j := range spans {
			if spans[j].Name == "GET /ping" {
				s := spans[j]
				serverSpan = &s
				break
			}
		}
		if serverSpan == nil {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if serverSpan == nil {
		names := make([]string, 0, len(exp.GetSpans()))
		for _, s := range exp.GetSpans() {
			names = append(names, s.Name)
		}
		t.Fatalf("没有 HTTP server span（已有 spans=%v；为空通常意味着漏了 tracerOpt）", names)
	}
	if got := resp.Header.Get("X-Trace-Id"); got != serverSpan.SpanContext.TraceID().String() {
		t.Fatalf("X-Trace-Id = %q, server span trace id = %s", got, serverSpan.SpanContext.TraceID())
	}
}

// freePort 从内核借一个空闲端口，避免固定端口在不同用例间冲突。
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
