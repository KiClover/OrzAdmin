package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/cloudwego/hertz/pkg/app"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// TraceIDHeader 是回给调用方的请求标识响应头。
const TraceIDHeader = "X-Trace-Id"

// ResponseTraceID 把本次请求的 trace id 写进响应头；
// 没有有效 Span（追踪未启用或未被采样）时写入本地随机 id，保证报障时总有一个可检索的标识。
func ResponseTraceID(ctx context.Context, c *app.RequestContext) {
	if sc := oteltrace.SpanFromContext(ctx).SpanContext(); sc.HasTraceID() {
		c.Response.Header.Set(TraceIDHeader, sc.TraceID().String())
		return
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return
	}
	c.Response.Header.Set(TraceIDHeader, hex.EncodeToString(b[:]))
}
