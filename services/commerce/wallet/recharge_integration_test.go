//go:build integration

package wallet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/wallettransaction"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
)

// 任务 6 的 MySQL 集成测试：充值创建、回调入账、幂等重放、取消后真实付款、
// 管理员刷新与全额退款。所有 Provider 行为由内存假网关模拟。

// fakeGateway 是可编程的支付网关替身。
type fakeGateway struct {
	mu            sync.Mutex
	creates       int
	refunds       int
	refunded      bool
	createFn      func(context.Context, payment.CheckoutRequest) (payment.CheckoutResult, error)
	checkoutFn    func(context.Context, string) (payment.CheckoutResult, error)
	transactionFn func(context.Context, string) (payment.TransactionResult, error)
	refundFn      func(context.Context, string) (payment.RefundResult, error)
}

func (g *fakeGateway) CreateCheckout(ctx context.Context, request payment.CheckoutRequest) (payment.CheckoutResult, error) {
	g.mu.Lock()
	g.creates++
	g.mu.Unlock()
	if g.createFn == nil {
		return payment.CheckoutResult{}, errors.New("create checkout is not configured")
	}
	return g.createFn(ctx, request)
}

func (g *fakeGateway) GetCheckout(ctx context.Context, checkoutID string) (payment.CheckoutResult, error) {
	if g.checkoutFn == nil {
		return payment.CheckoutResult{}, errors.New("get checkout is not configured")
	}
	return g.checkoutFn(ctx, checkoutID)
}

func (g *fakeGateway) GetTransaction(ctx context.Context, transactionID string) (payment.TransactionResult, error) {
	if g.transactionFn == nil {
		return payment.TransactionResult{}, errors.New("get transaction is not configured")
	}
	return g.transactionFn(ctx, transactionID)
}

func (g *fakeGateway) CreateFullRefund(ctx context.Context, transactionID string) (payment.RefundResult, error) {
	g.mu.Lock()
	g.refunds++
	g.mu.Unlock()
	if g.refundFn == nil {
		return payment.RefundResult{}, errors.New("refund is not configured")
	}
	return g.refundFn(ctx, transactionID)
}

func (g *fakeGateway) createCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.creates
}

func (g *fakeGateway) refundCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.refunds
}

// markRefunded 模拟 Provider 侧的退款结果已可查询。
func (g *fakeGateway) markRefunded() {
	g.mu.Lock()
	g.refunded = true
	g.mu.Unlock()
}

// isRefunded 报告 Provider 侧交易是否已变为全额退款。
func (g *fakeGateway) isRefunded() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.refunded
}

// fakeFactory 总是返回同一个假网关。
type fakeFactory struct{ gateway payment.PaymentGateway }

func (f fakeFactory) NewGateway(payment.GatewayCredentials) (payment.PaymentGateway, error) {
	return f.gateway, nil
}

// fakeCatalog 返回固定的成功跳转地址。
type fakeCatalog struct{ url string }

func (c fakeCatalog) ProductSuccessURL(context.Context, payment.GatewayCredentials) (string, error) {
	return c.url, nil
}

// rechargeTestService 构造已注入假网关与固定 Profile 快照的钱包服务。
func rechargeTestService(t *testing.T, gateway payment.PaymentGateway) *CommerceWalletService {
	t.Helper()

	svc, client, rawdb := integrationWalletService(t)
	dropWalletTables(t, rawdb)
	svc.Cfg = &conf.Config{Wallet: conf.WalletConf{OwnerNamespace: testOwnerNamespace}}
	svc.DB = db.NewCommerceWalletDB(client)
	svc.Runtime = newPolicyRuntime(t, rechargeSnapshot(), fakePresence{has: true})
	svc.Factory = fakeFactory{gateway: gateway}
	svc.Catalog = fakeCatalog{url: testSuccessURL}
	if _, err := svc.InitDatabase(context.Background(), &base.Empty{}); err != nil {
		t.Fatalf("初始化钱包表失败: %v", err)
	}
	return svc
}

// rechargeSnapshot、paidTransaction、refundedTransaction、webhookBody、webhookRequest
// 由非集成构建的 recharge_fixture_test.go 提供，避免两套夹具漂移。

// checkoutResult 构造与冻结报价匹配的 Checkout 响应。
func checkoutResult(request payment.CheckoutRequest) payment.CheckoutResult {
	return payment.CheckoutResult{
		ID:          testCheckoutID,
		URL:         "https://checkout.creem.io/chk_test_1",
		Status:      payment.CheckoutStatusPending,
		RawStatus:   "pending",
		Mode:        "test",
		ProductID:   testProductID,
		Units:       1,
		CustomPrice: request.CustomPrice,
		RequestID:   request.RequestID,
		Order: &payment.ProviderOrder{
			ID:            testOrderID,
			ProductID:     testProductID,
			Currency:      currencyUSD,
			Status:        "paid",
			Amount:        request.CustomPrice,
			TransactionID: testTransactionID,
		},
	}
}

func TestWalletRechargeLifecycleMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	gateway.createFn = func(_ context.Context, request payment.CheckoutRequest) (payment.CheckoutResult, error) {
		return checkoutResult(request), nil
	}
	gateway.checkoutFn = func(context.Context, string) (payment.CheckoutResult, error) {
		result := checkoutResult(payment.CheckoutRequest{CustomPrice: 1389, RequestID: "ignored"})
		result.Status = payment.CheckoutStatusCompleted
		return result, nil
	}
	gateway.transactionFn = func(context.Context, string) (payment.TransactionResult, error) {
		if gateway.isRefunded() {
			return refundedTransaction(), nil
		}
		return paidTransaction(), nil
	}
	gateway.refundFn = func(context.Context, string) (payment.RefundResult, error) {
		gateway.markRefunded()
		return payment.RefundResult{
			ID:            "ref_test_1",
			Mode:          "test",
			Status:        payment.RefundStatusSucceeded,
			RawStatus:     "succeeded",
			TransactionID: testTransactionID,
		}, nil
	}

	svc := rechargeTestService(t, gateway)
	ctx := context.Background()

	created, err := svc.CreateRecharge(ctx, &wallet.CreateRechargeReq{
		UserId:       "7001",
		RequestId:    "req-1",
		WalletAmount: 10000,
		Provider:     wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if err != nil {
		t.Fatalf("创建充值失败: %v", err)
	}
	if created.GetRecharge().GetStatus() != wallet.RechargeStatus_RECHARGE_STATUS_PENDING_PAYMENT {
		t.Fatalf("创建后状态应 PENDING_PAYMENT，实际 %s", created.GetRecharge().GetStatus())
	}
	if got := created.GetRecharge().GetProviderAmount(); got != 1389 {
		t.Fatalf("provider_amount 应为 1389，实际 %d", got)
	}
	if got := created.GetRecharge().GetCnyPerUsdRate(); got != "7.200000" {
		t.Fatalf("汇率应可复现为 7.200000，实际 %q", got)
	}
	rechargeID := created.GetRecharge().GetId()

	// 同 request_id 完全一致的重放：返回原订单，不再调用 Provider 创建。
	replayed, err := svc.CreateRecharge(ctx, &wallet.CreateRechargeReq{
		UserId:       "7001",
		RequestId:    "req-1",
		WalletAmount: 10000,
		Provider:     wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if err != nil {
		t.Fatalf("重放创建充值失败: %v", err)
	}
	if replayed.GetRecharge().GetId() != rechargeID {
		t.Fatalf("重放应返回原订单 %s，实际 %s", rechargeID, replayed.GetRecharge().GetId())
	}
	if gateway.createCount() != 1 {
		t.Fatalf("重放不应再次创建 Checkout，实际调用 %d 次", gateway.createCount())
	}

	// 同一 request_id 改金额：幂等冲突，不创建第二笔。
	_, err = svc.CreateRecharge(ctx, &wallet.CreateRechargeReq{
		UserId:       "7001",
		RequestId:    "req-1",
		WalletAmount: 10001,
		Provider:     wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if code := bizCodeOf(t, err); code != int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT) {
		t.Fatalf("同 request_id 改金额应 IDEMPOTENCY_CONFLICT，实际 code=%d", code)
	}

	// 回调入账：RECHARGE_CREDIT 一次、余额 +10000。
	accepted, err := svc.ProcessPaymentWebhook(ctx, webhookRequest(t, webhookBody(), testWebhookSecret))
	if err != nil {
		t.Fatalf("处理回调失败: %v", err)
	}
	if !accepted.GetAccepted() {
		t.Fatalf("有效回调应被接受，实际 message=%q", accepted.GetMessage())
	}
	assertRechargeStatus(t, svc, rechargeID, rechargeStatusSucceeded)
	assertBalance(t, svc, 7001, 10000)
	assertTransactionCount(t, svc, txTypeRechargeCredit, 1)

	// 重复回调必须幂等：余额与流水都不再增加。
	if _, err := svc.ProcessPaymentWebhook(ctx, webhookRequest(t, webhookBody(), testWebhookSecret)); err != nil {
		t.Fatalf("重复回调失败: %v", err)
	}
	assertBalance(t, svc, 7001, 10000)
	assertTransactionCount(t, svc, txTypeRechargeCredit, 1)

	// 用户视图与管理员视图的隔离。
	userView, err := svc.GetRecharge(ctx, &wallet.GetRechargeReq{UserId: "7001", RechargeId: rechargeID})
	if err != nil {
		t.Fatalf("用户查询充值失败: %v", err)
	}
	if userView.GetRecharge().GetStatus() != wallet.RechargeStatus_RECHARGE_STATUS_SUCCEEDED {
		t.Fatalf("用户视图状态错误: %s", userView.GetRecharge().GetStatus())
	}
	adminView, err := svc.AdminGetRecharge(ctx, &wallet.AdminGetRechargeReq{RechargeId: rechargeID})
	if err != nil {
		t.Fatalf("管理员查询充值失败: %v", err)
	}
	if adminView.GetRecharge().GetUserId() != "7001" ||
		adminView.GetRecharge().GetProviderCheckoutId() != testCheckoutID {
		t.Fatalf("管理员视图字段缺失: %+v", adminView.GetRecharge())
	}

	// 全额退款：本地冲回、余额归零、只写一次 RECHARGE_REFUND。
	refunded, err := svc.AdminRefundRecharge(ctx, &wallet.AdminRefundRechargeReq{
		OperatorId: "9001",
		RechargeId: rechargeID,
		Reason:     "customer requested refund",
	})
	if err != nil {
		t.Fatalf("全额退款失败: %v", err)
	}
	if refunded.GetRecharge() == nil {
		t.Fatal("退款响应缺少充值视图")
	}
	assertRechargeStatus(t, svc, rechargeID, rechargeStatusRefunded)
	assertBalance(t, svc, 7001, 0)
	assertTransactionCount(t, svc, txTypeRechargeRefund, 1)

	// 重复退款保持 REFUNDED，不重复冲回。
	if _, err := svc.AdminRefundRecharge(ctx, &wallet.AdminRefundRechargeReq{
		OperatorId: "9001",
		RechargeId: rechargeID,
		Reason:     "repeat",
	}); err != nil {
		t.Fatalf("重复退款失败: %v", err)
	}
	assertBalance(t, svc, 7001, 0)
	assertTransactionCount(t, svc, txTypeRechargeRefund, 1)
}

func TestWalletRechargeUnknownCreationMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	gateway.createFn = func(context.Context, payment.CheckoutRequest) (payment.CheckoutResult, error) {
		return payment.CheckoutResult{}, payment.ErrGatewayUnavailable
	}
	svc := rechargeTestService(t, gateway)

	created, err := svc.CreateRecharge(context.Background(), &wallet.CreateRechargeReq{
		UserId:       "7002",
		RequestId:    "req-unknown",
		WalletAmount: 5000,
		Provider:     wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if err != nil {
		t.Fatalf("未知结果创建不应返回错误: %v", err)
	}
	if created.GetRecharge().GetStatus() != wallet.RechargeStatus_RECHARGE_STATUS_REVIEW {
		t.Fatalf("未知结果应进入 REVIEW，实际 %s", created.GetRecharge().GetStatus())
	}
	assertBalance(t, svc, 7002, 0)
	assertTransactionCount(t, svc, txTypeRechargeCredit, 0)
}

func TestWalletCancelThenPaidMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	gateway.createFn = func(_ context.Context, request payment.CheckoutRequest) (payment.CheckoutResult, error) {
		return checkoutResult(request), nil
	}
	gateway.checkoutFn = func(context.Context, string) (payment.CheckoutResult, error) {
		result := checkoutResult(payment.CheckoutRequest{CustomPrice: 1389, RequestID: "ignored"})
		result.Status = payment.CheckoutStatusCompleted
		return result, nil
	}
	gateway.transactionFn = func(context.Context, string) (payment.TransactionResult, error) {
		return paidTransaction(), nil
	}
	svc := rechargeTestService(t, gateway)
	ctx := context.Background()

	created, err := svc.CreateRecharge(ctx, &wallet.CreateRechargeReq{
		UserId:       "7003",
		RequestId:    "req-cancel",
		WalletAmount: 10000,
		Provider:     wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if err != nil {
		t.Fatalf("创建充值失败: %v", err)
	}
	rechargeID := created.GetRecharge().GetId()

	canceled, err := svc.CancelRecharge(ctx, &wallet.CancelRechargeReq{UserId: "7003", RechargeId: rechargeID})
	if err != nil {
		t.Fatalf("取消失败: %v", err)
	}
	if canceled.GetRecharge().GetStatus() != wallet.RechargeStatus_RECHARGE_STATUS_CANCELED {
		t.Fatalf("取消后应 CANCELED，实际 %s", canceled.GetRecharge().GetStatus())
	}

	// 取消后真实付款仍需入账，不能被本地取消掩盖。
	if _, err := svc.ProcessPaymentWebhook(ctx, webhookRequest(t, webhookBody(), testWebhookSecret)); err != nil {
		t.Fatalf("取消后回调失败: %v", err)
	}
	assertRechargeStatus(t, svc, rechargeID, rechargeStatusSucceeded)
	assertBalance(t, svc, 7003, 10000)
}

func TestWalletRechargeWebhookRejectsBadSignatureMySQL(t *testing.T) {
	svc := rechargeTestService(t, &fakeGateway{})

	resp, err := svc.ProcessPaymentWebhook(context.Background(), webhookRequest(t, webhookBody(), "wrong-secret"))
	if err != nil {
		t.Fatalf("错误签名不应返回 RPC 错误: %v", err)
	}
	if resp.GetAccepted() {
		t.Fatal("错误签名必须拒绝")
	}
	if strings.Contains(resp.GetMessage(), testWebhookSecret) {
		t.Fatalf("响应不得泄漏 Secret: %q", resp.GetMessage())
	}
}

func TestWalletAdminRefreshMySQL(t *testing.T) {
	gateway := &fakeGateway{}
	gateway.createFn = func(context.Context, payment.CheckoutRequest) (payment.CheckoutResult, error) {
		return payment.CheckoutResult{}, payment.ErrGatewayUnavailable
	}
	svc := rechargeTestService(t, gateway)
	ctx := context.Background()

	// 未知创建进入 REVIEW；管理员确认 Provider 侧没有对象后才可标 FAILED。
	unknown, err := svc.CreateRecharge(ctx, &wallet.CreateRechargeReq{
		UserId:       "7004",
		RequestId:    "req-refresh-unknown",
		WalletAmount: 5000,
		Provider:     wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if err != nil {
		t.Fatalf("未知创建不应返回错误: %v", err)
	}
	if unknown.GetRecharge().GetStatus() != wallet.RechargeStatus_RECHARGE_STATUS_REVIEW {
		t.Fatalf("未知创建应 REVIEW，实际 %s", unknown.GetRecharge().GetStatus())
	}
	refreshed, err := svc.AdminRefreshRecharge(ctx, &wallet.AdminRefreshRechargeReq{
		OperatorId:                "9001",
		RechargeId:                unknown.GetRecharge().GetId(),
		Reason:                    "checked provider dashboard",
		ConfirmProviderNotCreated: true,
	})
	if err != nil {
		t.Fatalf("确认未创建失败: %v", err)
	}
	if refreshed.GetRecharge().GetRecharge().GetStatus() != wallet.RechargeStatus_RECHARGE_STATUS_FAILED {
		t.Fatalf("确认未创建后应 FAILED，实际 %s", refreshed.GetRecharge().GetRecharge().GetStatus())
	}

	// 有 Provider 事实时，刷新必须收敛到 SUCCEEDED 并入账。
	gateway.createFn = func(_ context.Context, request payment.CheckoutRequest) (payment.CheckoutResult, error) {
		return checkoutResult(request), nil
	}
	gateway.checkoutFn = func(context.Context, string) (payment.CheckoutResult, error) {
		result := checkoutResult(payment.CheckoutRequest{CustomPrice: 1389, RequestID: "ignored"})
		result.Status = payment.CheckoutStatusCompleted
		return result, nil
	}
	gateway.transactionFn = func(context.Context, string) (payment.TransactionResult, error) {
		return paidTransaction(), nil
	}
	pending, err := svc.CreateRecharge(ctx, &wallet.CreateRechargeReq{
		UserId:       "7005",
		RequestId:    "req-refresh-paid",
		WalletAmount: 10000,
		Provider:     wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
	})
	if err != nil {
		t.Fatalf("创建第二笔充值失败: %v", err)
	}
	converged, err := svc.AdminRefreshRecharge(ctx, &wallet.AdminRefreshRechargeReq{
		OperatorId: "9001",
		RechargeId: pending.GetRecharge().GetId(),
		Reason:     "provider shows a paid transaction",
	})
	if err != nil {
		t.Fatalf("刷新已支付订单失败: %v", err)
	}
	if converged.GetRecharge().GetRecharge().GetStatus() != wallet.RechargeStatus_RECHARGE_STATUS_SUCCEEDED {
		t.Fatalf("刷新后应 SUCCEEDED，实际 %s", converged.GetRecharge().GetRecharge().GetStatus())
	}
	assertBalance(t, svc, 7005, 10000)
	assertTransactionCount(t, svc, txTypeRechargeCredit, 1)
}

// assertRechargeStatus 断言订单当前状态。
func assertRechargeStatus(t *testing.T, svc *CommerceWalletService, rawID, want string) {
	t.Helper()
	recharge, err := svc.DB.Recharge.Get(context.Background(), mustUint64(t, rawID))
	if err != nil {
		t.Fatalf("读取充值失败: %v", err)
	}
	if recharge.Status != want {
		t.Fatalf("充值状态应为 %s，实际 %s", want, recharge.Status)
	}
}

// assertBalance 断言钱包余额。
func assertBalance(t *testing.T, svc *CommerceWalletService, userID, want int64) {
	t.Helper()
	account, err := svc.DB.Account.Query().Where().All(context.Background())
	if err != nil {
		t.Fatalf("读取钱包失败: %v", err)
	}
	for _, item := range account {
		if item.UserID == uint64(userID) {
			if item.BalanceAmount != want {
				t.Fatalf("用户 %d 余额应为 %d，实际 %d", userID, want, item.BalanceAmount)
			}
			return
		}
	}
	if want != 0 {
		t.Fatalf("用户 %d 没有钱包记录，期望余额 %d", userID, want)
	}
}

// assertTransactionCount 断言指定类型的流水条数。
func assertTransactionCount(t *testing.T, svc *CommerceWalletService, txType string, want int) {
	t.Helper()
	count, err := svc.DB.Transaction.Query().Where(wallettransaction.Type(txType)).Count(context.Background())
	if err != nil {
		t.Fatalf("统计流水失败: %v", err)
	}
	if count != want {
		t.Fatalf("流水 %s 条数应为 %d，实际 %d", txType, want, count)
	}
}

// mustUint64 解析十进制 ID。
func mustUint64(t *testing.T, raw string) uint64 {
	t.Helper()
	var value uint64
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil {
		t.Fatalf("非法 ID %q: %v", raw, err)
	}
	return value
}

// fingerprint 仅用于断言错误信息不泄漏敏感值。
func fingerprint(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

var _ = ent.IsNotFound
