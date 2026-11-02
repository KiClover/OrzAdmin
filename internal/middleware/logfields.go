package middleware

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/sirupsen/logrus"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// RequestLogFields 返回 HTTP 请求诊断的白名单字段。
//
// 只允许路由此类"结构性"信息进入日志：method、route（路由模板而非完整 URL）、status、
// client_ip，以及可用的 trace_id。绝不返回 header、query、body、完整 URL——
// Authorization、SecretKey、Cookie、密码、验证码都在这些位置，日志不是它们的存放地。
//
// 新增字段前先确认它不是外部输入原文；需要新字段时优先加在 trace span 上。
func RequestLogFields(ctx context.Context, c *app.RequestContext, status int) logrus.Fields {
	fields := logrus.Fields{
		"method":    string(c.Method()),
		"route":     routeTemplate(c),
		"status":    status,
		"client_ip": c.ClientIP(),
	}
	if sc := oteltrace.SpanFromContext(ctx).SpanContext(); sc.HasTraceID() {
		fields["trace_id"] = sc.TraceID().String()
	}
	return fields
}

// routeTemplate 优先返回注册的路由模板（/api/user/:id），避免把路径参数与 query 原文写进日志；
// 路由尚未匹配时退回请求路径本身。
func routeTemplate(c *app.RequestContext) string {
	if full := c.FullPath(); full != "" {
		return full
	}
	return string(c.Path())
}
