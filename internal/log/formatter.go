package log

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

const defaultTimestampFormat = "2006-01-02 15:04:05"

// ModuleFormatter 输出 "[MODULE][LEVEL] 2006-01-02 15:04:05: message key=value"。
// 无状态（只读 entry），可安全并发；消息体原样输出，不改写业务文案。
type ModuleFormatter struct {
	TimestampFormat string // 缺省 "2006-01-02 15:04:05"
}

func (f *ModuleFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	tsFormat := f.TimestampFormat
	if tsFormat == "" {
		tsFormat = defaultTimestampFormat
	}

	module := ModuleApp
	if m, ok := entry.Data["module"].(string); ok && m != "" {
		module = strings.ToUpper(m)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[%s][%s] %s: %s", module, levelTag(entry.Level),
		entry.Time.Format(tsFormat), entry.Message)

	keys := make([]string, 0, len(entry.Data))
	for k := range entry.Data {
		if k == "module" {
			continue // module 已体现在前缀，不重复输出
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%s", k, quoteIfNeeded(fmt.Sprint(entry.Data[k])))
	}

	b.WriteByte('\n')
	return []byte(b.String()), nil
}

// levelTag 输出大写级别标签；logrus 的 WarnLevel.String() 为 "warning"，
// 设计规范（doc/LoggingDesign.MD LEVEL 定义）要求统一为 "WARN"。
func levelTag(l logrus.Level) string {
	if l == logrus.WarnLevel {
		return "WARN"
	}
	return strings.ToUpper(l.String())
}

func quoteIfNeeded(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\n\"") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
