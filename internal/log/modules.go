package log

import "github.com/sirupsen/logrus"

// 模块名常量：日志前缀只能来自这些代码常量，不接受外部输入，防止伪造日志前缀。
const (
	ModuleApp   = "APP"
	ModuleDB    = "DB"
	ModuleRedis = "REDIS"
	ModuleHTTP  = "HTTP"
	ModuleRPC   = "RPC"
	ModuleAuth  = "AUTH"
	ModuleFx    = "FX"
)

// WithModule 为日志条目打模块标签；未打标签的日志按 [APP] 输出。
// 与 logrus.WithContext / WithError 链式调用兼容。
func WithModule(module string) *logrus.Entry {
	return logrus.WithField("module", module)
}
