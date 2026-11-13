//go:build integration

package wallet

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/wallettransaction"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting"
)

// 任务 5 的资金验收：管理员调账必须走真实事务、并发与历史重放。
// 需要真实 MySQL：WALLET_TEST_MYSQL_DSN，例如
// user:pass@tcp(127.0.0.1:3306)/orz_wallet_test?parseTime=True
//
// 运行：
//   go test -tags=integration ./services/commerce/wallet \
//     -run '^TestWalletAdminAdjustment' -count=1 -v

const integrationOperatorID = "42"

// errInjectedTransactionCreate 是回滚验收注入的固定错误。
var errInjectedTransactionCreate = errors.New("injected wallet transaction create failure")

// adminAdjustRequest 构造管理员调账请求。
func adminAdjustRequest(userID, requestID string, delta int64, reason, operator string) *wallet.AdminAdjustBalanceReq {
	return &wallet.AdminAdjustBalanceReq{
		OperatorId:  operator,
		UserId:      userID,
		RequestId:   requestID,
		DeltaAmount: delta,
		Reason:      reason,
	}
}

// seedWallet 预建一个零余额钱包，不产生流水。
func seedWallet(t *testing.T, client *ent.Client, userID uint64) *ent.WalletAccount {
	t.Helper()

	account, err := client.WalletAccount.Create().
		SetUserID(userID).
		SetCurrency(currencyCNY).
		Save(context.Background())
	if err != nil {
		t.Fatalf("预建钱包失败: %v", err)
	}
	return account
}

// walletRevisionOf 读取钱包 revision，用于断言重放不改版本。
func walletRevisionOf(t *testing.T, client *ent.Client, userID uint64) int64 {
	t.Helper()

	account := walletAccountOf(t, client, userID)
	if account == nil {
		t.Fatalf("用户 %d 没有钱包", userID)
	}
	return int64(account.Revision)
}

// adminAdjustmentTransactions 返回该钱包的管理员调账流水（按创建顺序）。
func adminAdjustmentTransactions(t *testing.T, client *ent.Client, walletID uint64) []*ent.WalletTransaction {
	t.Helper()

	rows, err := client.WalletTransaction.Query().
		Where(
			wallettransaction.WalletID(walletID),
			wallettransaction.Type(txTypeAdminAdjustment),
		).
		Order(ent.Asc(wallettransaction.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		t.Fatalf("查询调账流水失败: %v", err)
	}
	return rows
}

// seedDebitBlock 写入一条 review_blocks_debit=true 的充值记录。
func seedDebitBlock(t *testing.T, client *ent.Client, walletID uint64, suffix string) {
	t.Helper()

	ctx := context.Background()
	if _, err := client.WalletRecharge.Create().
		SetWalletID(walletID).
		SetRequestID("req-block-" + suffix).
		SetMerchantOrderNo("merchant-block-" + suffix).
		SetProviderEnvironment(integrationProviderEnv).
		SetPaymentProfileID("profile-block-" + suffix).
		SetPaymentProfileRevision(1).
		SetProviderProductID("product-block-" + suffix).
		SetWalletAmount(1000).
		SetProviderAmount(139).
		SetFxRateNumerator(36).
		SetFxRateDenominator(5).
		SetFxSettingRevision(3).
		SetFxQuotedAt(testNow).
		SetStatus("REVIEW").
		SetReviewBlocksDebit(true).
		Save(ctx); err != nil {
		t.Fatalf("写入阻断记录失败: %v", err)
	}
}

// walletRuntimeWith 构造可在用例中改写快照的运行时服务。
func walletRuntimeWith(t *testing.T, mutate func(*setting.WalletRuntimeSettings)) *Service {
	t.Helper()

	snap := validSnapshot(12)
	if mutate != nil {
		mutate(snap)
	}
	runtime, err := NewService(Options{
		Source:   &fakeSettingsSource{snap: snap},
		Presence: fakePresence{has: true},
	})
	if err != nil {
		t.Fatalf("构造运行时服务失败: %v", err)
	}
	runtime.now = func() time.Time { return testNow }
	if err := runtime.Refresh(context.Background()); err != nil {
		t.Fatalf("加载运行时快照失败: %v", err)
	}
	return runtime
}

// TestWalletAdminAdjustmentMySQL 冻结资金结果与历史重放：
// 负数调账可把余额扣成负值，重放返回原始不可变流水而不是当前余额。
func TestWalletAdminAdjustmentMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	first, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
		"7001", "adjust-1", -500, "correction", integrationOperatorID))
	if err != nil {
		t.Fatalf("负调账失败: %v", err)
	}
	if first.GetTransaction().GetTransaction().GetBalanceAfter() != -500 ||
		first.GetTransaction().GetTransaction().GetType() != wallet.TransactionType_TRANSACTION_TYPE_ADMIN_ADJUSTMENT {
		t.Fatalf("调账结果错误: %+v", first.GetTransaction())
	}
	if first.GetTransaction().GetTransaction().GetDeltaAmount() != -500 {
		t.Fatalf("delta_amount 应保留符号，实际 %d", first.GetTransaction().GetTransaction().GetDeltaAmount())
	}

	account := walletAccountOf(t, client, 7001)
	if account == nil {
		t.Fatal("首次调账必须建钱包")
	}
	rows := adminAdjustmentTransactions(t, client, account.ID)
	if len(rows) != 1 {
		t.Fatalf("调账流水应为 1 条，实际 %d", len(rows))
	}
	if rows[0].Description != "correction" || rows[0].Actor != "admin:"+integrationOperatorID {
		t.Fatalf("落库审计字段错误: description=%q actor=%q", rows[0].Description, rows[0].Actor)
	}
	if rows[0].BusinessType != businessTypeAdminAdjustment || rows[0].BusinessID != "adjust-1" {
		t.Fatalf("业务键错误: %+v", rows[0])
	}
	if rows[0].RelatedTransactionID != nil {
		t.Fatalf("调账不得关联其它交易，实际 %d", *rows[0].RelatedTransactionID)
	}
	if rows[0].OwnerNamespace != testOwnerNamespace {
		t.Fatalf("owner_namespace 必须由服务端派生，实际 %q", rows[0].OwnerNamespace)
	}

	if _, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
		"7001", "adjust-2", 800, "restore", integrationOperatorID)); err != nil {
		t.Fatalf("加款失败: %v", err)
	}
	if got := walletBalanceOf(t, client, 7001); got != 300 {
		t.Fatalf("余额应为 -500+800=300，实际 %d", got)
	}

	replay, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
		"7001", "adjust-1", -500, "correction", integrationOperatorID))
	if err != nil {
		t.Fatalf("重放失败: %v", err)
	}
	if replay.GetTransaction().GetTransaction().GetId() != first.GetTransaction().GetTransaction().GetId() {
		t.Fatalf("重放必须返回原交易 %s，实际 %s",
			first.GetTransaction().GetTransaction().GetId(), replay.GetTransaction().GetTransaction().GetId())
	}
	if replay.GetTransaction().GetTransaction().GetBalanceAfter() != -500 {
		t.Fatalf("重放应返回原始 balance_after=-500，实际 %d", replay.GetTransaction().GetTransaction().GetBalanceAfter())
	}
	if got := walletBalanceOf(t, client, 7001); got != 300 {
		t.Fatalf("重放不得改变余额，实际 %d", got)
	}
}

// TestWalletAdminAdjustmentReplayMySQL 冻结幂等判定：
// 完全一致（含仅空白差异）才重放，任一业务字段变化都返回 IDEMPOTENCY_CONFLICT。
func TestWalletAdminAdjustmentReplayMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	account := seedWallet(t, client, integrationUserID)
	req := adminAdjustRequest(formatDecimalID(integrationUserID), "idem-1", 100, "manual", integrationOperatorID)

	first, err := svc.AdminAdjustBalance(ctx, req)
	if err != nil {
		t.Fatalf("首次调账失败: %v", err)
	}
	revision := walletRevisionOf(t, client, integrationUserID)
	balance := walletBalanceOf(t, client, integrationUserID)
	count := transactionCountOf(t, client, account.ID)

	t.Run("whitespace only replay succeeds", func(t *testing.T) {
		replay, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
			" "+formatDecimalID(integrationUserID)+" ", " idem-1 ", 100, " manual ", integrationOperatorID))
		if err != nil {
			t.Fatalf("仅空白差异应重放成功: %v", err)
		}
		if replay.GetTransaction().GetTransaction().GetId() != first.GetTransaction().GetTransaction().GetId() {
			t.Fatalf("重放应返回原交易")
		}
	})

	conflicts := []struct {
		name string
		req  *wallet.AdminAdjustBalanceReq
	}{
		{"other user", adminAdjustRequest(formatDecimalID(integrationSecondUser), "idem-1", 100, "manual", integrationOperatorID)},
		{"other sign", adminAdjustRequest(formatDecimalID(integrationUserID), "idem-1", -100, "manual", integrationOperatorID)},
		{"other amount", adminAdjustRequest(formatDecimalID(integrationUserID), "idem-1", 200, "manual", integrationOperatorID)},
		{"other reason", adminAdjustRequest(formatDecimalID(integrationUserID), "idem-1", 100, "different", integrationOperatorID)},
		{"other operator", adminAdjustRequest(formatDecimalID(integrationUserID), "idem-1", 100, "manual", "43")},
	}
	for _, tc := range conflicts {
		t.Run(tc.name+" conflicts", func(t *testing.T) {
			code := bizCodeOfIntegration(t, adminAdjustErrorOf(svc, ctx, tc.req))
			if code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT) {
				t.Fatalf("应以 IDEMPOTENCY_CONFLICT 拒绝，实际 code=%d", code)
			}
			if got := walletBalanceOf(t, client, integrationUserID); got != balance {
				t.Fatalf("冲突不得改余额，期望 %d，实际 %d", balance, got)
			}
			if got := walletRevisionOf(t, client, integrationUserID); got != revision {
				t.Fatalf("冲突不得改 revision，期望 %d，实际 %d", revision, got)
			}
			if got := transactionCountOf(t, client, account.ID); got != count {
				t.Fatalf("冲突不得新增流水，期望 %d，实际 %d", count, got)
			}
		})
	}
}

// adminAdjustErrorOf 只返回错误本身，便于断言业务码。
func adminAdjustErrorOf(svc *CommerceWalletService, ctx context.Context, req *wallet.AdminAdjustBalanceReq) error {
	_, err := svc.AdminAdjustBalance(ctx, req)
	return err
}

// TestWalletAdminAdjustmentConcurrencyMySQL 冻结并发语义：
// 同一 request_id 只记一次账，不同 request_id 逐笔串行累加。
func TestWalletAdminAdjustmentConcurrencyMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	t.Run("same request id records once", func(t *testing.T) {
		account := seedWallet(t, client, integrationUserID)
		startRevision := walletRevisionOf(t, client, integrationUserID)

		const attempts = 20
		var (
			wait sync.WaitGroup
			mu   sync.Mutex
			ids  = make(map[string]struct{})
		)
		for i := 0; i < attempts; i++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				resp, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
					formatDecimalID(integrationUserID), "concurrent-same", 100, "burst", integrationOperatorID))
				if err != nil {
					t.Errorf("并发重放必须幂等成功: %v", err)
					return
				}
				mu.Lock()
				defer mu.Unlock()
				ids[resp.GetTransaction().GetTransaction().GetId()] = struct{}{}
			}()
		}
		wait.Wait()

		if len(ids) != 1 {
			t.Fatalf("同一 request_id 只应产生一条交易，实际 %d 条", len(ids))
		}
		if got := walletBalanceOf(t, client, integrationUserID); got != 100 {
			t.Fatalf("只应加款一次，余额应为 100，实际 %d", got)
		}
		if got := walletRevisionOf(t, client, integrationUserID); got != startRevision+1 {
			t.Fatalf("revision 只应 +1，实际 %d", got-startRevision)
		}
		if got := transactionCountOf(t, client, account.ID); got != 1 {
			t.Fatalf("流水数应为 1，实际 %d", got)
		}
	})

	t.Run("distinct request ids accumulate", func(t *testing.T) {
		secondUser := integrationSecondUser
		account := seedWallet(t, client, secondUser)

		const attempts = 20
		var wait sync.WaitGroup
		for i := 0; i < attempts; i++ {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				requestID := "concurrent-" + formatDecimalID(uint64(index))
				if _, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
					formatDecimalID(secondUser), requestID, 10, "burst", integrationOperatorID)); err != nil {
					t.Errorf("不同 request_id 应各自成功: %v", err)
				}
			}(i)
		}
		wait.Wait()

		if got := walletBalanceOf(t, client, secondUser); got != attempts*10 {
			t.Fatalf("余额应为 %d，实际 %d", attempts*10, got)
		}
		if got := transactionCountOf(t, client, account.ID); got != attempts {
			t.Fatalf("流水数应为 %d，实际 %d", attempts, got)
		}
		if got := walletRevisionOf(t, client, secondUser); got != attempts {
			t.Fatalf("revision 应为 %d，实际 %d", attempts, got)
		}
	})
}

// TestWalletAdminAdjustmentFirstWalletMySQL 冻结首次建行与跨用户抢同一业务键：
// 只有一条流水落库，失败一方事务创建的钱包必须回滚。
func TestWalletAdminAdjustmentFirstWalletMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	t.Run("first negative adjustment creates wallet", func(t *testing.T) {
		if walletAccountOf(t, client, integrationUserID) != nil {
			t.Fatal("夹具应为无钱包用户")
		}
		resp, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
			formatDecimalID(integrationUserID), "first-adjust", -500, "debt", integrationOperatorID))
		if err != nil {
			t.Fatalf("首次负调账失败: %v", err)
		}
		if resp.GetTransaction().GetTransaction().GetBalanceAfter() != -500 {
			t.Fatalf("应记录 -500 欠款，实际 %d", resp.GetTransaction().GetTransaction().GetBalanceAfter())
		}
		if got := walletBalanceOf(t, client, integrationUserID); got != -500 {
			t.Fatalf("余额应为 -500，实际 %d", got)
		}
	})

	t.Run("two users race for one business key", func(t *testing.T) {
		users := []uint64{integrationSecondUser, integrationThirdUser}
		for _, user := range users {
			if walletAccountOf(t, client, user) != nil {
				t.Fatalf("夹具要求用户 %d 无钱包", user)
			}
		}

		var (
			wait      sync.WaitGroup
			mu        sync.Mutex
			succeeded int
			conflicts int
		)
		for _, user := range users {
			wait.Add(1)
			go func(u uint64) {
				defer wait.Done()
				_, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
					formatDecimalID(u), "race-key", 100, "race", integrationOperatorID))
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					succeeded++
				case bizCodeOfIntegration(t, err) == int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT):
					conflicts++
				default:
					t.Errorf("意外的抢键错误: %v", err)
				}
			}(user)
		}
		wait.Wait()

		if succeeded != 1 || conflicts != 1 {
			t.Fatalf("期望 1 成功 / 1 冲突，实际 %d / %d", succeeded, conflicts)
		}

		created := 0
		for _, user := range users {
			if account := walletAccountOf(t, client, user); account != nil {
				created++
				if count := transactionCountOf(t, client, account.ID); count != 1 {
					t.Fatalf("成功方应只有 1 条流水，实际 %d", count)
				}
			}
		}
		if created != 1 {
			t.Fatalf("失败方事务创建的的钱包必须回滚，实际钱包数 %d", created)
		}
	})
}

// TestWalletAdminAdjustmentOverflowMySQL 冻结溢出语义：
// 超界属于可预期的 STATE_CONFLICT，且不得留下半成品。
func TestWalletAdminAdjustmentOverflowMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	t.Run("upper bound", func(t *testing.T) {
		account := seedWallet(t, client, integrationUserID)
		if _, err := client.WalletAccount.UpdateOneID(account.ID).
			SetBalanceAmount(math.MaxInt64).
			AddRevision(1).
			Save(ctx); err != nil {
			t.Fatalf("构造 MaxInt64 余额失败: %v", err)
		}
		revision := walletRevisionOf(t, client, integrationUserID)

		code := bizCodeOfIntegration(t, adminAdjustErrorOf(svc, ctx, adminAdjustRequest(
			formatDecimalID(integrationUserID), "overflow-up", 1, "overflow", integrationOperatorID)))
		if code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_STATE_CONFLICT) {
			t.Fatalf("上溢应 STATE_CONFLICT，实际 code=%d", code)
		}
		if got := walletBalanceOf(t, client, integrationUserID); got != math.MaxInt64 {
			t.Fatalf("余额不得变化，实际 %d", got)
		}
		if got := walletRevisionOf(t, client, integrationUserID); got != revision {
			t.Fatalf("revision 不得变化，实际 %d", got)
		}
		if got := transactionCountOf(t, client, account.ID); got != 0 {
			t.Fatalf("不得留下流水，实际 %d", got)
		}
	})

	t.Run("lower bound", func(t *testing.T) {
		account := seedWallet(t, client, integrationSecondUser)
		if _, err := client.WalletAccount.UpdateOneID(account.ID).
			SetBalanceAmount(math.MinInt64).
			AddRevision(1).
			Save(ctx); err != nil {
			t.Fatalf("构造 MinInt64 余额失败: %v", err)
		}

		code := bizCodeOfIntegration(t, adminAdjustErrorOf(svc, ctx, adminAdjustRequest(
			formatDecimalID(integrationSecondUser), "overflow-down", -1, "overflow", integrationOperatorID)))
		if code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_STATE_CONFLICT) {
			t.Fatalf("下溢应 STATE_CONFLICT，实际 code=%d", code)
		}
		if got := walletBalanceOf(t, client, integrationSecondUser); got != math.MinInt64 {
			t.Fatalf("余额不得变化，实际 %d", got)
		}
		if got := transactionCountOf(t, client, account.ID); got != 0 {
			t.Fatalf("不得留下流水，实际 %d", got)
		}
	})
}

// failTransactionCreateHook 在开启时拦截任意 WalletTransaction 创建。
type failTransactionCreateHook struct {
	mu   sync.Mutex
	fail bool
}

func (h *failTransactionCreateHook) setFail(fail bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fail = fail
}

func (h *failTransactionCreateHook) hook() ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			h.mu.Lock()
			fail := h.fail
			h.mu.Unlock()
			if fail && mutation.Op().Is(ent.OpCreate) {
				if _, ok := mutation.(*ent.WalletTransactionMutation); ok {
					return nil, errInjectedTransactionCreate
				}
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

// TestWalletAdminAdjustmentRollbackMySQL 冻结事务原子性：
// 流水写入失败时，余额更新、revision 与首次建行都必须回滚。
func TestWalletAdminAdjustmentRollbackMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	hook := &failTransactionCreateHook{}
	client.WalletTransaction.Use(hook.hook())

	t.Run("existing wallet rolls back", func(t *testing.T) {
		account := seedWallet(t, client, integrationUserID)
		if _, err := client.WalletAccount.UpdateOneID(account.ID).
			SetBalanceAmount(700).
			AddRevision(1).
			Save(ctx); err != nil {
			t.Fatalf("构造前置余额失败: %v", err)
		}
		revision := walletRevisionOf(t, client, integrationUserID)

		hook.setFail(true)
		code := bizCodeOfIntegration(t, adminAdjustErrorOf(svc, ctx, adminAdjustRequest(
			formatDecimalID(integrationUserID), "rollback-1", 300, "rollback", integrationOperatorID)))
		hook.setFail(false)
		if code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INTERNAL) {
			t.Fatalf("流水失败应 INTERNAL，实际 code=%d", code)
		}
		if got := walletBalanceOf(t, client, integrationUserID); got != 700 {
			t.Fatalf("余额必须回滚到 700，实际 %d", got)
		}
		if got := walletRevisionOf(t, client, integrationUserID); got != revision {
			t.Fatalf("revision 必须回滚，实际 %d", got)
		}
		if got := transactionCountOf(t, client, account.ID); got != 0 {
			t.Fatalf("不得留下流水，实际 %d", got)
		}

		if _, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
			formatDecimalID(integrationUserID), "rollback-1", 300, "rollback", integrationOperatorID)); err != nil {
			t.Fatalf("关闭注入后同一请求应成功: %v", err)
		}
		if got := walletBalanceOf(t, client, integrationUserID); got != 1000 {
			t.Fatalf("成功调账后余额应为 1000，实际 %d", got)
		}
	})

	t.Run("first wallet creation rolls back", func(t *testing.T) {
		user := integrationSecondUser
		if walletAccountOf(t, client, user) != nil {
			t.Fatalf("夹具要求用户 %d 无钱包", user)
		}

		hook.setFail(true)
		code := bizCodeOfIntegration(t, adminAdjustErrorOf(svc, ctx, adminAdjustRequest(
			formatDecimalID(user), "rollback-2", 100, "rollback", integrationOperatorID)))
		hook.setFail(false)
		if code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INTERNAL) {
			t.Fatalf("流水失败应 INTERNAL，实际 code=%d", code)
		}
		if walletAccountOf(t, client, user) != nil {
			t.Fatal("首次建行也必须回滚")
		}

		if _, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
			formatDecimalID(user), "rollback-2", 100, "rollback", integrationOperatorID)); err != nil {
			t.Fatalf("关闭注入后应成功: %v", err)
		}
		if got := walletBalanceOf(t, client, user); got != 100 {
			t.Fatalf("余额应为 100，实际 %d", got)
		}
	})
}

// TestWalletAdminAdjustmentDebtMySQL 冻结欠款与消费阻断：
// 调账可制造并抵消欠款，但普通消费在欠款或 Provider 阻断期间仍被拒绝。
func TestWalletAdminAdjustmentDebtMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	adjust := func(requestID string, delta int64) {
		t.Helper()
		if _, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
			formatDecimalID(integrationUserID), requestID, delta, "debt", integrationOperatorID)); err != nil {
			t.Fatalf("调账 %s 失败: %v", requestID, err)
		}
	}

	adjust("debt-1", -500)
	if got := walletBalanceOf(t, client, integrationUserID); got != -500 {
		t.Fatalf("余额应为 -500，实际 %d", got)
	}

	view, err := svc.GetWallet(ctx, &wallet.GetWalletReq{UserId: formatDecimalID(integrationUserID)})
	if err != nil {
		t.Fatalf("查询钱包失败: %v", err)
	}
	if view.GetWallet().GetStatus() != wallet.WalletStatus_WALLET_STATUS_DEBT {
		t.Fatalf("负余额应显示 DEBT，实际 %v", view.GetWallet().GetStatus())
	}

	if code := bizCodeOfIntegration(t, errOfSecondCall(svc, ctx, &wallet.DebitBalanceReq{
		UserId:       formatDecimalID(integrationUserID),
		Currency:     currencyCNY,
		Amount:       1,
		BusinessType: "order",
		BusinessId:   "debt-debit-1",
	})); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INSUFFICIENT_BALANCE) {
		t.Fatalf("欠款期间消费应 INSUFFICIENT_BALANCE，实际 code=%d", code)
	}

	adjust("debt-2", 200)
	if view, err = svc.GetWallet(ctx, &wallet.GetWalletReq{UserId: formatDecimalID(integrationUserID)}); err != nil {
		t.Fatalf("查询钱包失败: %v", err)
	}
	if view.GetWallet().GetStatus() != wallet.WalletStatus_WALLET_STATUS_DEBT {
		t.Fatalf("仍欠款时应保持 DEBT，实际 %v", view.GetWallet().GetStatus())
	}

	adjust("debt-3", 300)
	if view, err = svc.GetWallet(ctx, &wallet.GetWalletReq{UserId: formatDecimalID(integrationUserID)}); err != nil {
		t.Fatalf("查询钱包失败: %v", err)
	}
	if view.GetWallet().GetStatus() != wallet.WalletStatus_WALLET_STATUS_ACTIVE ||
		view.GetWallet().GetBalanceAmount() != 0 {
		t.Fatalf("抵消欠款后应 ACTIVE/0，实际 %+v", view.GetWallet())
	}

	// Provider 逆向事故阻断期间：调账仍成功，普通消费仍被拒。
	account := walletAccountOf(t, client, integrationUserID)
	if account == nil {
		t.Fatal("缺少钱包")
	}
	seedDebitBlock(t, client, account.ID, "admin-adjust")
	adjust("debt-4", 100)
	adjust("debt-5", -100)
	if got := walletBalanceOf(t, client, integrationUserID); got != 0 {
		t.Fatalf("阻断期间调账应生效，余额应为 0，实际 %d", got)
	}
	if code := bizCodeOfIntegration(t, errOfSecondCall(svc, ctx, &wallet.DebitBalanceReq{
		UserId:       formatDecimalID(integrationUserID),
		Currency:     currencyCNY,
		Amount:       1,
		BusinessType: "order",
		BusinessId:   "blocked-debit-1",
	})); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_BALANCE_BLOCKED) {
		t.Fatalf("阻断期间消费应 BALANCE_BLOCKED，实际 code=%d", code)
	}
}

// TestWalletAdminAdjustmentQueryMySQL 冻结查询口径：
// 用户视图只含用户字段，管理员视图带完整审计信息。
func TestWalletAdminAdjustmentQueryMySQL(t *testing.T) {
	svc, _, _ := balanceTestService(t)
	ctx := context.Background()

	if _, err := svc.AdminAdjustBalance(ctx, adminAdjustRequest(
		formatDecimalID(integrationUserID), "query-adjust-1", 250, "query", integrationOperatorID)); err != nil {
		t.Fatalf("调账失败: %v", err)
	}

	adminAdjustmentType := wallet.TransactionType_TRANSACTION_TYPE_ADMIN_ADJUSTMENT
	userPage, err := svc.ListTransactions(ctx, &wallet.ListTransactionsReq{
		UserId:   formatDecimalID(integrationUserID),
		Type:     &adminAdjustmentType,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("用户按类型查询失败: %v", err)
	}
	if len(userPage.GetItems()) != 1 {
		t.Fatalf("用户应查到 1 条调账流水，实际 %d", len(userPage.GetItems()))
	}
	item := userPage.GetItems()[0]
	if item.GetType() != wallet.TransactionType_TRANSACTION_TYPE_ADMIN_ADJUSTMENT || item.GetDeltaAmount() != 250 || item.GetDescription() != "query" {
		t.Fatalf("用户流水字段错误: %+v", item)
	}

	adminPage, err := svc.AdminListTransactions(ctx, &wallet.AdminListTransactionsReq{
		Type:     &adminAdjustmentType,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("管理员按类型查询失败: %v", err)
	}
	if len(adminPage.GetItems()) != 1 {
		t.Fatalf("管理员应查到 1 条调账流水，实际 %d", len(adminPage.GetItems()))
	}
	adminItem := adminPage.GetItems()[0]
	if adminItem.GetActor() != "admin:"+integrationOperatorID ||
		adminItem.GetBusinessType() != businessTypeAdminAdjustment ||
		adminItem.GetBusinessId() != "query-adjust-1" ||
		adminItem.GetOwnerNamespace() != testOwnerNamespace {
		t.Fatalf("管理员审计字段错误: %+v", adminItem)
	}
	if adminItem.GetUserId() != formatDecimalID(integrationUserID) {
		t.Fatalf("管理员视图应带 user_id，实际 %q", adminItem.GetUserId())
	}
}

// TestWalletAdminAdjustmentGuardMySQL 冻结运行策略：
// 功能关闭后同请求被 guard 拒绝且账本不变，恢复后重放返回原交易。
func TestWalletAdminAdjustmentGuardMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	account := seedWallet(t, client, integrationUserID)
	req := adminAdjustRequest(formatDecimalID(integrationUserID), "guard-1", 100, "guard", integrationOperatorID)

	first, err := svc.AdminAdjustBalance(ctx, req)
	if err != nil {
		t.Fatalf("启用状态下调账失败: %v", err)
	}
	revision := walletRevisionOf(t, client, integrationUserID)
	count := transactionCountOf(t, client, account.ID)

	svc.Runtime = walletRuntimeWith(t, func(snap *setting.WalletRuntimeSettings) {
		snap.Enabled = false
	})
	code := bizCodeOfIntegration(t, adminAdjustErrorOf(svc, ctx, req))
	if code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED) {
		t.Fatalf("功能关闭应 FEATURE_DISABLED，实际 code=%d", code)
	}
	if got := walletBalanceOf(t, client, integrationUserID); got != 100 {
		t.Fatalf("被拒绝的重放不得改余额，实际 %d", got)
	}
	if got := walletRevisionOf(t, client, integrationUserID); got != revision {
		t.Fatalf("被拒绝的重放不得改 revision，实际 %d", got)
	}
	if got := transactionCountOf(t, client, account.ID); got != count {
		t.Fatalf("被拒绝的重放不得新增流水，实际 %d", got)
	}

	svc.Runtime = walletRuntimeWith(t, nil)
	replay, err := svc.AdminAdjustBalance(ctx, req)
	if err != nil {
		t.Fatalf("恢复快照后应重放成功: %v", err)
	}
	if replay.GetTransaction().GetTransaction().GetId() != first.GetTransaction().GetTransaction().GetId() {
		t.Fatalf("应返回原交易，实际 %s", replay.GetTransaction().GetTransaction().GetId())
	}
}
