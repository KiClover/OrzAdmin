package log

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/sirupsen/logrus"
)

// Redacted 是敏感值被替换后的固定占位符。替换不可逆，保留"此处有值"的语义便于排障。
const Redacted = "[REDACTED]"

// sensitiveKeySuffixes 是结构化字段名的敏感后缀表。
//
// 比较前键名统一转小写并去掉分隔符（- _ . 等），因此 "SecretKey"、"secret_key"、
// "X-Secret-Key" 归一后都是 "xsecretkey"，命中后缀 "secretkey"。
//
// 用后缀而非子串匹配，既覆盖 user_password / access_token / client_secret 这类组合键，
// 又不会把 token_count、password_policy 这类统计/策略字段误判成凭据。
var sensitiveKeySuffixes = []string{
	"authorization",
	"cookie",
	"sessionid",
	"session",
	"secretkey",
	"secret",
	"token",
	"password",
	"passwd",
	"pwd",
	"passphrase",
	"captcha",
	"verifycode",
	"smscode",
	"mailcode",
	"authcode",
	"credential",
	"credentials",
	"accesskey",
	"apikey",
	"privatekey",
	"signature",
	"bearer",
}

// sensitiveExactKeys 是需要精确命中的敏感键。
//
// "code" 只在精确命中时脱敏：本项目 RegisterByEmail/RegisterByMobile/LoginByCodeReq/
// FindPwdReq 用 `code` 承载邮箱/短信验证码（凭据），而错误码统一写作 error_code /
// biz_code 等带前缀的键，不受此规则影响。
var sensitiveExactKeys = map[string]struct{}{
	"code": {},
}

// 文本脱敏规则：日志消息与字符串字段值都要能兜住"把凭据拼进文案"的写法。
var (
	// "Bearer <token>" / "Basic <base64>"：保留 scheme，凭据整体替换。
	// 先于/后于 key=value 处理的顺序很关键：见 sanitizeText。
	bearerCredentialRe = regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9._~+/=\-]{8,}`)
	basicCredentialRe  = regexp.MustCompile(`(?i)\b(basic)\s+[A-Za-z0-9+/=._~\-]{8,}`)
	// JWT（三段 base64url）即使没有字段名也一律脱敏。
	jwtCredentialRe = regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{6,}\.[A-Za-z0-9_\-]{6,}(?:\.[A-Za-z0-9_\-]*)?`)
	// key=value / key: value / "key":"value"：键名命中敏感表时替换值。
	// 键名后可选的 ["']? 让 JSON（"password":"..."）和 header（SecretKey: ...）走同一条规则。
	keyValueRe = regexp.MustCompile(`\b([A-Za-z0-9_\-]{2,64})["']?\s*[:=]\s*("[^"\n]*"|'[^'\n]*'|[^\s,;&)\]}"']+)`)
)

// needsSanitizing 是正则前的廉价预筛：绝大多数日志不含这些片段，直接原样返回。
var sanitizeMarkers = []string{
	"authoriz", "cookie", "secret", "token", "passw", "pwd", "captcha",
	"verify", "credential", "apikey", "api_key", "access_key", "accesskey",
	"private_key", "privatekey", "signature", "session", "bearer", "eyj",
	"code=", "code:", "code\":",
}

// SanitizeHook 是统一脱敏 Hook：写出前把日志条目里的凭据替换成 Redacted。
//
// 覆盖两类泄露面：
//  1. 结构化字段：键名命中敏感表时整值替换；map 值递归处理；
//  2. 文本：消息与字符串字段值里的 Authorization / Cookie / SecretKey / Token /
//     密码 / 验证码 / 服务密钥等 key=value、Bearer、JWT 片段。
//
// Hook 无状态、并发安全（logrus 写路径本身串行，且只改本次 Entry 的副本）。
type SanitizeHook struct{}

// NewSanitizeHook 创建脱敏 Hook，供 NewLogger 安装到全局 Logger。
func NewSanitizeHook() *SanitizeHook { return &SanitizeHook{} }

// Levels 对所有级别生效：凭据泄露与级别无关。
func (h *SanitizeHook) Levels() []logrus.Level { return logrus.AllLevels }

// Fire 在 Entry 落到 Formatter/Writer 之前就地脱敏。
func (h *SanitizeHook) Fire(entry *logrus.Entry) error {
	if entry == nil {
		return nil
	}
	entry.Message = sanitizeText(entry.Message)
	for k, v := range entry.Data {
		if isSensitiveKey(k) {
			entry.Data[k] = Redacted
			continue
		}
		entry.Data[k] = sanitizeValue(v)
	}
	return nil
}

// installSanitizeHook 幂等安装：同一 Logger 重复构造不会叠加多个 Hook。
func installSanitizeHook(logger *logrus.Logger) {
	if logger == nil {
		return
	}
	for _, hooks := range logger.Hooks {
		for _, hook := range hooks {
			if _, ok := hook.(*SanitizeHook); ok {
				return
			}
		}
	}
	logger.AddHook(NewSanitizeHook())
}

// isSensitiveKey 判断字段名是否为凭据字段（大小写与分隔符不敏感）。
func isSensitiveKey(key string) bool {
	norm := normalizeKey(key)
	if norm == "" {
		return false
	}
	if _, ok := sensitiveExactKeys[norm]; ok {
		return true
	}
	for _, suffix := range sensitiveKeySuffixes {
		if strings.HasSuffix(norm, suffix) {
			return true
		}
	}
	return false
}

// normalizeKey 只保留字母与数字并转小写："X-Secret_Key" -> "xsecretkey"。
func normalizeKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	for _, r := range key {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// sanitizeValue 清洗非敏感键的值：字符串做文本脱敏，map/slice 递归，error 仅在有泄露迹象时转文本。
func sanitizeValue(v any) any {
	switch val := v.(type) {
	case string:
		return sanitizeText(val)
	case error:
		// 正常错误原文保持 error 类型（Formatter 行为不变）；只有确实带凭据时才降级成已脱敏文本。
		if !needsSanitizing(val.Error()) {
			return v
		}
		return sanitizeText(val.Error())
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			if isSensitiveKey(k) {
				out[k] = Redacted
				continue
			}
			out[k] = sanitizeValue(item)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(val))
		for k, item := range val {
			if isSensitiveKey(k) {
				out[k] = Redacted
				continue
			}
			out[k] = sanitizeText(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(val))
		for _, item := range val {
			out = append(out, sanitizeValue(item))
		}
		return out
	case []string:
		out := make([]string, 0, len(val))
		for _, item := range val {
			out = append(out, sanitizeText(item))
		}
		return out
	default:
		return v
	}
}

// sanitizeText 对一段文本做凭据脱敏；无泄露迹象时原样返回，不改写普通日志文案。
//
// 顺序：先按 key=value 规则取值（能保留键名便于检索），再处理 Bearer/Basic/JWT。
// key=value 遇到 "Bearer" 这类纯 scheme 值时跳过，避免把 scheme 当凭据替换后
// 让真正的凭据裸露在下一个词（Authorization: Bearer xxx）。
func sanitizeText(s string) string {
	if s == "" || !needsSanitizing(s) {
		return s
	}
	s = redactSensitiveKeyValues(s)
	s = bearerCredentialRe.ReplaceAllString(s, "$1 "+Redacted)
	s = basicCredentialRe.ReplaceAllString(s, "$1 "+Redacted)
	s = jwtCredentialRe.ReplaceAllString(s, Redacted)
	return s
}

// needsSanitizing 是廉价的预筛，避免给每条日志都跑正则。
func needsSanitizing(s string) bool {
	lower := strings.ToLower(s)
	for _, marker := range sanitizeMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// redactSensitiveKeyValues 把 "key=value" 形式中敏感键对应的值替换为 Redacted。
// 保留原键名、分隔符与引号形式，便于按 key 继续检索。
func redactSensitiveKeyValues(s string) string {
	matches := keyValueRe.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, m := range matches {
		// keyValueRe 有两个捕获组：m[2:4] 是键名，m[4:6] 是值。
		key := s[m[2]:m[3]]
		if m[4] < 0 || m[5] < 0 {
			continue
		}
		value := s[m[4]:m[5]]
		if !isSensitiveKey(key) || isSchemeOnly(value) || isRedactedValue(value) {
			continue
		}
		b.WriteString(s[last:m[4]])
		quote := ""
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
			quote = string(value[0])
		}
		b.WriteString(quote + Redacted + quote)
		last = m[5]
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// isSchemeOnly 判断取到的值是否只是认证 scheme（真正的凭据在下一个词）。
func isSchemeOnly(value string) bool {
	switch strings.ToLower(strings.Trim(value, `"'`)) {
	case "bearer", "basic", "digest", "token":
		return true
	default:
		return false
	}
}

// isRedactedValue 判断值是否已被上一次脱敏处理过，保证 sanitizeText 幂等。
func isRedactedValue(value string) bool {
	inner := strings.Trim(value, `"'`)
	return inner == Redacted || strings.HasPrefix(inner, Redacted)
}
