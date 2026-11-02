package middleware

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/sirupsen/logrus"
)

// captureStandardLog 把全局 logrus 输出重定向到 buffer，测试结束后恢复。
func captureStandardLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	std := logrus.StandardLogger()
	origOut, origFormatter := std.Out, std.Formatter
	var buf bytes.Buffer
	std.SetOutput(&buf)
	std.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: true})
	t.Cleanup(func() {
		std.SetOutput(origOut)
		std.SetFormatter(origFormatter)
	})
	return &buf
}

// TestUseCasbinMwMissingActorDoesNotLogSecrets 覆盖 LoggingAuditReport §3.5 的原始缺陷：
// 缺 userId 分支曾用 `%v` 打印完整 *protocol.Request，把请求头与请求体整体写进日志。
func TestUseCasbinMwMissingActorDoesNotLogSecrets(t *testing.T) {
	buf := captureStandardLog(t)

	const (
		headerJWT    = "header-jwt-value-123"
		headerSecret = "sk_live_secret_header"
		cookieValue  = "sessionid=cookie-secret-value"
		bodyPassword = "{{SECRET_tu5etmhm}}"
		queryToken   = "{{SECRET_hyevxxej}}"
	)
	c := app.NewContext(0)
	c.Request.SetMethod("GET")
	c.Request.SetRequestURI("/api/system/user/list?token=" + queryToken)
	c.Request.Header.Set("Authorization", "Bearer "+headerJWT)
	c.Request.Header.Set("SecretKey", headerSecret)
	c.Request.Header.Set("Cookie", cookieValue)
	c.Request.SetBodyString(`{"password":"` + bodyPassword + `","code":"123456"}`)

	NewCasbinMw(&conf.Config{}, nil).UseCasbinMw(context.Background(), c)

	if got := c.Response.StatusCode(); got != 401 {
		t.Fatalf("status = %d, want 401", got)
	}
	out := buf.String()
	for name, secret := range map[string]string{
		"Authorization": headerJWT,
		"SecretKey":     headerSecret,
		"Cookie":        cookieValue,
		"请求体密码":         bodyPassword,
		"query token":   queryToken,
		"验证码":           "123456",
	} {
		if strings.Contains(out, secret) {
			t.Fatalf("日志泄露了 %s: %s", name, out)
		}
	}
	if !strings.Contains(out, "get user id from context failed") {
		t.Fatalf("缺少诊断日志: %s", out)
	}
	// 白名单字段仍必须齐全，否则日志失去排障价值。
	for _, want := range []string{"method=GET", "route=/api/system/user/list", "status=401", "module=AUTH"} {
		if !strings.Contains(out, want) {
			t.Fatalf("缺少白名单字段 %q: %s", want, out)
		}
	}
}

// TestRequestLogFieldsWhitelist 守护白名单本身：只能出现结构性字段，且不含 query 原文。
func TestRequestLogFieldsWhitelist(t *testing.T) {
	c := app.NewContext(0)
	c.Request.SetMethod("POST")
	c.Request.SetRequestURI("/api/auth/login?next=/dashboard")
	c.Request.Header.Set("Authorization", "Bearer should-not-appear")
	c.Request.SetBodyString(`{"password":"should-not-appear"}`)

	fields := RequestLogFields(context.Background(), c, 200)
	for _, banned := range []string{"authorization", "cookie", "header", "body", "url", "query", "request"} {
		if _, bad := fields[banned]; bad {
			t.Fatalf("白名单出现了字段 %q: %v", banned, fields)
		}
	}
	if fields["method"] != "POST" || fields["status"] != 200 {
		t.Fatalf("fields = %v", fields)
	}
	if route, _ := fields["route"].(string); strings.Contains(route, "next=") || route == "" {
		t.Fatalf("route 不应包含 query 原文: %v", fields["route"])
	}
}
