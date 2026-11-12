package wallet

import (
	"testing"
)

// 兑换码明文权限必须独立于管理员路由权限：拥有 /admin/redemption-codes
// 的调用方仍然可能没有 wallet.redemption-code 的读取能力。
const (
	testAdminSubject      = "9001"
	testCodeReaderRole    = "wallet-code-reader"
	testCodePlaintextObj  = "wallet.redemption-code"
	testCodePlaintextVerb = "read"
)

func TestCasbinCodeGuardFailsClosedWithoutEnforcer(t *testing.T) {
	guard := newCasbinCodeGuard(nil)
	allowed, err := guard.Allowed(testAdminSubject)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("guard without enforcer must deny plaintext access")
	}

	enforcer := newTestEnforcer(t)
	guard = newCasbinCodeGuard(enforcer)
	if allowed, err := guard.Allowed(""); err != nil || allowed {
		t.Fatalf("empty operator must be denied, got allowed=%v err=%v", allowed, err)
	}
}

func TestCasbinCodeGuardRequiresDedicatedPolicy(t *testing.T) {
	enforcer := newTestEnforcer(t)
	guard := newCasbinCodeGuard(enforcer)

	if allowed, err := guard.Allowed(testAdminSubject); err != nil || allowed {
		t.Fatalf("no policy yet, got allowed=%v err=%v", allowed, err)
	}

	// 只有路由权限时，明文读取依旧被拒绝。
	if _, err := enforcer.AddPolicy("admin", "/admin/redemption-codes/list", "POST"); err != nil {
		t.Fatalf("add admin policy: %v", err)
	}
	if _, err := enforcer.AddRoleForUser(testAdminSubject, "admin"); err != nil {
		t.Fatalf("bind admin role: %v", err)
	}
	if allowed, err := guard.Allowed(testAdminSubject); err != nil || allowed {
		t.Fatalf("route policy must not grant plaintext, got allowed=%v err=%v", allowed, err)
	}

	// 独立策略授予后才可读明文。
	if _, err := enforcer.AddPolicy(testCodeReaderRole, testCodePlaintextObj, testCodePlaintextVerb); err != nil {
		t.Fatalf("add plaintext policy: %v", err)
	}
	if _, err := enforcer.AddRoleForUser(testAdminSubject, testCodeReaderRole); err != nil {
		t.Fatalf("bind code reader role: %v", err)
	}
	if allowed, err := guard.Allowed(testAdminSubject); err != nil || !allowed {
		t.Fatalf("dedicated policy must grant plaintext, got allowed=%v err=%v", allowed, err)
	}

	// 其他管理员不受影响。
	if allowed, err := guard.Allowed("9002"); err != nil || allowed {
		t.Fatalf("unrelated administrator must stay denied, got allowed=%v err=%v", allowed, err)
	}
}

func TestCasbinCodeGuardDoesNotLeakThroughWildcard(t *testing.T) {
	enforcer := newTestEnforcer(t)
	guard := newCasbinCodeGuard(enforcer)

	// keyMatch2 通配符只匹配路由对象，不会匹配兑换码明文对象。
	if _, err := enforcer.AddPolicy("admin", "/admin/*", "GET"); err != nil {
		t.Fatalf("add wildcard policy: %v", err)
	}
	if _, err := enforcer.AddRoleForUser(testAdminSubject, "admin"); err != nil {
		t.Fatalf("bind admin role: %v", err)
	}
	if allowed, err := guard.Allowed(testAdminSubject); err != nil || allowed {
		t.Fatalf("wildcard route policy must not grant plaintext, got allowed=%v err=%v", allowed, err)
	}
}
