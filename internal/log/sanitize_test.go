package log

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"go.uber.org/fx/fxtest"
)

func newEntry(data logrus.Fields, msg string) *logrus.Entry {
	return &logrus.Entry{Logger: logrus.New(), Data: data, Message: msg, Level: logrus.ErrorLevel}
}

func TestSanitizeHookRedactsSensitiveDataKeys(t *testing.T) {
	sensitive := map[string]string{
		"authorization": "Bearer header-token-value",
		"SecretKey":     "sk_live_secret",
		"X-Secret-Key":  "sk_live_header_secret",
		"secret_key":    "sk_live_snake",
		"user_password": "p@ssw0rd",
		"new_password":  "n3w",
		"access_token":  "tok-access",
		"Cookie":        "sessionid=cookie-value",
		"captcha":       "9876",
		"code":          "654321",
		"client_secret": "cs-value",
		"api_key":       "api-key-value",
	}
	data := logrus.Fields{
		"error_code":  1001,
		"route":       "/api/system/user/:id",
		"status":      401,
		"user_id":     uint64(10086),
		"token_count": 3,
	}
	for k, v := range sensitive {
		data[k] = v
	}

	entry := newEntry(data, "rbac denied")
	if err := NewSanitizeHook().Fire(entry); err != nil {
		t.Fatal(err)
	}

	for k := range sensitive {
		if got := entry.Data[k]; got != Redacted {
			t.Fatalf("敏感字段 %q = %v, want %q", k, got, Redacted)
		}
	}
	// 非凭据字段必须原样保留，否则日志失去排障价值。
	if got := entry.Data["error_code"]; got != 1001 {
		t.Fatalf("error_code = %v, want 1001（业务错误码不是凭据）", got)
	}
	if got := entry.Data["route"]; got != "/api/system/user/:id" {
		t.Fatalf("route = %v", got)
	}
	if got := entry.Data["token_count"]; got != 3 {
		t.Fatalf("token_count = %v, want 3（统计字段不应被当作 token）", got)
	}
	if got := entry.Data["user_id"]; got != uint64(10086) {
		t.Fatalf("user_id = %v", got)
	}
}

func TestSanitizeText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string // 输出中不得出现
	}{
		{"bearer header", `auth failed Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig`, []string{"eyJhbGciOiJIUzI1NiJ9.payload.sig"}},
		{"secret key kv", `SecretKey=sk_live_9f8e7d6c`, []string{"sk_live_9f8e7d6c"}},
		{"json body", `login req {"password":"p@ssw0rd","code":"123456"}`, []string{"p@ssw0rd", "123456"}},
		{"query token", `/api/user?token=query-token-value&page=1`, []string{"query-token-value"}},
		{"cookie header", `Cookie: sessionid=abc123; path=/`, []string{"abc123"}},
		{"bare jwt", `parse token err: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signature`, []string{"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signature"}},
		{"nested key", `cfg {"db":{"password":"db-secret"}}`, []string{"db-secret"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeText(tc.in)
			for _, leak := range tc.want {
				if strings.Contains(got, leak) {
					t.Fatalf("sanitizeText(%q) = %q, 仍包含 %q", tc.in, got, leak)
				}
			}
			if !strings.Contains(got, Redacted) {
				t.Fatalf("sanitizeText(%q) = %q, 未出现脱敏占位符", tc.in, got)
			}
		})
	}
}

func TestSanitizeTextKeepsNormalMessages(t *testing.T) {
	for _, msg := range []string{
		"database connected",
		"request bind err: JSON parse error",
		"get user id from context failed",
		"crypt password err: bcrypt: hashedSecret is not the hash of the given password",
		"casbin enforce err: rule not found",
	} {
		if got := sanitizeText(msg); got != msg {
			t.Fatalf("sanitizeText(%q) = %q, 普通日志不应被改写", msg, got)
		}
	}
}

func TestSanitizeTextIsIdempotent(t *testing.T) {
	const in = `Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig password="p@ss"`
	once := sanitizeText(in)
	twice := sanitizeText(once)
	if once != twice {
		t.Fatalf("脱敏非幂等: once=%q twice=%q", once, twice)
	}
}

func TestSanitizeHookNestedMapAndError(t *testing.T) {
	entry := newEntry(logrus.Fields{
		"request": map[string]any{
			"headers": map[string]any{"Authorization": "Bearer nested-token"},
			"body":    `{"password":"nested-password"}`,
			"route":   "/api/login",
		},
		"raw_headers": []string{"Content-Type: text/plain", "Authorization: Bearer slice-token"},
		"error":       errors.New(`parse token err: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig`),
	}, "")

	// 无泄露迹象的 error 保持原类型（Formatter 行为不变）。
	clean := newEntry(logrus.Fields{"error": errors.New("connection refused")}, "")
	if err := NewSanitizeHook().Fire(clean); err != nil {
		t.Fatal(err)
	}
	if _, ok := clean.Data["error"].(error); !ok {
		t.Fatalf("干净 error 被改成 %T，应保持 error", clean.Data["error"])
	}

	if err := NewSanitizeHook().Fire(entry); err != nil {
		t.Fatal(err)
	}
	req, ok := entry.Data["request"].(map[string]any)
	if !ok {
		t.Fatalf("request = %T", entry.Data["request"])
	}
	headers, ok := req["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers = %T", req["headers"])
	}
	if got := headers["Authorization"]; got != Redacted {
		t.Fatalf("嵌套 Authorization = %v, want %q", got, Redacted)
	}
	if body, _ := req["body"].(string); strings.Contains(body, "nested-password") {
		t.Fatalf("嵌套 body 未脱敏: %q", body)
	}
	if req["route"] != "/api/login" {
		t.Fatalf("route = %v", req["route"])
	}
	if errVal, ok := entry.Data["error"].(string); !ok || !strings.Contains(errVal, Redacted) {
		t.Fatalf("含凭据的 error 未脱敏: %v", entry.Data["error"])
	}
	// []string 切片里的 header 原文同样要脱敏覆盖。
	rawHeaders, ok := entry.Data["raw_headers"].([]string)
	if !ok || len(rawHeaders) != 2 {
		t.Fatalf("raw_headers = %v", entry.Data["raw_headers"])
	}
	if strings.Contains(rawHeaders[1], "slice-token") || !strings.Contains(rawHeaders[1], Redacted) {
		t.Fatalf("切片内 Authorization 未脱敏: %q", rawHeaders[1])
	}
	if rawHeaders[0] != "Content-Type: text/plain" {
		t.Fatalf("非敏感 header 被改写: %q", rawHeaders[0])
	}
}

func TestNewLoggerInstallsSanitizeHookOnce(t *testing.T) {
	snapshotStandardLogger(t)
	std := logrus.StandardLogger()

	for i := 0; i < 2; i++ { // 重复构造模拟 Fx 重启/多测试初始化
		if _, err := NewLogger(fxtest.NewLifecycle(t), validConf()); err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for _, hook := range std.Hooks[logrus.ErrorLevel] {
		if _, ok := hook.(*SanitizeHook); ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("SanitizeHook 安装次数 = %d, want 1", count)
	}

	var buf bytes.Buffer
	std.SetOutput(&buf)
	std.SetFormatter(&ModuleFormatter{})
	logrus.WithFields(logrus.Fields{
		"module":        "AUTH",
		"authorization": "Bearer leaked-by-field",
		"route":         "/api/user",
	}).Error(`login failed SecretKey=sk_live_leaked_by_message`)

	out := buf.String()
	for _, leak := range []string{"leaked-by-field", "sk_live_leaked_by_message"} {
		if strings.Contains(out, leak) {
			t.Fatalf("全局 Logger 输出仍含凭据 %q: %s", leak, out)
		}
	}
	if !strings.Contains(out, Redacted) {
		t.Fatalf("全局 Logger 输出缺少脱敏占位符: %s", out)
	}
	if !strings.Contains(out, "route=/api/user") {
		t.Fatalf("白名单字段丢失: %s", out)
	}
}
