//go:build integration

package wallet

import (
	"context"
	"testing"

	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
)

// 显式"重建支付链接"的 MySQL 集成测试。
//
// 这些用例覆盖三类入口：CREATE 重放（订单停在 CREATED）、用户重试、管理员重试。
// 共同约束：只有"从未记录过 Provider 对象"的订单可以重建，重建不改金额/汇率，
// 也不产生任何余额或流水。

const rebuiltCheckoutURL = "https://checkout.creem.io/chk_test_1"

// createReviewRecharge 造一笔"渠道结果未知"的 REVIEW 订单。
func createReviewRecharge(
	t *testing.T,
	svc *CommerceWalletService,
	gateway *fakeGateway,
	userID, requestID string,
	walletAmount int64,
) string {
	t.Helper()

	gateway.createFn = func(context.Context, payment.CheckoutRequest) (payment.CheckoutResult, error) {
		return payment.CheckoutResult{}, payment.ErrGatewayUnavailable
	}
	created, err := svc.CreateRecharge(context.Background(), &wallet.CreateRechargeReq{
		UserId:       userID,
		RequestId:    requestID,
		WalletAmount: walletAmount,
		Provider:     wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if err != nil {
		t.Fatalf("创建未知结果订单失败: %v", err)
	}
	if got := created.GetRecharge().GetStatus(); got != wallet.RechargeStatus_RECHARGE_STATUS_REVIEW {
		t.Fatalf("未知结果应进入 REVIEW，实际 %s", got)
	}
	if got := created.GetRecharge().GetCheckoutUrl(); got != "" {
		t.Fatalf("未知结果不应带支付链接，实际 %q", got)
	}
	return created.GetRecharge().GetId()
}

// acceptCheckout 让假网关从"未知"切换为"成功建单"。
func acceptCheckout(gateway *fakeGateway) {
	gateway.createFn = func(_ context.Context, request payment.CheckoutRequest) (payment.CheckoutResult, error) {
		return checkoutResult(request), nil
	}
}

func TestWalletRetryCheckoutRebuildsReviewMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	svc := rechargeTestService(t, gateway)
	ctx := context.Background()

	rechargeID := createReviewRecharge(t, svc, gateway, "7001", "req-retry-1", 5000)
	if gateway.createCount() != 1 {
		t.Fatalf("首次创建应调用 Provider 一次，实际 %d 次", gateway.createCount())
	}

	acceptCheckout(gateway)
	rebuilt, err := svc.RetryRechargeCheckout(ctx, &wallet.RetryRechargeCheckoutReq{
		UserId:     "7001",
		RechargeId: rechargeID,
	})
	if err != nil {
		t.Fatalf("重建支付链接失败: %v", err)
	}
	if got := rebuilt.GetRecharge().GetStatus(); got != wallet.RechargeStatus_RECHARGE_STATUS_PENDING_PAYMENT {
		t.Fatalf("重建后应 PENDING_PAYMENT，实际 %s", got)
	}
	if got := rebuilt.GetRecharge().GetCheckoutUrl(); got != rebuiltCheckoutURL {
		t.Fatalf("重建后应带回支付链接 %q，实际 %q", rebuiltCheckoutURL, got)
	}
	if gateway.createCount() != 2 {
		t.Fatalf("重建应再调用 Provider 一次，实际 %d 次", gateway.createCount())
	}
	// 重建不产生资金动作。
	assertBalance(t, svc, 7001, 0)
	assertTransactionCount(t, svc, txTypeRechargeCredit, 0)

	// 二次点击：已经有链接，直接返回现状，不再调用 Provider。
	again, err := svc.RetryRechargeCheckout(ctx, &wallet.RetryRechargeCheckoutReq{
		UserId:     "7001",
		RechargeId: rechargeID,
	})
	if err != nil {
		t.Fatalf("重复重建应幂等返回: %v", err)
	}
	if got := again.GetRecharge().GetCheckoutUrl(); got != rebuiltCheckoutURL {
		t.Fatalf("重复重建应返回同一链接，实际 %q", got)
	}
	if gateway.createCount() != 2 {
		t.Fatalf("重复重建不应再调用 Provider，实际 %d 次", gateway.createCount())
	}
}

func TestWalletRetryCheckoutRejectsSettledRechargeMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	svc := rechargeTestService(t, gateway)
	acceptCheckout(gateway)
	// 回调核对会查询 Checkout 与 Transaction，这里模拟已完成支付的订单。
	gateway.checkoutFn = func(context.Context, string) (payment.CheckoutResult, error) {
		result := checkoutResult(payment.CheckoutRequest{CustomPrice: 1389, RequestID: "ignored"})
		result.Status = payment.CheckoutStatusCompleted
		return result, nil
	}
	gateway.transactionFn = func(context.Context, string) (payment.TransactionResult, error) {
		return paidTransaction(), nil
	}
	ctx := context.Background()

	created, err := svc.CreateRecharge(ctx, &wallet.CreateRechargeReq{
		UserId:       "7001",
		RequestId:    "req-paid",
		WalletAmount: 10000,
		Provider:     wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if err != nil {
		t.Fatalf("创建充值失败: %v", err)
	}
	rechargeID := created.GetRecharge().GetId()
	if _, err := svc.ProcessPaymentWebhook(ctx, webhookRequest(t, webhookBody(), testWebhookSecret)); err != nil {
		t.Fatalf("处理回调失败: %v", err)
	}
	assertRechargeStatus(t, svc, rechargeID, rechargeStatusSucceeded)

	_, err = svc.RetryRechargeCheckout(ctx, &wallet.RetryRechargeCheckoutReq{
		UserId:     "7001",
		RechargeId: rechargeID,
	})
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_STATE_CONFLICT) {
		t.Fatalf("已支付订单重建应 STATE_CONFLICT，实际 code=%d", code)
	}
	if gateway.createCount() != 1 {
		t.Fatalf("拒绝重建时不应调用 Provider，实际 %d 次", gateway.createCount())
	}
}

func TestWalletRetryCheckoutRequiresOwnershipMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	svc := rechargeTestService(t, gateway)
	rechargeID := createReviewRecharge(t, svc, gateway, "7002", "req-owner", 5000)

	_, err := svc.RetryRechargeCheckout(context.Background(), &wallet.RetryRechargeCheckoutReq{
		UserId:     "7001",
		RechargeId: rechargeID,
	})
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND) {
		t.Fatalf("他人订单应 NOT_FOUND，实际 code=%d", code)
	}
}

func TestWalletRetryCheckoutRejectsProviderObjectMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	svc := rechargeTestService(t, gateway)
	ctx := context.Background()
	rechargeID := createReviewRecharge(t, svc, gateway, "7003", "req-provider", 5000)

	// 模拟"已经有 Provider 对象但结果未知"的 REVIEW：这类订单必须人工核对，不能重建。
	if _, err := svc.DB.Recharge.UpdateOneID(mustUint64(t, rechargeID)).
		SetProviderCheckoutID("chk_legacy").
		Save(ctx); err != nil {
		t.Fatalf("写入 Provider 对象失败: %v", err)
	}
	acceptCheckout(gateway)

	_, err := svc.RetryRechargeCheckout(ctx, &wallet.RetryRechargeCheckoutReq{
		UserId:     "7003",
		RechargeId: rechargeID,
	})
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_STATE_CONFLICT) {
		t.Fatalf("已有 Provider 对象应 STATE_CONFLICT，实际 code=%d", code)
	}
	if gateway.createCount() != 1 {
		t.Fatalf("拒绝重建时不应调用 Provider，实际 %d 次", gateway.createCount())
	}
}

func TestWalletRetryCheckoutKeepsReviewOnUnknownMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	svc := rechargeTestService(t, gateway)
	ctx := context.Background()
	rechargeID := createReviewRecharge(t, svc, gateway, "7001", "req-unknown-retry", 5000)

	// 网关仍不可用：REVIEW 保持原状，调用方拿到空链接，可以稍后再点。
	kept, err := svc.RetryRechargeCheckout(ctx, &wallet.RetryRechargeCheckoutReq{
		UserId:     "7001",
		RechargeId: rechargeID,
	})
	if err != nil {
		t.Fatalf("未知结果重试不应报错: %v", err)
	}
	if got := kept.GetRecharge().GetStatus(); got != wallet.RechargeStatus_RECHARGE_STATUS_REVIEW {
		t.Fatalf("未知结果应保持 REVIEW，实际 %s", got)
	}
	if got := kept.GetRecharge().GetCheckoutUrl(); got != "" {
		t.Fatalf("未知结果不应带链接，实际 %q", got)
	}
	if gateway.createCount() != 2 {
		t.Fatalf("重试应调用 Provider 一次，实际 %d 次", gateway.createCount())
	}
	assertBalance(t, svc, 7001, 0)
}

func TestWalletAdminRetryCheckoutRecordsOperatorMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	svc := rechargeTestService(t, gateway)
	ctx := context.Background()
	rechargeID := createReviewRecharge(t, svc, gateway, "7001", "req-admin-retry", 5000)
	acceptCheckout(gateway)

	rebuilt, err := svc.AdminRetryRechargeCheckout(ctx, &wallet.AdminRetryRechargeCheckoutReq{
		OperatorId: "9001",
		RechargeId: rechargeID,
		Reason:     "customer reports missing payment link",
	})
	if err != nil {
		t.Fatalf("管理员重建失败: %v", err)
	}
	if got := rebuilt.GetRecharge().GetUserId(); got != "7001" {
		t.Fatalf("管理员视图应带用户 ID 7001，实际 %q", got)
	}
	if got := rebuilt.GetRecharge().GetRecharge().GetStatus(); got != wallet.RechargeStatus_RECHARGE_STATUS_PENDING_PAYMENT {
		t.Fatalf("管理员视图状态应 PENDING_PAYMENT，实际 %s", got)
	}

	row, err := svc.DB.Recharge.Get(ctx, mustUint64(t, rechargeID))
	if err != nil {
		t.Fatalf("读取充值失败: %v", err)
	}
	if got := stringOrEmpty(row.ReviewedBy); got != "9001" {
		t.Fatalf("重建操作人应记录为 9001，实际 %q", got)
	}
	if row.Status != rechargeStatusPendingPayment || stringOrEmpty(row.CheckoutURL) != rebuiltCheckoutURL {
		t.Fatalf("落库字段不符: status=%s url=%q", row.Status, stringOrEmpty(row.CheckoutURL))
	}
}

func TestWalletRetryCheckoutRejectsChangedProfileMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	svc := rechargeTestService(t, gateway)
	ctx := context.Background()
	rechargeID := createReviewRecharge(t, svc, gateway, "7001", "req-profile", 5000)
	acceptCheckout(gateway)

	// 快照里换了一个商品：重建会把订单指向另一个 Product，必须拒绝。
	changed := rechargeSnapshot()
	changed.Revision = 12
	changed.TestProfile.ProductId = "prod_other_1"
	svc.Runtime = newPolicyRuntime(t, changed, fakePresence{has: true})

	_, err := svc.RetryRechargeCheckout(ctx, &wallet.RetryRechargeCheckoutReq{
		UserId:     "7001",
		RechargeId: rechargeID,
	})
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_STATE_CONFLICT) {
		t.Fatalf("Profile 换代应 STATE_CONFLICT，实际 code=%d", code)
	}
	if gateway.createCount() != 1 {
		t.Fatalf("拒绝重建时不应调用 Provider，实际 %d 次", gateway.createCount())
	}
}

func TestWalletReplayRebuildsCreatedRechargeMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	svc := rechargeTestService(t, gateway)
	acceptCheckout(gateway)
	ctx := context.Background()

	created, err := svc.CreateRecharge(ctx, &wallet.CreateRechargeReq{
		UserId:       "7001",
		RequestId:    "req-created-replay",
		WalletAmount: 5000,
		Provider:     wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if err != nil {
		t.Fatalf("创建充值失败: %v", err)
	}
	rechargeID := created.GetRecharge().GetId()

	// 模拟"插入 CREATED 后进程中断"：订单没有任何 Provider 结果。
	if _, err := svc.DB.Recharge.UpdateOneID(mustUint64(t, rechargeID)).
		SetStatus(rechargeStatusCreated).
		ClearCheckoutURL().
		ClearProviderCheckoutID().
		ClearProviderOrderID().
		ClearProviderTransactionID().
		Save(ctx); err != nil {
		t.Fatalf("重置为 CREATED 失败: %v", err)
	}

	replayed, err := svc.CreateRecharge(ctx, &wallet.CreateRechargeReq{
		UserId:       "7001",
		RequestId:    "req-created-replay",
		WalletAmount: 5000,
		Provider:     wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if err != nil {
		t.Fatalf("重放 CREATED 订单失败: %v", err)
	}
	if got := replayed.GetRecharge().GetCheckoutUrl(); got != rebuiltCheckoutURL {
		t.Fatalf("重放应重建出链接 %q，实际 %q", rebuiltCheckoutURL, got)
	}
	assertRechargeStatus(t, svc, rechargeID, rechargeStatusPendingPayment)
	if gateway.createCount() != 2 {
		t.Fatalf("重放 CREATED 应重新调用 Provider，实际 %d 次", gateway.createCount())
	}
	assertBalance(t, svc, 7001, 0)
}