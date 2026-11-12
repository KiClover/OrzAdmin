package log

import "github.com/sirupsen/logrus"

// 模块名常量：日志前缀只能来自这些代码常量，不接受外部输入，防止伪造日志前缀。
const (
	ModuleApp        = "APP"
	ModuleDB         = "DB"
	ModuleRedis      = "REDIS"
	ModuleHTTP       = "HTTP"
	ModuleRPC        = "RPC"
	ModuleAuth       = "AUTH"
	ModuleFx         = "FX"
	ModuleTrace      = "TRACE"
	ModuleAccess     = "ACCESS"     // services/system/access
	ModuleMessage    = "MESSAGE"    // services/system/message
	ModuleSetting    = "SETTING"    // services/system/setting
	ModuleUser       = "USER"       // services/system/user
	ModuleToken      = "TOKEN"      // services/system/token
	ModuleWallet     = "WALLET"     // services/commerce/wallet
	ModuleDiscovery  = "DISCOVERY"  // internal/discovery 服务注册发现
	ModuleRunner     = "RUNNER"     // internal/constructor 服务运行器
	ModuleMiddleware = "MIDDLEWARE" // internal/middleware 鉴权等中间件
)

// WithModule 为日志条目打模块标签；未打标签的日志按 [APP] 输出。
// 与 logrus.WithContext / WithError 链式调用兼容。
func WithModule(module string) *logrus.Entry {
	return logrus.WithField("module", module)
}
