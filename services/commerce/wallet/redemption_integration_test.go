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

// createCustomCode 通过管理员 RPC 创建指定明文的兑换码。
func createCustomCode(t *testing.T, svc *CommerceWalletService, requestID string, codeType walletpb.RedemptionCodeType, code string, amount int64) *walletpb.AdminRedemptionCodeView {
	t.Helper()

	requested := code
	resp, err := svc.AdminCreateRedemptionCode(context.Background(), &walletpb.AdminCreateRedemptionCodeReq{
		OperatorId: "9001",
		RequestId:  requestID,
		Type:       codeType,
		Amount:     amount,
		Code:       &requested,
	})
	if err != nil {
		t.Fatalf("创建自定义兑换码失败: %v", err)
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

func TestRedemptionCustomCodeMySQL(t *testing.T) {
	svc, _ := redemptionTestService(t)
	ctx := context.Background()

	created := createCustomCode(t, svc, "req-custom-1", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE, "Vip-2026", 500)
	if created.GetCode() != "Vip-2026" {
		t.Fatalf("自定义码被改写: %q", created.GetCode())
	}

	if _, err := redeem(svc, "7201", "Vip-2026"); err != nil {
		t.Fatalf("精确兑换失败: %v", err)
	}
	for _, wrong := range []string{"vip-2026", "Vip2026", "Vip-2026 "} {
		_, err := redeem(svc, "7202", wrong)
		if got := bizCodeOfIntegration(t, err); got != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND) {
			t.Fatalf("%q 不应命中, 错误码 = %d", wrong, got)
		}
	}

	// 同码不同 request_id：唯一索引冲突，必须明确报错而不是换成随机码。
	duplicate := "Vip-2026"
	_, err := svc.AdminCreateRedemptionCode(ctx, &walletpb.AdminCreateRedemptionCodeReq{
		OperatorId: "9001",
		RequestId:  "req-custom-2",
		Type:       walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE,
		Amount:     500,
		Code:       &duplicate,
	})
	if got := bizCodeOfIntegration(t, err); got != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_STATE_CONFLICT) {
		t.Fatalf("重复自定义码错误码 = %d", got)
	}

	// 同 request_id 同码：重放第一次的记录。
	replayed := createCustomCode(t, svc, "req-custom-1", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE, "Vip-2026", 500)
	if replayed.GetId() != created.GetId() {
		t.Fatalf("同码重放应返回同一记录: %s/%s", created.GetId(), replayed.GetId())
	}

	// 同 request_id 但明文不同：属于换了意图的重复请求。
	changed := "vip-2026"
	_, err = svc.AdminCreateRedemptionCode(ctx, &walletpb.AdminCreateRedemptionCodeReq{
		OperatorId: "9001",
		RequestId:  "req-custom-1",
		Type:       walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE,
		Amount:     500,
		Code:       &changed,
	})
	if got := bizCodeOfIntegration(t, err); got != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT) {
		t.Fatalf("同请求不同码错误码 = %d", got)
	}

	// 仅大小写不同是另一个码，可以共存并按原值兑换。
	lower := createCustomCode(t, svc, "req-custom-3", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE, "vip-2026", 500)
	if lower.GetCode() != "vip-2026" {
		t.Fatalf("小写自定义码被改写: %q", lower.GetCode())
	}
	if _, err := redeem(svc, "7203", "vip-2026"); err != nil {
		t.Fatalf("小写码精确兑换失败: %v", err)
	}
	if _, err := redeem(svc, "7204", "VIP-2026"); bizCodeOfIntegration(t, err) != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND) {
		t.Fatalf("VIP-2026 不应命中")
	}
}

// createCappedCode 通过管理员 RPC 创建指定总兑换次数的兑换码。
func createCappedCode(svc *CommerceWalletService, requestID string, codeType walletpb.RedemptionCodeType, amount, maxRedemptions int64) (*walletpb.AdminRedemptionCodeView, error) {
	resp, err := svc.AdminCreateRedemptionCode(context.Background(), &walletpb.AdminCreateRedemptionCodeReq{
		OperatorId:     "9001",
		RequestId:      requestID,
		Type:           codeType,
		Amount:         amount,
		MaxRedemptions: &maxRedemptions,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetCode(), nil
}

// TestRedemptionMaxRedemptionsMySQL 覆盖次数上限的创建、兑换、幂等与非法输入。
func TestRedemptionMaxRedemptionsMySQL(t *testing.T) {
	svc, client := redemptionTestService(t)
	ctx := context.Background()

	capped, err := createCappedCode(svc, "req-max-capped", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 300, 2)
	if err != nil {
		t.Fatalf("创建限次兑换码失败: %v", err)
	}
	if capped.GetMaxRedemptions() != 2 {
		t.Fatalf("新建码 max_redemptions = %d，期望 2", capped.GetMaxRedemptions())
	}

	if _, err := redeem(svc, "7401", capped.GetCode()); err != nil {
		t.Fatalf("第一人兑换失败: %v", err)
	}
	if _, err := redeem(svc, "7402", capped.GetCode()); err != nil {
		t.Fatalf("第二人兑换失败: %v", err)
	}
	if _, err := redeem(svc, "7403", capped.GetCode()); err == nil {
		t.Fatalf("次数用满后第三人不应兑换成功")
	} else if got := bizCodeOfIntegration(t, err); got != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND) {
		t.Fatalf("用满后错误码 = %d，期望 NOT_FOUND", got)
	}
	if account := walletAccountOf(t, client, 7403); account != nil {
		t.Fatalf("用满后第三人不应创建钱包: %+v", account)
	}

	row, err := client.RedemptionCode.Query().Where(redemptioncode.RequestID("req-max-capped")).Only(ctx)
	if err != nil {
		t.Fatalf("读取兑换码失败: %v", err)
	}
	// 用满后仍保持 ACTIVE，对外不新增状态。
	if row.Status != redemptionStatusActive || row.RedeemedCount != 2 || row.MaxRedemptions != 2 {
		t.Fatalf("用满后记录 = %s/%d/%d", row.Status, row.RedeemedCount, row.MaxRedemptions)
	}

	// 0 表示不限制：与未传该字段的既有行为一致。
	unlimited, err := createCappedCode(svc, "req-max-unlimited", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 100, 0)
	if err != nil {
		t.Fatalf("创建不限次兑换码失败: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := redeem(svc, fmt.Sprintf("%d", 7410+i), unlimited.GetCode()); err != nil {
			t.Fatalf("不限次第 %d 人兑换失败: %v", i+1, err)
		}
	}

	// 相同 request_id 与相同上限属于合法重放；上限变化属于换了意图。
	replayed, err := createCappedCode(svc, "req-max-capped", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 300, 2)
	if err != nil {
		t.Fatalf("相同上限的重放失败: %v", err)
	}
	if replayed.GetId() != capped.GetId() {
		t.Fatalf("重放返回了不同的兑换码: %s/%s", capped.GetId(), replayed.GetId())
	}
	if _, err := createCappedCode(svc, "req-max-capped", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 300, 3); err == nil {
		t.Fatalf("相同 request_id 不同上限应 idempotency conflict")
	} else if got := bizCodeOfIntegration(t, err); got != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT) {
		t.Fatalf("不同上限错误码 = %d", got)
	}

	// 上限超出存储类型或为负：参数错误。
	if _, err := createCappedCode(svc, "req-max-overflow", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 100, int64(1)<<32); bizCodeOfIntegration(t, err) != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
		t.Fatalf("超出 uint32 上限应为 INVALID_ARGUMENT")
	}
	if _, err := createCappedCode(svc, "req-max-negative", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 100, -1); bizCodeOfIntegration(t, err) != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
		t.Fatalf("负上限应为 INVALID_ARGUMENT")
	}
	// UNIQUE 码只能全局一次，设置总次数没有意义。
	if _, err := createCappedCode(svc, "req-max-unique", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE, 100, 2); bizCodeOfIntegration(t, err) != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
		t.Fatalf("UNIQUE 码带次数上限应为 INVALID_ARGUMENT")
	}
	// uint32 上界本身合法。
	boundary, err := createCappedCode(svc, "req-max-boundary", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 100, int64(1)<<32-1)
	if err != nil {
		t.Fatalf("uint32 上界应可创建: %v", err)
	}
	if boundary.GetMaxRedemptions() != int64(1)<<32-1 {
		t.Fatalf("上界码 max_redemptions = %d", boundary.GetMaxRedemptions())
	}

	// 列表的 ACTIVE 过滤仍包含已用满的码，仅展示次数。
	active := walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE
	list, err := svc.AdminListRedemptionCodes(ctx, &walletpb.AdminListRedemptionCodesReq{Status: &active, PageSize: 20})
	if err != nil {
		t.Fatalf("列出 ACTIVE 兑换码失败: %v", err)
	}
	var exhaustedSeen bool
	for _, item := range list.GetItems() {
		if item.GetId() == capped.GetId() {
			exhaustedSeen = item.GetRedeemedCount() == 2 && item.GetMaxRedemptions() == 2
		}
	}
	if !exhaustedSeen {
		t.Fatalf("已用满的码应仍出现在 ACTIVE 列表并带出次数")
	}
}

// TestRedemptionMaxRedemptionsConcurrencyMySQL 验证并发下恰好只有 N 人兑换成功。
func TestRedemptionMaxRedemptionsConcurrencyMySQL(t *testing.T) {
	svc, client := redemptionTestService(t)

	const cap = 3
	code, err := createCappedCode(svc, "req-max-race", walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED, 100, cap)
	if err != nil {
		t.Fatalf("创建限次兑换码失败: %v", err)
	}

	const workers = 10
	var wait sync.WaitGroup
	wait.Add(workers)
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		userID := fmt.Sprintf("%d", 7501+i)
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
	if success != cap {
		t.Fatalf("并发兑换成功次数 = %d，期望 %d", success, cap)
	}

	row, err := client.RedemptionCode.Query().Where(redemptioncode.RequestID("req-max-race")).Only(context.Background())
	if err != nil {
		t.Fatalf("读取兑换码失败: %v", err)
	}
	if row.RedeemedCount != cap {
		t.Fatalf("redeemed_count = %d，期望 %d", row.RedeemedCount, cap)
	}

	total := int64(0)
	for i := 0; i < workers; i++ {
		account := walletAccountOf(t, client, uint64(7501+i))
		if account == nil {
			continue
		}
		total += account.BalanceAmount
	}
	if total != cap*100 {
		t.Fatalf("并发入账总额 = %d，期望 %d", total, cap*100)
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
