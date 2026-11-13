package conf

import "fmt"

// WalletConf 只承载随部署变化的钱包参数。
//
// 技术性约束（超时、响应体大小、刷新周期、分页上限等）属于代码常量，
// 见 internal/walletpolicy 包，不在此处暴露。
type WalletConf struct {
	// OwnerNamespace 是本钱包实例代表调用方派生的固定业务命名空间。
	// 设计 §12 要求它来自服务端启动配置（专属 listener 身份），
	// 请求体只能携带该命名空间内的业务键，不能覆盖它。
	OwnerNamespace string `mapstructure:"owner_namespace"`
}

// Validate 校验部署配置的格式。
func (c WalletConf) Validate() error {
	if !validOwnerNamespace(c.OwnerNamespace) {
		return fmt.Errorf("wallet.owner_namespace must match [a-z0-9][a-z0-9._-]{0,63}: %q", c.OwnerNamespace)
	}
	return nil
}

// validOwnerNamespace 限定命名空间为小写字母/数字开头、最多 64 字符的安全 token，
// 避免把空白或控制字符写进不可变业务键。
func validOwnerNamespace(ns string) bool {
	if len(ns) == 0 || len(ns) > 64 {
		return false
	}
	for i, r := range ns {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
