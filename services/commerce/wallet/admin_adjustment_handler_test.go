package wallet

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// 这组测试冻结管理员调账的入口行为：动作登记、统一 guard 与参数校验顺序。
// 余额/幂等的真实数据库行为放在集成测试与 Task 5 验收。

func validAdminAdjustRequest() *wallet.AdminAdjustBalanceReq {
	return &wallet.AdminAdjustBalanceReq{
		OperatorId:  "42",
		UserId:      "7001",
		RequestId:   "adj-1",
		DeltaAmount: -500,
		Reason:      "manual correction",
	}
}

func TestAdminAdjustBalanceActionIsNewValue(t *testing.T) {
	if string(ActionAdminAdjustBalance) != "admin_adjust_balance" {
		t.Fatalf("动作名必须稳定，实际 %q", string(ActionAdminAdjustBalance))
	}
	if got := actionCategories[ActionAdminAdjustBalance]; got != categoryNewValue {
		t.Fatalf("管理员调账会改动余额，必须归入 categoryNewValue，实际 %v", got)
	}
	if _, ok := actionCategories[ActionAdminAdjustBalance]; !ok {
		t.Fatal("管理员调账动作必须登记在动作策略表中")
	}
}

func TestAdminAdjustBalanceRequiresTrustedSnapshot(t *testing.T) {
	cases := []struct {
		name string
		svc  CommerceWalletService
		code wallet.WalletErrorCode
	}{
		{
			name: "wallet disabled",
			svc: func() CommerceWalletService {
				snapshot := validSnapshot(11)
				snapshot.Enabled = false
				return testHandler(newPolicyRuntime(t, snapshot, nil))
			}(),
			code: wallet.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED,
		},
		{
			name: "no snapshot",
			svc:  testHandler(newPolicyRuntime(t, nil, fakePresence{})),
			code: wallet.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED,
		},
		{
			name: "stale snapshot",
			svc: func() CommerceWalletService {
				runtime := newPolicyRuntime(t, validSnapshot(12), nil)
				runtime.now = func() time.Time { return testNow.Add(walletpolicy.SettingStaleThreshold + time.Minute) }
				return testHandler(runtime)
			}(),
			code: wallet.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED,
		},
		{
			name: "presence unknown",
			svc:  testHandler(newPolicyRuntime(t, nil, fakePresence{err: errors.New("db down")})),
			code: wallet.WalletErrorCode_WALLET_ERROR_CODE_UNAVAILABLE,
		},
		{
			name: "data exists without settings",
			svc:  testHandler(newPolicyRuntime(t, nil, fakePresence{has: true})),
			code: wallet.WalletErrorCode_WALLET_ERROR_CODE_UNAVAILABLE,
		},
		{
			name: "runtime not wired",
			svc:  CommerceWalletService{},
			code: wallet.WalletErrorCode_WALLET_ERROR_CODE_UNAVAILABLE,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.svc.AdminAdjustBalance(context.Background(), validAdminAdjustRequest())
			if code := bizCodeOf(t, err); code != int32(tc.code) {
				t.Fatalf("期望 %v，实际 code=%d", tc.code, code)
			}
		})
	}
}

// 调账不受 recharge_mode 与 redemption_enabled 收窄：它既不新建充值，也不涉及兑换码。
func TestAdminAdjustBalanceAllowedWhenRechargeAndRedemptionClosed(t *testing.T) {
	snapshot := validSnapshot(13)
	snapshot.RechargeMode = rechargeModeDisabled
	snapshot.RedemptionEnabled = false
	handler := testHandler(newPolicyRuntime(t, snapshot, nil))

	if err := handler.guard(context.Background(), ActionAdminAdjustBalance); err != nil {
		t.Fatalf("充值/兑换关闭不应阻塞调账: %v", err)
	}
}

func TestAdminAdjustBalanceValidatesInputBeforeDatabase(t *testing.T) {
	// validSnapshot 让 guard 通过；非法参数必须在触达数据库之前被拒绝，
	// 因此未注入 DB 的 handler 也不会 panic。
	handler := testHandler(newPolicyRuntime(t, validSnapshot(14), nil))

	cases := []struct {
		name   string
		mutate func(*wallet.AdminAdjustBalanceReq)
	}{
		{"missing operator", func(r *wallet.AdminAdjustBalanceReq) { r.OperatorId = "" }},
		{"invalid operator", func(r *wallet.AdminAdjustBalanceReq) { r.OperatorId = "not-a-number" }},
		{"zero operator", func(r *wallet.AdminAdjustBalanceReq) { r.OperatorId = "0" }},
		{"invalid user", func(r *wallet.AdminAdjustBalanceReq) { r.UserId = "abc" }},
		{"zero user", func(r *wallet.AdminAdjustBalanceReq) { r.UserId = "0" }},
		{"empty request id", func(r *wallet.AdminAdjustBalanceReq) { r.RequestId = "" }},
		{"blank request id", func(r *wallet.AdminAdjustBalanceReq) { r.RequestId = "   " }},
		{"too long request id", func(r *wallet.AdminAdjustBalanceReq) { r.RequestId = strings.Repeat("a", maxBusinessIDLen+1) }},
		{"empty reason", func(r *wallet.AdminAdjustBalanceReq) { r.Reason = "" }},
		{"blank reason", func(r *wallet.AdminAdjustBalanceReq) { r.Reason = "\t \n" }},
		{"too long reason", func(r *wallet.AdminAdjustBalanceReq) { r.Reason = strings.Repeat("a", maxDescriptionLen+1) }},
		{"zero delta", func(r *wallet.AdminAdjustBalanceReq) { r.DeltaAmount = 0 }},
		{"min int64 delta", func(r *wallet.AdminAdjustBalanceReq) { r.DeltaAmount = math.MinInt64 }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validAdminAdjustRequest()
			tc.mutate(req)
			_, err := handler.AdminAdjustBalance(context.Background(), req)
			if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
				t.Fatalf("期望 INVALID_ARGUMENT，实际 code=%d", code)
			}
		})
	}

	t.Run("nil request", func(t *testing.T) {
		_, err := handler.AdminAdjustBalance(context.Background(), nil)
		if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
			t.Fatalf("nil 请求应 INVALID_ARGUMENT，实际 code=%d", code)
		}
	})
}

// storageUnavailable 只把可识别的「存储暂时不可用」映射为 UNAVAILABLE；
// 其余数据库错误保持 INTERNAL，且绝不把驱动错误文本透给调用方。
func TestAdminAdjustmentStorageUnavailableClassification(t *testing.T) {
	badConn := fmt.Errorf("lock wallet: %w", driver.ErrBadConn)
	deadline := fmt.Errorf("lock wallet: %w", context.DeadlineExceeded)
	if !storageUnavailable(badConn) || !storageUnavailable(deadline) {
		t.Fatal("可识别的存储不可用必须映射为 UNAVAILABLE")
	}
	if storageUnavailable(errors.New("duplicate key value violates unique constraint")) {
		t.Fatal("唯一索引冲突不应被当成存储不可用")
	}
	if storageUnavailable(nil) {
		t.Fatal("nil 不是存储不可用")
	}
}
