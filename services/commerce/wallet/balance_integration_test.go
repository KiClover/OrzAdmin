//go:build integration

package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/walletaccount"
	"github.com/KiClover/orzAdmin/ent/wallettransaction"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/cloudwego/kitex/pkg/kerrors"
)

// 任务 4 的资金集成测试：余额与流水必须同事务、并发不超扣、补偿只成功一次。
// 需要真实 MySQL：WALLET_TEST_MYSQL_DSN，例如
// user:pass@tcp(127.0.0.1:3306)/orz_wallet_test?parseTime=True
//
// 运行：
//   go test -tags=integration ./services/commerce/wallet \
//     -run 'TestWallet(BalanceConcurrency|Replay|RefundRace)MySQL$' -count=1

const (
	integrationUserID      uint64 = 7001
	integrationSecondUser  uint64 = 7002
	integrationThirdUser   uint64 = 7003
	integrationProviderEnv        = "TEST"
)

// balanceTestService 提供已建表、已启用运行时快照的 handler。
func balanceTestService(t *testing.T) (*CommerceWalletService, *ent.Client, *sql.DB) {
	t.Helper()

	svc, client, rawdb := integrationWalletService(t)
	svc.Cfg = &conf.Config{Wallet: conf.WalletConf{OwnerNamespace: testOwnerNamespace}}
	svc.DB = db.NewCommerceWalletDB(client)
	svc.Runtime = enabledWalletRuntime(t, svc.DB)

	dropWalletTables(t, rawdb)
	if _, err := svc.InitDatabase(context.Background(), &base.Empty{}); err != nil {
		t.Fatalf("初始化钱包表失败: %v", err)
	}
	return svc, client, rawdb
}

func enabledWalletRuntime(t *testing.T, presence DataPresence) *Service {
	t.Helper()

	runtime, err := NewService(Options{
		Source:   &fakeSettingsSource{snap: validSnapshot(11)},
		Presence: presence,
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

// creditWallet 是任务 4 的测试夹具：充值入账属于任务 5 的记账入口，
// 此处在事务外直接写入账户余额与一条历史流水，只为构造扣款前置条件。
func creditWallet(t *testing.T, client *ent.Client, userID uint64, amount int64, businessID string) *ent.WalletTransaction {
	t.Helper()

	ctx := context.Background()
	account, err := client.WalletAccount.Create().
		SetUserID(userID).
		SetCurrency(currencyCNY).
		SetBalanceAmount(amount).
		Save(ctx)
	if err != nil {
		t.Fatalf("创建钱包失败: %v", err)
	}

	transaction, err := client.WalletTransaction.Create().
		SetWalletID(account.ID).
		SetType(txTypeRechargeCredit).
		SetCurrency(currencyCNY).
		SetDeltaAmount(amount).
		SetBalanceAfter(amount).
		SetOwnerNamespace(testOwnerNamespace).
		SetBusinessType("recharge").
		SetBusinessID(businessID).
		SetDescription("integration seed").
		SetActor("admin").
		SetCreatedAt(testNow).
		Save(ctx)
	if err != nil {
		t.Fatalf("写入充值流水失败: %v", err)
	}
	return transaction
}

func walletBalanceOf(t *testing.T, client *ent.Client, userID uint64) int64 {
	t.Helper()

	account, err := client.WalletAccount.Query().Where(walletaccount.UserID(userID)).Only(context.Background())
	if err != nil {
		t.Fatalf("读取钱包失败: %v", err)
	}
	return account.BalanceAmount
}

func transactionCountOf(t *testing.T, client *ent.Client, walletID uint64) int {
	t.Helper()

	count, err := client.WalletTransaction.Query().Where(wallettransaction.WalletID(walletID)).Count(context.Background())
	if err != nil {
		t.Fatalf("统计流水失败: %v", err)
	}
	return count
}

func bizCodeOfIntegration(t *testing.T, err error) int32 {
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

func TestWalletBalanceConcurrencyMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	t.Run("serialized debits never overdraw", func(t *testing.T) {
		seed := creditWallet(t, client, integrationUserID, 1000, "seed-1")
		walletID := seed.WalletID

		const attempts = 20
		var (
			wait     sync.WaitGroup
			mu       sync.Mutex
			accepted int
			rejected int
		)
		for i := 0; i < attempts; i++ {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				_, err := svc.DebitBalance(ctx, &wallet.DebitBalanceReq{
					UserId:       formatDecimalID(integrationUserID),
					Currency:     currencyCNY,
					Amount:       100,
					BusinessType: "order",
					BusinessId:   fmt.Sprintf("concurrent-%d", index),
				})
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					accepted++
				case bizCodeOfIntegration(t, err) == int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INSUFFICIENT_BALANCE):
					rejected++
				default:
					t.Errorf("意外的扣款错误: %v", err)
				}
			}(i)
		}
		wait.Wait()

		if accepted != 10 || rejected != 10 {
			t.Fatalf("期望 10 成功 / 10 余额不足，实际 %d / %d", accepted, rejected)
		}
		if balance := walletBalanceOf(t, client, integrationUserID); balance != 0 {
			t.Fatalf("并发扣款后余额应为 0，实际 %d", balance)
		}
		if count := transactionCountOf(t, client, walletID); count != attempts/2+1 {
			t.Fatalf("流水数应为 11（1 充值 + 10 扣款），实际 %d", count)
		}
	})

	t.Run("same business key records once", func(t *testing.T) {
		secondUser := integrationSecondUser
		credit := creditWallet(t, client, secondUser, 5000, "seed-2")

		const attempts = 16
		var (
			wait sync.WaitGroup
			mu   sync.Mutex
			ids  = make(map[string]struct{})
		)
		for i := 0; i < attempts; i++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				resp, err := svc.DebitBalance(ctx, &wallet.DebitBalanceReq{
					UserId:       formatDecimalID(secondUser),
					Currency:     currencyCNY,
					Amount:       100,
					BusinessType: "order",
					BusinessId:   "same-key-1",
				})
				if err != nil {
					t.Errorf("重放应幂等成功: %v", err)
					return
				}
				mu.Lock()
				defer mu.Unlock()
				ids[resp.GetTransaction().GetId()] = struct{}{}
			}()
		}
		wait.Wait()

		if len(ids) != 1 {
			t.Fatalf("同一业务键只应产生一条交易，实际 %d 条", len(ids))
		}
		if balance := walletBalanceOf(t, client, secondUser); balance != 4900 {
			t.Fatalf("只应扣款一次，余额应为 4900，实际 %d", balance)
		}
		if count := transactionCountOf(t, client, credit.WalletID); count != 2 {
			t.Fatalf("流水数应为 2，实际 %d", count)
		}
	})
}

func TestWalletReplayMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	seed := creditWallet(t, client, integrationUserID, 2000, "seed-replay")
	command := func(amount int64, businessType string) *wallet.DebitBalanceReq {
		return &wallet.DebitBalanceReq{
			UserId:       formatDecimalID(integrationUserID),
			Currency:     currencyCNY,
			Amount:       amount,
			BusinessType: businessType,
			BusinessId:   "order-1",
		}
	}

	first, err := svc.DebitBalance(ctx, command(300, "order"))
	if err != nil {
		t.Fatalf("首次扣款失败: %v", err)
	}
	replay, err := svc.DebitBalance(ctx, command(300, "order"))
	if err != nil {
		t.Fatalf("完全重放应成功: %v", err)
	}
	if replay.GetTransaction().GetId() != first.GetTransaction().GetId() {
		t.Fatalf("重放应返回原交易 %s，实际 %s",
			first.GetTransaction().GetId(), replay.GetTransaction().GetId())
	}

	if code := bizCodeOfIntegration(t, errOfSecondCall(svc, ctx, command(301, "order"))); code !=
		int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT) {
		t.Fatalf("同键不同金额应 IDEMPOTENCY_CONFLICT，实际 code=%d", code)
	}

	if _, err := svc.DebitBalance(ctx, command(300, "another-type")); err != nil {
		t.Fatalf("不同业务类型是独立业务键: %v", err)
	}

	if balance := walletBalanceOf(t, client, integrationUserID); balance != 1400 {
		t.Fatalf("余额应为 2000-600=1400，实际 %d", balance)
	}
	if count := transactionCountOf(t, client, seed.WalletID); count != 3 {
		t.Fatalf("流水数应为 3（1 充值 + 2 扣款），实际 %d", count)
	}
}

func errOfSecondCall(svc *CommerceWalletService, ctx context.Context, req *wallet.DebitBalanceReq) error {
	_, err := svc.DebitBalance(ctx, req)
	return err
}

func TestWalletRefundRaceMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	seed := creditWallet(t, client, integrationUserID, 1000, "seed-refund")
	debit, err := svc.DebitBalance(ctx, &wallet.DebitBalanceReq{
		UserId:       formatDecimalID(integrationUserID),
		Currency:     currencyCNY,
		Amount:       900,
		BusinessType: "order",
		BusinessId:   "order-refund-1",
	})
	if err != nil {
		t.Fatalf("扣款失败: %v", err)
	}

	const attempts = 8
	var (
		wait sync.WaitGroup
		mu   sync.Mutex
		ids  = make(map[string]struct{})
	)
	for i := 0; i < attempts; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			resp, err := svc.RefundBalance(ctx, &wallet.RefundBalanceReq{
				RelatedTransactionId: debit.GetTransaction().GetId(),
			})
			if err != nil {
				t.Errorf("补偿应幂等成功: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			ids[resp.GetTransaction().GetId()] = struct{}{}
		}()
	}
	wait.Wait()

	if len(ids) != 1 {
		t.Fatalf("同一笔扣款只能补偿一次，实际 %d 条补偿", len(ids))
	}
	if balance := walletBalanceOf(t, client, integrationUserID); balance != 1000 {
		t.Fatalf("补偿后余额应恢复为 1000，实际 %d", balance)
	}
	if count := transactionCountOf(t, client, seed.WalletID); count != 3 {
		t.Fatalf("流水数应为 3（充值/扣款/补偿），实际 %d", count)
	}

	// 非 BALANCE_DEBIT 的交易不能作为补偿对象。
	if code := bizCodeOfIntegration(t, errOfRefund(svc, ctx, formatDecimalID(seed.ID))); code !=
		int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
		t.Fatalf("补偿非扣款交易应 INVALID_ARGUMENT，实际 code=%d", code)
	}

	// 其它 owner namespace 的交易按不存在处理，避免探测。
	other := *svc
	other.Cfg = &conf.Config{Wallet: conf.WalletConf{OwnerNamespace: "another"}}
	if code := bizCodeOfIntegration(t, errOfRefund(&other, ctx, debit.GetTransaction().GetId())); code !=
		int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND) {
		t.Fatalf("跨 namespace 补偿应 NOT_FOUND，实际 code=%d", code)
	}
}

func errOfRefund(svc *CommerceWalletService, ctx context.Context, relatedID string) error {
	_, err := svc.RefundBalance(ctx, &wallet.RefundBalanceReq{RelatedTransactionId: relatedID})
	return err
}

func TestWalletDebtRulesMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	seed := creditWallet(t, client, integrationUserID, 500, "seed-debt")
	// 充值退款允许把余额扣成负数。
	if _, err := svc.applyBalanceChange(ctx, balanceCommand{
		UserID:               integrationUserID,
		Currency:             currencyCNY,
		TransactionType:      txTypeRechargeRefund,
		Amount:               800,
		OwnerNamespace:       testOwnerNamespace,
		BusinessType:         "recharge_refund",
		BusinessID:           formatDecimalID(seed.ID),
		RelatedTransactionID: &seed.ID,
		Actor:                "admin",
	}); err != nil {
		t.Fatalf("充值退款应允许负余额: %v", err)
	}
	if balance := walletBalanceOf(t, client, integrationUserID); balance != -300 {
		t.Fatalf("负余额应为 -300，实际 %d", balance)
	}

	// 负余额期间禁止消费。
	if code := bizCodeOfIntegration(t, errOfSecondCall(svc, ctx, &wallet.DebitBalanceReq{
		UserId:       formatDecimalID(integrationUserID),
		Currency:     currencyCNY,
		Amount:       1,
		BusinessType: "order",
		BusinessId:   "debt-debit-1",
	})); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INSUFFICIENT_BALANCE) {
		t.Fatalf("负余额消费应 INSUFFICIENT_BALANCE，实际 code=%d", code)
	}

	// Provider 逆向事故阻断会在消费前拦截。
	account, err := client.WalletAccount.Query().Where(walletaccount.UserID(integrationUserID)).Only(ctx)
	if err != nil {
		t.Fatalf("读取钱包失败: %v", err)
	}
	if _, err := client.WalletRecharge.Create().
		SetWalletID(account.ID).
		SetRequestID("req-block-1").
		SetMerchantOrderNo("merchant-block-1").
		SetProviderEnvironment(integrationProviderEnv).
		SetPaymentProfileID("profile-block-1").
		SetPaymentProfileRevision(1).
		SetProviderProductID("product-block-1").
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

	// 充值入账属于任务 5；这里直接调整余额，确保阻断先于余额不足生效。
	if _, err := client.WalletAccount.UpdateOneID(account.ID).
		SetBalanceAmount(700).
		AddRevision(1).
		Save(ctx); err != nil {
		t.Fatalf("调整余额失败: %v", err)
	}
	if code := bizCodeOfIntegration(t, errOfSecondCall(svc, ctx, &wallet.DebitBalanceReq{
		UserId:       formatDecimalID(integrationUserID),
		Currency:     currencyCNY,
		Amount:       100,
		BusinessType: "order",
		BusinessId:   "blocked-debit-1",
	})); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_BALANCE_BLOCKED) {
		t.Fatalf("阻断期间消费应 BALANCE_BLOCKED，实际 code=%d", code)
	}
}

func TestWalletQueryMySQL(t *testing.T) {
	svc, client, _ := balanceTestService(t)
	ctx := context.Background()

	first := creditWallet(t, client, integrationUserID, 300, "query-seed-1")
	creditWallet(t, client, integrationSecondUser, 200, "query-seed-2")
	creditWallet(t, client, integrationThirdUser, 100, "query-seed-3")

	if _, err := svc.DebitBalance(ctx, &wallet.DebitBalanceReq{
		UserId:       formatDecimalID(integrationUserID),
		Currency:     currencyCNY,
		Amount:       50,
		BusinessType: "order",
		BusinessId:   "query-debit-1",
	}); err != nil {
		t.Fatalf("扣款失败: %v", err)
	}

	t.Run("user wallet view", func(t *testing.T) {
		resp, err := svc.GetWallet(ctx, &wallet.GetWalletReq{UserId: formatDecimalID(integrationUserID)})
		if err != nil {
			t.Fatalf("查询钱包失败: %v", err)
		}
		if resp.GetWallet().GetBalanceAmount() != 250 || resp.GetWallet().GetRevision() != 2 {
			t.Fatalf("钱包视图错误: %+v", resp.GetWallet())
		}

		empty, err := svc.GetWallet(ctx, &wallet.GetWalletReq{UserId: formatDecimalID(999999)})
		if err != nil {
			t.Fatalf("未开户用户应返回零余额视图: %v", err)
		}
		if empty.GetWallet().GetBalanceAmount() != 0 ||
			empty.GetWallet().GetStatus() != wallet.WalletStatus_WALLET_STATUS_ACTIVE {
			t.Fatalf("零余额视图错误: %+v", empty.GetWallet())
		}
	})

	t.Run("transaction pagination", func(t *testing.T) {
		page, err := svc.ListTransactions(ctx, &wallet.ListTransactionsReq{
			UserId:   formatDecimalID(integrationUserID),
			PageSize: 1,
		})
		if err != nil {
			t.Fatalf("查询流水失败: %v", err)
		}
		if len(page.GetItems()) != 1 || !page.GetHasMore() || page.GetNextCursor() == "" {
			t.Fatalf("第一页应有 1 条并带游标: %+v", page)
		}
		second, err := svc.ListTransactions(ctx, &wallet.ListTransactionsReq{
			UserId:   formatDecimalID(integrationUserID),
			PageSize: 1,
			Cursor:   page.GetNextCursor(),
		})
		if err != nil {
			t.Fatalf("翻页失败: %v", err)
		}
		if len(second.GetItems()) != 1 || second.GetHasMore() {
			t.Fatalf("第二页应有 1 条且不再有下一页: %+v", second)
		}
		if second.GetItems()[0].GetId() == page.GetItems()[0].GetId() {
			t.Fatal("翻页不应重复上一页的交易")
		}
	})

	t.Run("admin wallet pagination", func(t *testing.T) {
		page, err := svc.AdminListWallets(ctx, &wallet.AdminListWalletsReq{PageSize: 2})
		if err != nil {
			t.Fatalf("管理员查询钱包失败: %v", err)
		}
		if len(page.GetItems()) != 2 || !page.GetHasMore() || page.GetNextCursor() == "" {
			t.Fatalf("第一页应有 2 条并带游标: %+v", page)
		}
		second, err := svc.AdminListWallets(ctx, &wallet.AdminListWalletsReq{
			PageSize: 2,
			Cursor:   page.GetNextCursor(),
		})
		if err != nil {
			t.Fatalf("管理员翻页失败: %v", err)
		}
		if len(second.GetItems()) != 1 || second.GetHasMore() {
			t.Fatalf("第二页应有 1 条且结束: %+v", second)
		}
	})

	t.Run("admin filters", func(t *testing.T) {
		debtStatus := wallet.WalletStatus_WALLET_STATUS_DEBT
		debtWallet, err := svc.AdminListWallets(ctx, &wallet.AdminListWalletsReq{
			Status: &debtStatus,
		})
		if err != nil {
			t.Fatalf("按状态过滤失败: %v", err)
		}
		if len(debtWallet.GetItems()) != 0 {
			t.Fatalf("当前没有负债钱包: %+v", debtWallet)
		}

		debitType := wallet.TransactionType_TRANSACTION_TYPE_BALANCE_DEBIT
		filtered, err := svc.AdminListTransactions(ctx, &wallet.AdminListTransactionsReq{
			UserId: stringPtr(formatDecimalID(integrationUserID)),
			Type:   &debitType,
		})
		if err != nil {
			t.Fatalf("按类型过滤失败: %v", err)
		}
		if len(filtered.GetItems()) != 1 {
			t.Fatalf("应只有 1 条扣款流水: %+v", filtered)
		}
		if filtered.GetItems()[0].GetWalletId() != formatDecimalID(first.WalletID) {
			t.Fatalf("管理员视图应带 wallet_id: %+v", filtered.GetItems()[0])
		}
	})

	t.Run("admin get wallet validations", func(t *testing.T) {
		if code := bizCodeOfIntegration(t, errOfAdminGetWallet(svc, ctx, &wallet.AdminGetWalletReq{})); code !=
			int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
			t.Fatalf("缺少参数应 INVALID_ARGUMENT，实际 code=%d", code)
		}

		mismatch := &wallet.AdminGetWalletReq{
			UserId:   stringPtr(formatDecimalID(integrationSecondUser)),
			WalletId: stringPtr(formatDecimalID(first.WalletID)),
		}
		if code := bizCodeOfIntegration(t, errOfAdminGetWallet(svc, ctx, mismatch)); code !=
			int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
			t.Fatalf("归属不一致应 INVALID_ARGUMENT，实际 code=%d", code)
		}

		missing := &wallet.AdminGetWalletReq{WalletId: stringPtr(formatDecimalID(999999))}
		if code := bizCodeOfIntegration(t, errOfAdminGetWallet(svc, ctx, missing)); code !=
			int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND) {
			t.Fatalf("不存在的钱包应 NOT_FOUND，实际 code=%d", code)
		}

		ok, err := svc.AdminGetWallet(ctx, &wallet.AdminGetWalletReq{UserId: stringPtr(formatDecimalID(integrationUserID))})
		if err != nil {
			t.Fatalf("按用户查询钱包失败: %v", err)
		}
		if ok.GetWallet().GetUserId() != formatDecimalID(integrationUserID) {
			t.Fatalf("管理员视图应带归属: %+v", ok.GetWallet())
		}
	})

	// 未开户用户的流水查询是空结果，而不是错误。
	missingUser, err := svc.ListTransactions(ctx, &wallet.ListTransactionsReq{
		UserId: formatDecimalID(999999),
	})
	if err != nil {
		t.Fatalf("未开户用户查询流水应成功: %v", err)
	}
	if len(missingUser.GetItems()) != 0 || missingUser.GetHasMore() {
		t.Fatalf("未开户用户不应有流水: %+v", missingUser)
	}
}

func errOfAdminGetWallet(svc *CommerceWalletService, ctx context.Context, req *wallet.AdminGetWalletReq) error {
	_, err := svc.AdminGetWallet(ctx, req)
	return err
}

func stringPtr(value string) *string { return &value }
