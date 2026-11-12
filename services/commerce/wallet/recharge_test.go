package wallet

import (
	"errors"
	"strings"
	"testing"

	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	settings "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
	"github.com/cloudwego/kitex/pkg/kerrors"
)

// fixedRecharge 构造一笔处于 SUCCEEDED 的冻结报价订单。
func fixedRecharge() *ent.WalletRecharge {
	checkoutID := testCheckoutID
	checkoutURL := "https://checkout.creem.io/" + testCheckoutID
	orderID := testOrderID
	transactionID := testTransactionID
	paidAt := testNow
	return &ent.WalletRecharge{
		ID:                     42,
		CreatedAt:              testNow,
		UpdatedAt:              testNow,
		WalletID:               7,
		RequestID:              "req-1",
		MerchantOrderNo:        "cw0123456789abcdef01234567",
		Provider:               providerNameCreem,
		ProviderEnvironment:    providerEnvironmentTest,
		PaymentProfileID:       testProfileID,
		PaymentProfileRevision: 3,
		ProviderProductID:      testProductID,
		ProviderCheckoutID:     &checkoutID,
		ProviderOrderID:        &orderID,
		ProviderTransactionID:  &transactionID,
		CheckoutURL:            &checkoutURL,
		WalletAmount:           10000,
		WalletCurrency:         currencyCNY,
		ProviderAmount:         1389,
		ProviderCurrency:       currencyUSD,
		FxRateNumerator:        36,
		FxRateDenominator:      5,
		FxSettingRevision:      11,
		FxQuotedAt:             testNow,
		FxRoundingMode:         fxRoundingMode,
		Status:                 rechargeStatusSucceeded,
		PaidAt:                 &paidAt,
		Revision:               2,
	}
}

func TestRechargeStatusProtoRoundTrip(t *testing.T) {
	names := []string{
		rechargeStatusCreated,
		rechargeStatusPendingPayment,
		rechargeStatusSucceeded,
		rechargeStatusCanceled,
		rechargeStatusFailed,
		rechargeStatusReview,
		rechargeStatusRefunding,
		rechargeStatusRefunded,
	}
	for _, name := range names {
		proto := rechargeStatusProto(name)
		if proto == wallet.RechargeStatus_RECHARGE_STATUS_UNSPECIFIED {
			t.Fatalf("状态 %q 没有对应的协议枚举", name)
		}
		back, err := rechargeStatusName(proto)
		if err != nil {
			t.Fatalf("状态 %q 往返失败: %v", name, err)
		}
		if back != name {
			t.Fatalf("状态往返不一致: %q -> %q", name, back)
		}
	}
	if rechargeStatusProto("SOMETHING_NEW") != wallet.RechargeStatus_RECHARGE_STATUS_UNSPECIFIED {
		t.Fatal("未知状态不得映射为任何具体枚举")
	}
	if _, err := rechargeStatusName(wallet.RechargeStatus_RECHARGE_STATUS_UNSPECIFIED); err == nil {
		t.Fatal("零值协议状态必须报错，不能当作 CREATED")
	}
}

func TestProviderCodeRoundTrip(t *testing.T) {
	code, err := providerCode(wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM)
	if err != nil {
		t.Fatalf("CREEM 应可编码: %v", err)
	}
	if code != providerNameCreem {
		t.Fatalf("编码应为 %q，实际 %q", providerNameCreem, code)
	}
	if providerFromCode(code) != wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM {
		t.Fatal("provider 往返失败")
	}
	if _, err := providerCode(wallet.PaymentProvider_PAYMENT_PROVIDER_UNSPECIFIED); err == nil {
		t.Fatal("未指定 provider 必须报错")
	}
	if providerFromCode("STRIPE") != wallet.PaymentProvider_PAYMENT_PROVIDER_UNSPECIFIED {
		t.Fatal("未知 provider 应返回未指定")
	}
}

func TestProviderEnvironmentMapping(t *testing.T) {
	env, err := providerEnvironmentFromProfile(providerEnvironmentTest)
	if err != nil || env != wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST {
		t.Fatalf("TEST 映射失败: %v %v", env, err)
	}
	env, err = providerEnvironmentFromProfile(providerEnvironmentProduction)
	if err != nil || env != wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_PRODUCTION {
		t.Fatalf("PRODUCTION 映射失败: %v %v", env, err)
	}
	if _, err := providerEnvironmentFromProfile("test"); err == nil {
		t.Fatal("小写环境必须拒绝，避免大小写漂移")
	}
	if _, err := providerEnvironmentFromProfile(""); err == nil {
		t.Fatal("空环境必须拒绝")
	}
	if providerEnvironmentProto(providerEnvironmentTest) != wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST {
		t.Fatal("TEST 反向映射失败")
	}
}

func TestRechargeProfileSelection(t *testing.T) {
	svc := &CommerceWalletService{}

	profile, env, err := svc.rechargeProfile(rechargeSnapshot())
	if err != nil {
		t.Fatalf("TEST 模式应选中 TestProfile: %v", err)
	}
	if profile.GetProfileId() != testProfileID ||
		env != wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST {
		t.Fatalf("选中了错误的 Profile: %s %v", profile.GetProfileId(), env)
	}

	production := rechargeSnapshot()
	production.RechargeMode = rechargeModeProduction
	if _, _, err := svc.rechargeProfile(production); err == nil {
		t.Fatal("缺少 Production Profile 时必须拒绝，不能回落到测试凭据")
	}

	disabled := rechargeSnapshot()
	disabled.RechargeMode = rechargeModeDisabled
	if _, _, err := svc.rechargeProfile(disabled); err == nil {
		t.Fatal("关闭充值模式时必须拒绝")
	}

	broken := rechargeSnapshot()
	broken.TestProfile.Environment = "test"
	if _, _, err := svc.rechargeProfile(broken); err == nil {
		t.Fatal("Profile 环境非法时必须拒绝")
	}

	incomplete := rechargeSnapshot()
	incomplete.TestProfile.ApiKey = ""
	if _, _, err := svc.rechargeProfile(incomplete); err == nil {
		t.Fatal("Profile 缺少 API Key 时必须拒绝")
	}
}

func TestSnapshotRatePrefersFrozenFraction(t *testing.T) {
	rate, err := snapshotRate(rechargeSnapshot())
	if err != nil {
		t.Fatalf("解析快照汇率失败: %v", err)
	}
	if rate.Canonical != "7.200000" || rate.Numerator != 36 || rate.Denominator != 5 {
		t.Fatalf("冻结汇率错误: %+v", rate)
	}
	amount, err := QuoteProviderAmount(10000, rate)
	if err != nil || amount != 1389 {
		t.Fatalf("10000 分 @7.2 应为 1389 美分，实际 %d err=%v", amount, err)
	}

	fallback := &settings.WalletRuntimeSettings{CnyPerUsdRate: "7.2"}
	rate, err = snapshotRate(fallback)
	if err != nil {
		t.Fatalf("回退解析十进制汇率失败: %v", err)
	}
	if rate.Canonical != "7.200000" {
		t.Fatalf("回退汇率规范化错误: %q", rate.Canonical)
	}

	if _, err := snapshotRate(&settings.WalletRuntimeSettings{CnyPerUsdRate: "abc"}); err == nil {
		t.Fatal("非法汇率必须拒绝")
	}
	if _, err := snapshotRate(&settings.WalletRuntimeSettings{CnyPerUsdNumerator: 1, CnyPerUsdDenominator: 0}); err == nil {
		t.Fatal("分母为零必须拒绝")
	}
}

func TestRechargeViewMasksInternalFields(t *testing.T) {
	recharge := fixedRecharge()
	recharge.ReviewReason = stringPointer("provider returned an unknown status")
	recharge.RefundReason = stringPointer("customer requested")

	view := rechargeViewOf(recharge)
	if view.GetId() != "42" || view.GetStatus() != wallet.RechargeStatus_RECHARGE_STATUS_SUCCEEDED {
		t.Fatalf("用户视图基础字段错误: %+v", view)
	}
	if view.GetWalletAmount() != 10000 || view.GetProviderAmount() != 1389 {
		t.Fatal("用户视图金额必须与冻结报价一致")
	}
	if view.GetCnyPerUsdRate() != "7.200000" {
		t.Fatalf("用户视图汇率应可复现，实际 %q", view.GetCnyPerUsdRate())
	}
	if view.GetCheckoutUrl() == "" {
		t.Fatal("用户视图应保留 Checkout URL")
	}
	if view.GetPaidAt() != testNow.UnixMilli() {
		t.Fatalf("支付时间错误: %d", view.GetPaidAt())
	}
	// 用户视图不得暴露 Provider 内部 ID 或审核/退款细节；这些字段只存在于管理员视图。
	if _, ok := any(view).(*wallet.AdminRechargeView); ok {
		t.Fatal("用户视图不能是管理员视图")
	}

	admin := adminRechargeViewOf(recharge, 7001)
	if admin.GetUserId() != "7001" || admin.GetWalletId() != "7" {
		t.Fatalf("管理员视图归属错误: %+v", admin)
	}
	if admin.GetProviderCheckoutId() != testCheckoutID ||
		admin.GetProviderTransactionId() != testTransactionID {
		t.Fatal("管理员视图应暴露 Provider ID")
	}
	if admin.GetFxRateNumerator() != 36 || admin.GetFxRateDenominator() != 5 {
		t.Fatal("管理员视图应暴露冻结汇率分子分母")
	}
	if admin.GetReviewReason() == "" || admin.GetRefundReason() == "" {
		t.Fatal("管理员视图应暴露审核与退款原因")
	}
}

func TestValidateObservationAmount(t *testing.T) {
	recharge := fixedRecharge()
	amount := int64(1389)
	paid := int64(1389)
	zero := int64(0)
	discount := int64(100)

	cases := []struct {
		name    string
		tx      *payment.TransactionResult
		wantErr bool
	}{
		{"nil transaction is allowed", nil, false},
		{"matching payment", &payment.TransactionResult{Currency: currencyUSD, Amount: amount, AmountPaid: &paid}, false},
		{"wrong currency", &payment.TransactionResult{Currency: "EUR", Amount: amount, AmountPaid: &paid}, true},
		{"wrong amount", &payment.TransactionResult{Currency: currencyUSD, Amount: 1390, AmountPaid: &paid}, true},
		{"wrong paid amount", &payment.TransactionResult{Currency: currencyUSD, Amount: amount, AmountPaid: &zero}, true},
		{"discount rejected", &payment.TransactionResult{
			Currency: currencyUSD, Amount: amount, AmountPaid: &paid, DiscountAmount: &discount,
		}, true},
	}
	for _, tc := range cases {
		err := validateObservationAmount(recharge, tc.tx)
		if tc.wantErr && err == nil {
			t.Fatalf("%s: 期望报错，实际通过", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("%s: 不应报错: %v", tc.name, err)
		}
	}
}

func TestFullRefundSettled(t *testing.T) {
	frozen := int64(1389)
	partial := int64(689)
	refunded := int64(1389)
	paid := int64(1389)

	if fullRefundSettled(nil, frozen) {
		t.Fatal("nil 交易不能视为已退款")
	}
	full := &payment.TransactionResult{
		Status:         payment.TransactionStatusRefunded,
		Currency:       currencyUSD,
		Amount:         frozen,
		AmountPaid:     &paid,
		RefundedAmount: &refunded,
	}
	if !fullRefundSettled(full, frozen) {
		t.Fatal("全额退款事实应判定为已结算")
	}
	partialRefund := *full
	partialRefund.RefundedAmount = &partial
	if fullRefundSettled(&partialRefund, frozen) {
		t.Fatal("部分退款不能视为全额退款")
	}
	missing := *full
	missing.RefundedAmount = nil
	if fullRefundSettled(&missing, frozen) {
		t.Fatal("缺少 refunded_amount 不能视为已退款")
	}
	otherAmount := *full
	otherAmount.Amount = 100
	if fullRefundSettled(&otherAmount, frozen) {
		t.Fatal("金额不符不能视为已退款")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("短字符串不应被截断: %q", got)
	}
	if got := truncate("0123456789", 4); got != "0123" {
		t.Fatalf("应保留前 4 个字符: %q", got)
	}
	if got := truncate("abc", 0); got != "" {
		t.Fatalf("上限为零应返回空串: %q", got)
	}
}

func TestNewMerchantOrderNo(t *testing.T) {
	first := newMerchantOrderNo()
	second := newMerchantOrderNo()
	if !strings.HasPrefix(first, "cw") || len(first) != 26 {
		t.Fatalf("商户单号格式错误: %q", first)
	}
	if first == second {
		t.Fatalf("商户单号必须唯一: %q", first)
	}
}

func TestOrderAndTransactionIDOf(t *testing.T) {
	checkout := payment.CheckoutResult{
		Order: &payment.ProviderOrder{ID: testOrderID, TransactionID: testTransactionID},
	}
	if orderIDOf(checkout) != testOrderID {
		t.Fatalf("订单 ID 提取错误: %q", orderIDOf(checkout))
	}
	if transactionIDOf(checkout) != testTransactionID {
		t.Fatalf("交易 ID 提取错误: %q", transactionIDOf(checkout))
	}
	empty := payment.CheckoutResult{}
	if orderIDOf(empty) != "" || transactionIDOf(empty) != "" {
		t.Fatal("缺少订单时不应编造 Provider ID")
	}
}

func TestRechargeRateStringRejectsBrokenFractions(t *testing.T) {
	recharge := fixedRecharge()
	if got := rechargeRateString(recharge); got != "7.200000" {
		t.Fatalf("正常汇率应可复现，实际 %q", got)
	}
	recharge.FxRateDenominator = 0
	if got := rechargeRateString(recharge); got != "" {
		t.Fatalf("异常汇率必须返回空串，实际 %q", got)
	}
}

func TestRechargeErrorsAreBizErrors(t *testing.T) {
	for name, err := range map[string]error{
		"invalid":        invalidArgument("bad"),
		"state conflict": stateConflict("conflict"),
		"unavailable":    walletUnavailable("unavailable"),
		"not found":      walletNotFound("missing"),
		"feature off":    featureDisabled("off"),
	} {
		var biz kerrors.BizStatusErrorIface
		if !errors.As(err, &biz) {
			t.Fatalf("%s 必须是 BizStatusError，实际 %T", name, err)
		}
		if biz.BizStatusCode() == 0 {
			t.Fatalf("%s 的业务码不能为 0", name)
		}
	}
}
