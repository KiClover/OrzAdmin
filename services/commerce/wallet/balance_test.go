package wallet

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	settings "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/cloudwego/kitex/pkg/kerrors"
)

// 这组测试冻结任务 4 的入口行为：统一 guard、参数校验顺序与业务错误码。
// 需要真实数据库的余额/并发行为放在 balance_integration_test.go。

const testOwnerNamespace = "commerce"

// newPolicyRuntime 构造注入了固定时钟与可控数据存在性的运行时服务；
// snap 为 nil 表示从未成功加载过快照。
func newPolicyRuntime(t *testing.T, snap *settings.WalletRuntimeSettings, presence DataPresence) *Service {
	t.Helper()

	source := &fakeSettingsSource{snap: snap}
	runtime, err := NewService(Options{Source: source, Presence: presence})
	if err != nil {
		t.Fatalf("构造运行时服务失败: %v", err)
	}
	runtime.now = func() time.Time { return testNow }

	if snap != nil {
		if err := runtime.Refresh(context.Background()); err != nil {
			t.Fatalf("刷新运行时快照失败: %v", err)
		}
	}
	return runtime
}

// bizCodeOf 断言错误是 Kitex 业务错误并返回错误码。
func bizCodeOf(t *testing.T, err error) int32 {
	t.Helper()

	if err == nil {
		t.Fatal("期望业务错误，实际为 nil")
	}
	var biz kerrors.BizStatusErrorIface
	if !errors.As(err, &biz) {
		t.Fatalf("期望 Kitex 业务错误，实际为 %T: %v", err, err)
	}
	return biz.BizStatusCode()
}

func testHandler(runtime *Service) CommerceWalletService {
	return CommerceWalletService{
		Cfg:     &conf.Config{Wallet: conf.WalletConf{OwnerNamespace: testOwnerNamespace}},
		Runtime: runtime,
	}
}

func TestGuardAllowsReadOnEmptyDatabaseWithoutSnapshot(t *testing.T) {
	handler := testHandler(newPolicyRuntime(t, nil, fakePresence{}))

	for _, action := range []Action{ActionQuery, ActionListTransactions, ActionAdminGetWallet, ActionAdminListWallets, ActionAdminListTransactions} {
		if err := handler.guard(context.Background(), action); err != nil {
			t.Fatalf("空库无快照时只读动作 %s 应允许: %v", action, err)
		}
	}
}

func TestGuardRejectsNewValueWithoutSnapshot(t *testing.T) {
	handler := testHandler(newPolicyRuntime(t, nil, fakePresence{}))

	for _, action := range []Action{ActionDebitBalance, ActionCreateRecharge, ActionRedeemCode} {
		err := handler.guard(context.Background(), action)
		if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED) {
			t.Fatalf("%s 应返回 FEATURE_DISABLED，实际 code=%d", action, code)
		}
	}
}

func TestGuardRejectsNewValueWhenWalletDisabled(t *testing.T) {
	snapshot := validSnapshot(3)
	snapshot.Enabled = false
	handler := testHandler(newPolicyRuntime(t, snapshot, nil))

	if err := handler.guard(context.Background(), ActionQuery); err != nil {
		t.Fatalf("关闭开关不应阻塞查询: %v", err)
	}
	err := handler.guard(context.Background(), ActionDebitBalance)
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED) {
		t.Fatalf("关闭时应拒绝消费，实际 code=%d", code)
	}
}

func TestGuardRejectsNewValueWhenSnapshotStale(t *testing.T) {
	runtime := newPolicyRuntime(t, validSnapshot(4), nil)
	runtime.now = func() time.Time { return testNow.Add(walletpolicy.SettingStaleThreshold + time.Minute) }
	handler := testHandler(runtime)

	if err := handler.guard(context.Background(), ActionListTransactions); err != nil {
		t.Fatalf("stale 不应阻塞查询: %v", err)
	}
	if code := bizCodeOf(t, handler.guard(context.Background(), ActionDebitBalance)); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED) {
		t.Fatalf("stale 时应拒绝消费，实际 code=%d", code)
	}
	// 同 owner 补偿属于恢复动作，stale 时仍然允许。
	if err := handler.guard(context.Background(), ActionRefundBalance); err != nil {
		t.Fatalf("stale 时余额补偿应允许: %v", err)
	}
}

func TestGuardFailsClosedWhenPresenceUnknown(t *testing.T) {
	broken := newPolicyRuntime(t, nil, fakePresence{err: errors.New("db down")})
	err := testHandler(broken).guard(context.Background(), ActionQuery)
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_UNAVAILABLE) {
		t.Fatalf("数据存在性未知时应 UNAVAILABLE，实际 code=%d", code)
	}

	present := newPolicyRuntime(t, nil, fakePresence{has: true})
	err = testHandler(present).guard(context.Background(), ActionQuery)
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_UNAVAILABLE) {
		t.Fatalf("有数据但无快照时应 UNAVAILABLE，实际 code=%d", code)
	}
}

func TestGuardRequiresRuntime(t *testing.T) {
	err := (CommerceWalletService{}).guard(context.Background(), ActionQuery)
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_UNAVAILABLE) {
		t.Fatalf("未装配运行时服务时应 UNAVAILABLE，实际 code=%d", code)
	}
}

func TestDebitBalanceValidatesInputBeforeDatabase(t *testing.T) {
	handler := testHandler(newPolicyRuntime(t, validSnapshot(5), nil))

	_, err := handler.DebitBalance(context.Background(), &wallet.DebitBalanceReq{
		UserId: "not-a-number", Currency: currencyCNY, Amount: 100,
		BusinessType: "order", BusinessId: "1",
	})
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
		t.Fatalf("非法 user_id 应 INVALID_ARGUMENT，实际 code=%d", code)
	}

	_, err = handler.DebitBalance(context.Background(), &wallet.DebitBalanceReq{
		UserId: "7001", Currency: currencyUSD, Amount: 100,
		BusinessType: "order", BusinessId: "1",
	})
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
		t.Fatalf("USD 钱包消费应 INVALID_ARGUMENT，实际 code=%d", code)
	}

	_, err = handler.DebitBalance(context.Background(), &wallet.DebitBalanceReq{
		UserId: "7001", Currency: currencyCNY, Amount: 0,
		BusinessType: "order", BusinessId: "1",
	})
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
		t.Fatalf("零金额应 INVALID_ARGUMENT，实际 code=%d", code)
	}
}

func TestRefundBalanceValidatesRelatedID(t *testing.T) {
	handler := testHandler(newPolicyRuntime(t, validSnapshot(6), nil))

	_, err := handler.RefundBalance(context.Background(), &wallet.RefundBalanceReq{RelatedTransactionId: "abc"})
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
		t.Fatalf("非法 related_transaction_id 应 INVALID_ARGUMENT，实际 code=%d", code)
	}
}

func TestBusinessRPCsOutsideTask6StayNotImplemented(t *testing.T) {
	handler := testHandler(newPolicyRuntime(t, validSnapshot(7), nil))

	for name, call := range map[string]func() error{
		"RedeemCode": func() error {
			_, err := handler.RedeemCode(context.Background(), &wallet.RedeemCodeReq{})
			return err
		},
		"AdminCreateRedemptionCode": func() error {
			_, err := handler.AdminCreateRedemptionCode(context.Background(), &wallet.AdminCreateRedemptionCodeReq{})
			return err
		},
		"AdminListRedemptionCodes": func() error {
			_, err := handler.AdminListRedemptionCodes(context.Background(), &wallet.AdminListRedemptionCodesReq{})
			return err
		},
		"AdminSetRedemptionCodeStatus": func() error {
			_, err := handler.AdminSetRedemptionCodeStatus(context.Background(), &wallet.AdminSetRedemptionCodeStatusReq{})
			return err
		},
	} {
		if code := bizCodeOf(t, call()); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_NOT_IMPLEMENTED) {
			t.Fatalf("%s 应返回 NOT_IMPLEMENTED，实际 code=%d", name, code)
		}
	}
}
