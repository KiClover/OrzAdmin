package wallet

import (
	"fmt"

	"github.com/casbin/casbin/v2"
)

// 兑换码明文读取使用独立的对象与动作，不与 HTTP 路由策略共用：
// 拥有 /admin/redemption-codes 路由权限，并不自动获得查看明文码的能力。
const (
	walletCodePlaintextObject = "wallet.redemption-code"
	walletCodePlaintextAction = "read"
)

// plaintextCodeGuard 判定操作人能否读取兑换码明文。
//
// 设计 §11.4 的独立策略：
//
//	p = wallet-code-reader, wallet.redemption-code, read
//	g = <user id>, wallet-code-reader
//
// 因此管理员角色与普通管理员路由策略都不足以返回明文。
type plaintextCodeGuard interface {
	Allowed(operator string) (bool, error)
}

// casbinCodeGuard 使用进程共享的 Casbin 实例校验独立明文策略。
type casbinCodeGuard struct {
	enforcer *casbin.Enforcer
}

// newCasbinCodeGuard 包装共享 Casbin 实例；实例缺失时保持 fail closed。
func newCasbinCodeGuard(enforcer *casbin.Enforcer) plaintextCodeGuard {
	return casbinCodeGuard{enforcer: enforcer}
}

// Allowed 返回操作人是否被显式授予明文读取能力。
func (g casbinCodeGuard) Allowed(operator string) (bool, error) {
	if operator == "" || g.enforcer == nil {
		return false, nil
	}
	allowed, err := g.enforcer.Enforce(operator, walletCodePlaintextObject, walletCodePlaintextAction)
	if err != nil {
		return false, fmt.Errorf("wallet code authorization failed: %w", err)
	}
	return allowed, nil
}
