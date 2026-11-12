//go:build integration

package wallet

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/redemptioncode"
	"github.com/KiClover/orzAdmin/ent/walletaccount"
	"github.com/KiClover/orzAdmin/ent/wallettransaction"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	walletpb "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// redemptionTestService 准备已建表、已打开兑换开关的 handler。
func redemptionTestService(t *testing.T) (*CommerceWalletService, *ent.Client) {
	t.Helper()

	svc, client, rawdb := integrationWalletService(t)
	svc.Cfg = &conf.Config{Wallet: conf.WalletConf{OwnerNamespace: testOwnerNamespace}}
	svc.DB = db.NewCommerceWalletDB(client)

	runtime, err := NewService(Options{
		Source:   &fakeSettingsSource{snap: redemptionSnapshot()},
		Presence: svc.DB,
	})
	if err != nil {
		t.Fatalf("构造运行时服务失败: %v", err)
	}
	runtime.now = func() time.Time { return testNow }
	if err := runtime.Refresh(context.Background()); err != nil {
		t.Fatalf("加载运行时快照失败: %v", err)
	}
	svc.Runtime = runtime

	dropWalletTables(t, rawdb)
	if _, err := svc.InitDatabase(context.Background(), &base.Empty{}); err != nil {
		t.Fatalf("初始化钱包表失败: %v", err)
	}
	return svc, client
}

// createCode 通过管理员 RPC 创建兑换码。
func createCode(t *testing.T, svc *CommerceWalletService, requestID string, codeType walletpb.RedemptionCodeType, amount int64) *walletpb.AdminRedemptionCodeView {
	t.Helper()

	resp, err := svc.AdminCreateRedemptionCode(context.Background(), &walletpb.AdminCreateRedemptionCodeReq{
		OperatorId: "9001",
		RequestId:  requestID,
		Type:       codeType,
		Amount:     amount,
	})
	if err != nil {
		t.Fatalf("创建兑换码失败: %v", err)
	}
	return resp.GetCode()
}

// redeem 调用用户兑换 RPC。
func redeem(svc *CommerceWalletService, userID, code string) (*walletpb.RedeemCodeResp, error) {
	return svc.RedeemCode(context.Background(), &walletpb.RedeemCodeReq{
		UserId: userID,
		Code:   code,
	})
}

// walletAccountOf 读取指定用户的钱包，不存在时返回 nil。
func walletAccountOf(t *testing.T, client *ent.Client, userID uint64) *ent.WalletAccount {
	t.Helper()

	account, err := client.WalletAccount.Query().
		Where(walletaccount.UserID(userID)).
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		t.Fatalf("读取钱包失败: %v", err)
	}
	return account
}

// redemptionCreditCount 统计某钱包的兑换入账流水条数。
func redemptionCreditCount(t *testing.T, client *ent.Client, walletID uint64) int {
	t.Helper()

	count, err := client.WalletTransaction.Query().
		Where(
			wallettransaction.WalletID(walletID),
			wallettransaction.Type(txTypeRedemptionCredit),
		).
		Count(context.Background())
	if err != nil {
		t.Fatalf("统计兑换流水失败: %v", err)
	}
	return count
}

func TestRedemptionUniqueMySQL(t *testing.T) {
	svc, client := redemptionTestService(t)

	code := createCode(t, svc, "req-unique-1", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE, 500)
	if code.GetStatus() != walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE {
		t.Fatalf("新建码状态 = %v", code.GetStatus())
	}
	if len(code.GetCode()) != redemptionCodeLength {
		t.Fatalf("明文码长度 = %d", len(code.GetCode()))
	}

	first, err := redeem(svc, "7001", code.GetCode())
	if err != nil {
		t.Fatalf("首次兑换失败: %v", err)
	}
	if first.GetCreditedAmount() != 500 {
		t.Fatalf("入账金额 = %d", first.GetCreditedAmount())
	}
	if first.GetWallet().GetBalanceAmount() != 500 {
		t.Fatalf("余额 = %d", first.GetWallet().GetBalanceAmount())
	}

	if _, err := redeem(svc, "7002", code.GetCode()); err == nil {
		t.Fatalf("UNIQUE 码不应被第二个用户兑换")
	} else if got := bizCodeOfIntegration(t, err); got != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND) {
		t.Fatalf("重复兑换错误码 = %d，期望 NOT_FOUND", got)
	}

	account := walletAccountOf(t, client, 7001)
	if account == nil {
		t.Fatalf("兑换后应创建钱包")
	}
	if got := redemptionCreditCount(t, client, account.ID); got != 1 {
		t.Fatalf("兑换入账流水 = %d，期望 1", got)
	}

	row, err := client.RedemptionCode.Query().Where(redemptioncode.RequestID("req-unique-1")).Only(context.Background())
	if err != nil {
		t.Fatalf("读取兑换码失败: %v", err)
	}
	if row.Status != redemptionStatusClaimed {
		t.Fatalf("数据库状态 = %q，期望 CLAIMED", row.Status)
	}
	if row.RedeemedCount != 1 {
		t.Fatalf("redeemed_count = %d", row.RedeemedCount)
	}
}

func TestRedemptionSharedMySQL(t *testing.T) {
	svc, client := redemptionTestService(t)

	code := createCode(t, svc, "req-shared-1", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 300)

	if _, err := redeem(svc, "7001", code.GetCode()); err != nil {
		t.Fatalf("用户 7001 首次兑换失败: %v", err)
	}
	if _, err := redeem(svc, "7001", code.GetCode()); err == nil {
		t.Fatalf("同一用户不应重复兑换 SHARED 码")
	}
	if _, err := redeem(svc, "7002", code.GetCode()); err != nil {
		t.Fatalf("不同用户应可兑换 SHARED 码: %v", err)
	}

	if got := walletBalanceOf(t, client, 7001); got != 300 {
		t.Fatalf("7001 余额 = %d", got)
	}
	if got := walletBalanceOf(t, client, 7002); got != 300 {
		t.Fatalf("7002 余额 = %d", got)
	}

	row, err := client.RedemptionCode.Query().Where(redemptioncode.RequestID("req-shared-1")).Only(context.Background())
	if err != nil {
		t.Fatalf("读取兑换码失败: %v", err)
	}
	if row.Status != redemptionStatusActive {
		t.Fatalf("SHARED 码状态 = %q，期望 ACTIVE", row.Status)
	}
	if row.RedeemedCount != 2 {
		t.Fatalf("redeemed_count = %d，期望 2", row.RedeemedCount)
	}
}

func TestRedemptionExpiredMySQL(t *testing.T) {
	svc, client := redemptionTestService(t)

	code, err := newRedemptionCode()
	if err != nil {
		t.Fatalf("生成兑换码失败: %v", err)
	}
	past := time.Now().Add(-time.Minute)
	if _, err := client.RedemptionCode.Create().
		SetRequestID("req-expired-1").
		SetType(redemptionCodeTypeUnique).
		SetCode(code).
		SetAmount(500).
		SetCurrency(currencyCNY).
		SetStatus(redemptionStatusActive).
		SetExpiresAt(past).
		SetRedeemedCount(0).
		SetCreatedBy("9001").
		Save(context.Background()); err != nil {
		t.Fatalf("写入过期兑换码失败: %v", err)
	}

	if _, err := redeem(svc, "7001", code); err == nil {
		t.Fatalf("过期兑换码不应可用")
	}

	// 管理员列表的 EXPIRED 过滤器必须能查出它。
	expiredStatus := walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_EXPIRED
	resp, err := svc.AdminListRedemptionCodes(context.Background(), &walletpb.AdminListRedemptionCodesReq{
		Status: &expiredStatus,
	})
	if err != nil {
		t.Fatalf("按 EXPIRED 过滤失败: %v", err)
	}
	if len(resp.GetItems()) != 1 {
		t.Fatalf("EXPIRED 过滤条数 = %d，期望 1", len(resp.GetItems()))
	}
	if resp.GetItems()[0].GetStatus() != walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_EXPIRED {
		t.Fatalf("展示状态 = %v", resp.GetItems()[0].GetStatus())
	}
}

func TestRedemptionStatusRulesMySQL(t *testing.T) {
	svc, _ := redemptionTestService(t)
	ctx := context.Background()

	code := createCode(t, svc, "req-status-1", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE, 200)

	if _, err := svc.AdminSetRedemptionCodeStatus(ctx, &walletpb.AdminSetRedemptionCodeStatusReq{
		OperatorId: "9001",
		CodeId:     code.GetId(),
		Status:     walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_DISABLED,
	}); err != nil {
		t.Fatalf("禁用兑换码失败: %v", err)
	}
	if _, err := redeem(svc, "7001", code.GetCode()); err == nil {
		t.Fatalf("禁用兑换码不应可兑换")
	}

	if _, err := svc.AdminSetRedemptionCodeStatus(ctx, &walletpb.AdminSetRedemptionCodeStatusReq{
		OperatorId: "9001",
		CodeId:     code.GetId(),
		Status:     walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE,
	}); err != nil {
		t.Fatalf("重新启用兑换码失败: %v", err)
	}
	if _, err := redeem(svc, "7001", code.GetCode()); err != nil {
		t.Fatalf("重新启用后应可兑换: %v", err)
	}

	// CLAIMED 之后不得再改状态。
	_, err := svc.AdminSetRedemptionCodeStatus(ctx, &walletpb.AdminSetRedemptionCodeStatusReq{
		OperatorId: "9001",
		CodeId:     code.GetId(),
		Status:     walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_DISABLED,
	})
	if got := bizCodeOfIntegration(t, err); got != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_STATE_CONFLICT) {
		t.Fatalf("修改 CLAIMED 码错误码 = %d", got)
	}
}

func TestRedemptionAdminReplayMySQL(t *testing.T) {
	svc, _ := redemptionTestService(t)
	ctx := context.Background()

	first := createCode(t, svc, "req-replay-1", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 100)
	second := createCode(t, svc, "req-replay-1", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 100)
	if first.GetId() != second.GetId() || first.GetCode() != second.GetCode() {
		t.Fatalf("相同请求重放应返回同一兑换码: %s/%s", first.GetCode(), second.GetCode())
	}

	_, err := svc.AdminCreateRedemptionCode(ctx, &walletpb.AdminCreateRedemptionCodeReq{
		OperatorId: "9001",
		RequestId:  "req-replay-1",
		Type:       walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED,
		Amount:     101,
	})
	if got := bizCodeOfIntegration(t, err); got != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT) {
		t.Fatalf("相同请求不同金额错误码 = %d", got)
	}

	// 分页：再造一条，两页各 1 条，游标稳定。
	createCode(t, svc, "req-replay-2", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE, 200)

	list1, err := svc.AdminListRedemptionCodes(ctx, &walletpb.AdminListRedemptionCodesReq{PageSize: 1})
	if err != nil {
		t.Fatalf("列出兑换码失败: %v", err)
	}
	if len(list1.GetItems()) != 1 || !list1.GetHasMore() {
		t.Fatalf("第一页 = %d 条 has_more=%v", len(list1.GetItems()), list1.GetHasMore())
	}
	list2, err := svc.AdminListRedemptionCodes(ctx, &walletpb.AdminListRedemptionCodesReq{
		PageSize: 1,
		Cursor:   list1.GetNextCursor(),
	})
	if err != nil {
		t.Fatalf("第二页列出兑换码失败: %v", err)
	}
	if len(list2.GetItems()) != 1 || list2.GetHasMore() {
		t.Fatalf("第二页 = %d 条 has_more=%v", len(list2.GetItems()), list2.GetHasMore())
	}
	if list1.GetItems()[0].GetId() == list2.GetItems()[0].GetId() {
		t.Fatalf("翻页出现重复条目")
	}
}

func TestRedemptionConcurrencyMySQL(t *testing.T) {
	svc, client := redemptionTestService(t)

	t.Run("unique code wins exactly once", func(t *testing.T) {
		code := createCode(t, svc, "req-race-unique", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE, 500)

		const workers = 8
		var wait sync.WaitGroup
		wait.Add(workers)
		results := make(chan error, workers)
		for i := 0; i < workers; i++ {
			userID := fmt.Sprintf("%d", 7101+i)
			go func() {
				defer wait.Done()
				_, err := redeem(svc, userID, code.GetCode())
				results <- err
			}()
		}
		wait.Wait()
		close(results)

		success := 0
		for err := range results {
			if err == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatalf("并发兑换成功次数 = %d，期望 1", success)
		}

		total := int64(0)
		for i := 0; i < workers; i++ {
			account := walletAccountOf(t, client, uint64(7101+i))
			if account == nil {
				continue
			}
			total += account.BalanceAmount
			if got := redemptionCreditCount(t, client, account.ID); got != 1 {
				t.Fatalf("用户 %d 兑换流水 = %d", 7101+i, got)
			}
		}
		if total != 500 {
			t.Fatalf("并发入账总额 = %d，期望 500", total)
		}
	})

	t.Run("shared code allows each user once", func(t *testing.T) {
		code := createCode(t, svc, "req-race-shared", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 300)

		const workers = 10
		var wait sync.WaitGroup
		wait.Add(workers)
		results := make(chan error, workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer wait.Done()
				_, err := redeem(svc, "7201", code.GetCode())
				results <- err
			}()
		}
		wait.Wait()
		close(results)

		success := 0
		for err := range results {
			if err == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatalf("同一用户并发兑换成功次数 = %d，期望 1", success)
		}
		account := walletAccountOf(t, client, 7201)
		if account == nil || account.BalanceAmount != 300 {
			t.Fatalf("同一用户余额错误: %+v", account)
		}
		if got := redemptionCreditCount(t, client, account.ID); got != 1 {
			t.Fatalf("同一用户兑换流水 = %d，期望 1", got)
		}
	})

	t.Run("shared code serves different users", func(t *testing.T) {
		code := createCode(t, svc, "req-race-shared-multi", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 100)

		const workers = 6
		var wait sync.WaitGroup
		wait.Add(workers)
		results := make(chan error, workers)
		for i := 0; i < workers; i++ {
			userID := fmt.Sprintf("%d", 7301+i)
			go func() {
				defer wait.Done()
				_, err := redeem(svc, userID, code.GetCode())
				results <- err
			}()
		}
		wait.Wait()
		close(results)

		success := 0
		for err := range results {
			if err == nil {
				success++
			}
		}
		if success != workers {
			t.Fatalf("不同用户并发兑换成功次数 = %d，期望 %d", success, workers)
		}
	})
}
