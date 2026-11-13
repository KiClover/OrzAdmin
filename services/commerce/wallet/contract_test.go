package wallet

import (
	"reflect"
	"sort"
	"testing"

	walletpb "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// 这组测试冻结 doc/plan/WalletDesign.MD §11.1 的业务契约：
// 恰好 21 个 RPC、金额一律整数最小单位、对外 ID 一律字符串、枚举零值保留给 UNSPECIFIED。
// 任何后续任务改为增删 RPC 或改动金额/ID 类型，都必须先改设计与本测试。

// expectedRPCs 是 §11.1 允许的全部业务 RPC（顺序无关）。
var expectedRPCs = []string{
	// 用户类
	"GetWallet",
	"ListTransactions",
	"CreateRecharge",
	"GetRecharge",
	"ListRecharges",
	"CancelRecharge",
	"RedeemCode",
	// 可信内部类
	"DebitBalance",
	"RefundBalance",
	"ProcessPaymentWebhook",
	// 管理员类
	"AdminGetWallet",
	"AdminListWallets",
	"AdminListTransactions",
	"AdminGetRecharge",
	"AdminListRecharges",
	"AdminRefreshRecharge",
	"AdminRefundRecharge",
	"AdminCreateRedemptionCode",
	"AdminListRedemptionCodes",
	"AdminSetRedemptionCodeStatus",
	"AdminAdjustBalance",
}

// expectedMaintenanceRPCs 是 §11.1 允许的维护类 RPC（不计入业务 RPC）：
// 只同步钱包自己的四张表，不提供任何资金能力。
var expectedMaintenanceRPCs = []string{
	"InitDatabase",
}

func TestCommerceWalletServiceHasExactlyTwentyOneBusinessRPCsAndOneMaintenanceRPC(t *testing.T) {
	iface := reflect.TypeOf((*walletpb.CommerceWalletService)(nil)).Elem()

	maintenance := make(map[string]struct{}, len(expectedMaintenanceRPCs))
	for _, name := range expectedMaintenanceRPCs {
		maintenance[name] = struct{}{}
	}

	business := make([]string, 0, iface.NumMethod())
	gotMaintenance := make([]string, 0, len(expectedMaintenanceRPCs))
	for i := 0; i < iface.NumMethod(); i++ {
		name := iface.Method(i).Name
		if _, ok := maintenance[name]; ok {
			gotMaintenance = append(gotMaintenance, name)
			continue
		}
		business = append(business, name)
	}
	sort.Strings(business)
	sort.Strings(gotMaintenance)

	if len(business) != len(expectedRPCs) {
		t.Fatalf("钱包业务 RPC 必须恰好 %d 个，实际 %d 个: %v", len(expectedRPCs), len(business), business)
	}

	want := append([]string(nil), expectedRPCs...)
	sort.Strings(want)

	if !reflect.DeepEqual(business, want) {
		t.Fatalf("commerce wallet 业务 RPC 集合与设计不一致\n got: %v\nwant: %v", business, want)
	}

	wantMaintenance := append([]string(nil), expectedMaintenanceRPCs...)
	sort.Strings(wantMaintenance)
	if !reflect.DeepEqual(gotMaintenance, wantMaintenance) {
		t.Fatalf("commerce wallet 维护类 RPC 集合与设计不一致\n got: %v\nwant: %v", gotMaintenance, wantMaintenance)
	}

	if iface.NumMethod() != len(expectedRPCs)+len(expectedMaintenanceRPCs) {
		t.Fatalf("钱包 RPC 总数必须是 %d 个业务 + %d 个维护，实际 %d 个",
			len(expectedRPCs), len(expectedMaintenanceRPCs), iface.NumMethod())
	}
}

// TestForbiddenRPCAbsent 防回归：设计禁止的占位式能力（预扣、预览、充值预留）不得出现。
func TestForbiddenRPCAbsent(t *testing.T) {
	iface := reflect.TypeOf((*walletpb.CommerceWalletService)(nil)).Elem()

	forbidden := []string{"TopUp", "Preview", "Reservation", "Reserve", "Capture"}
	for i := 0; i < iface.NumMethod(); i++ {
		name := iface.Method(i).Name
		for _, bad := range forbidden {
			if name == bad || len(name) >= len(bad) && name[:len(bad)] == bad {
				t.Fatalf("出现设计禁止的 RPC: %s", name)
			}
		}
	}
}

func TestWalletEnumsKeepZeroValueReserved(t *testing.T) {
	cases := []struct {
		name string
		zero int32
	}{
		{"WalletStatus", int32(walletpb.WalletStatus_WALLET_STATUS_UNSPECIFIED)},
		{"TransactionType", int32(walletpb.TransactionType_TRANSACTION_TYPE_UNSPECIFIED)},
		{"RechargeStatus", int32(walletpb.RechargeStatus_RECHARGE_STATUS_UNSPECIFIED)},
		{"RedemptionCodeType", int32(walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNSPECIFIED)},
		{"RedemptionCodeStatus", int32(walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_UNSPECIFIED)},
		{"PaymentProvider", int32(walletpb.PaymentProvider_PAYMENT_PROVIDER_UNSPECIFIED)},
		{"ProviderEnvironment", int32(walletpb.ProviderEnvironment_PROVIDER_ENVIRONMENT_UNSPECIFIED)},
		{"WalletErrorCode", int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_UNSPECIFIED)},
	}
	for _, tc := range cases {
		if tc.zero != 0 {
			t.Fatalf("%s 的 UNSPECIFIED 必须为 0，实际 %d", tc.name, tc.zero)
		}
	}

	// 关键业务码必须与处理语义一一对应，后续任务直接按名引用。
	var _ walletpb.WalletErrorCode = walletpb.WalletErrorCode_WALLET_ERROR_CODE_STATE_CONFLICT
	var _ walletpb.WalletErrorCode = walletpb.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT
	var _ walletpb.WalletErrorCode = walletpb.WalletErrorCode_WALLET_ERROR_CODE_INSUFFICIENT_BALANCE
	var _ walletpb.WalletErrorCode = walletpb.WalletErrorCode_WALLET_ERROR_CODE_BALANCE_BLOCKED
	var _ walletpb.WalletErrorCode = walletpb.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED
	var _ walletpb.WalletErrorCode = walletpb.WalletErrorCode_WALLET_ERROR_CODE_RATE_LIMITED
	var _ walletpb.WalletErrorCode = walletpb.WalletErrorCode_WALLET_ERROR_CODE_NOT_IMPLEMENTED
}

// TestAdminAdjustmentEnumNumber 冻结 ADMIN_ADJUSTMENT 的枚举编号：数据库 CHECK、服务端与
// 调用方都按编号 6 识别，重新编号会让历史流水的类型读错。
func TestAdminAdjustmentEnumNumber(t *testing.T) {
	if int32(walletpb.TransactionType_TRANSACTION_TYPE_ADMIN_ADJUSTMENT) != 6 {
		t.Fatal("ADMIN_ADJUSTMENT must use enum number 6")
	}
}

// TestAdminAdjustmentWireContract 冻结管理员调账的内部 RPC 形状：
// operator/user/request/reason 一律字符串，delta_amount 是有符号 int64 分，
// owner_namespace 与 actor 由服务端派生，响应只带不可变流水。
func TestAdminAdjustmentWireContract(t *testing.T) {
	req := reflect.TypeOf(walletpb.AdminAdjustBalanceReq{})
	strType := reflect.TypeOf("")
	for _, name := range []string{"OperatorId", "UserId", "RequestId", "Reason"} {
		field, ok := req.FieldByName(name)
		if !ok {
			t.Fatalf("AdminAdjustBalanceReq 缺少字段 %s", name)
		}
		if field.Type != strType {
			t.Fatalf("AdminAdjustBalanceReq.%s 必须是字符串，实际 %s", name, field.Type)
		}
	}
	delta, ok := req.FieldByName("DeltaAmount")
	if !ok || delta.Type != reflect.TypeOf(int64(0)) {
		t.Fatalf("AdminAdjustBalanceReq.DeltaAmount 必须是 int64 分")
	}
	if _, ok := req.FieldByName("OwnerNamespace"); ok {
		t.Fatalf("AdminAdjustBalanceReq 不得包含 OwnerNamespace，owner_namespace 必须由服务端配置固定")
	}

	resp := reflect.TypeOf(walletpb.AdminAdjustBalanceResp{})
	if resp.NumField() != 1 {
		t.Fatalf("AdminAdjustBalanceResp 只能包含 transaction 一个字段，实际 %d 个", resp.NumField())
	}
	transaction, ok := resp.FieldByName("Transaction")
	if !ok || transaction.Type != reflect.TypeOf((*walletpb.AdminTransactionView)(nil)) {
		t.Fatalf("AdminAdjustBalanceResp.Transaction 必须是 AdminTransactionView")
	}
}

// TestMoneyFieldsUseIntegerMinorUnits 冻结 §5.1：钱包金额为 int64 CNY 分，
// Provider 金额为 int64 USD 美分，汇率是十进制字符串，全链路禁止浮点。
func TestMoneyFieldsUseIntegerMinorUnits(t *testing.T) {
	int64Type := reflect.TypeOf(int64(0))
	strType := reflect.TypeOf("")

	cases := []struct {
		typ    reflect.Type
		fields map[string]reflect.Type
	}{
		{
			typ: reflect.TypeOf(walletpb.WalletView{}),
			fields: map[string]reflect.Type{
				"Currency":      strType,
				"BalanceAmount": int64Type,
			},
		},
		{
			typ: reflect.TypeOf(walletpb.TransactionView{}),
			fields: map[string]reflect.Type{
				"Id":           strType,
				"Type":         reflect.TypeOf(walletpb.TransactionType(0)),
				"Currency":     strType,
				"DeltaAmount":  int64Type,
				"BalanceAfter": int64Type,
			},
		},
		{
			typ: reflect.TypeOf(walletpb.RechargeView{}),
			fields: map[string]reflect.Type{
				"Id":               strType,
				"WalletAmount":     int64Type,
				"WalletCurrency":   strType,
				"ProviderAmount":   int64Type,
				"ProviderCurrency": strType,
				"CnyPerUsdRate":    strType,
			},
		},
		{
			typ: reflect.TypeOf(walletpb.AdminRechargeView{}),
			fields: map[string]reflect.Type{
				"Recharge":          reflect.TypeOf((*walletpb.RechargeView)(nil)),
				"FxRateNumerator":   int64Type,
				"FxRateDenominator": int64Type,
				"FxSettingRevision": int64Type,
				"FxRoundingMode":    strType,
			},
		},
		{
			typ: reflect.TypeOf(walletpb.AdminRedemptionCodeView{}),
			fields: map[string]reflect.Type{
				"Id":       strType,
				"Code":     strType,
				"Amount":   int64Type,
				"Currency": strType,
			},
		},
	}

	for _, tc := range cases {
		for name, wantType := range tc.fields {
			field, ok := tc.typ.FieldByName(name)
			if !ok {
				t.Fatalf("%s 缺少字段 %s", tc.typ.Name(), name)
			}
			if field.Type != wantType {
				t.Fatalf("%s.%s 类型应为 %s，实际 %s", tc.typ.Name(), name, wantType, field.Type)
			}
		}

		// 金额与汇率禁止使用浮点。
		for i := 0; i < tc.typ.NumField(); i++ {
			kind := tc.typ.Field(i).Type.Kind()
			if kind == reflect.Float32 || kind == reflect.Float64 {
				t.Fatalf("%s.%s 使用了浮点类型，违反金额/汇率禁止浮点约束", tc.typ.Name(), tc.typ.Field(i).Name)
			}
		}
	}
}

// TestExternalIDsAreStrings 冻结 §5.3：所有对外 ID 一律十进制字符串。
func TestExternalIDsAreStrings(t *testing.T) {
	strType := reflect.TypeOf("")

	cases := []struct {
		typ    reflect.Type
		fields []string
	}{
		{reflect.TypeOf(walletpb.TransactionView{}), []string{"Id"}},
		{reflect.TypeOf(walletpb.RechargeView{}), []string{"Id"}},
		{reflect.TypeOf(walletpb.AdminWalletView{}), []string{"UserId", "WalletId"}},
		{reflect.TypeOf(walletpb.AdminTransactionView{}), []string{"WalletId", "UserId", "RelatedTransactionId"}},
		{reflect.TypeOf(walletpb.AdminRechargeView{}), []string{"UserId", "WalletId", "RequestId", "MerchantOrderNo"}},
		{reflect.TypeOf(walletpb.AdminRedemptionCodeView{}), []string{"Id", "ClaimedByUserId", "CreatedBy"}},
		{reflect.TypeOf(walletpb.GetWalletReq{}), []string{"UserId"}},
		{reflect.TypeOf(walletpb.GetRechargeReq{}), []string{"UserId", "RechargeId"}},
		{reflect.TypeOf(walletpb.CancelRechargeReq{}), []string{"UserId", "RechargeId"}},
		{reflect.TypeOf(walletpb.RedeemCodeReq{}), []string{"UserId"}},
		{reflect.TypeOf(walletpb.DebitBalanceReq{}), []string{"UserId", "BusinessType", "BusinessId"}},
		{reflect.TypeOf(walletpb.RefundBalanceReq{}), []string{"RelatedTransactionId"}},
		{reflect.TypeOf(walletpb.AdminAdjustBalanceReq{}), []string{"OperatorId", "UserId", "RequestId"}},
	}

	for _, tc := range cases {
		for _, name := range tc.fields {
			field, ok := tc.typ.FieldByName(name)
			if !ok {
				t.Fatalf("%s 缺少字段 %s", tc.typ.Name(), name)
			}
			if field.Type != strType {
				t.Fatalf("%s.%s 必须是对外 ID 字符串，实际 %s", tc.typ.Name(), name, field.Type)
			}
		}
	}
}

// TestTrustedDebitNeverAcceptsNegativeOrOwnerNamespace 冻结 §7.1：
// DebitBalance 只收正数金额，owner_namespace 由服务端配置固定，请求中不得出现。
func TestTrustedDebitNeverAcceptsOwnerNamespace(t *testing.T) {
	debit := reflect.TypeOf(walletpb.DebitBalanceReq{})
	if _, ok := debit.FieldByName("OwnerNamespace"); ok {
		t.Fatalf("DebitBalanceReq 不得包含 OwnerNamespace，owner_namespace 必须由服务端配置固定")
	}
	amount, ok := debit.FieldByName("Amount")
	if !ok || amount.Type != reflect.TypeOf(int64(0)) {
		t.Fatalf("DebitBalanceReq.Amount 必须是 int64 分")
	}
	currency, ok := debit.FieldByName("Currency")
	if !ok || currency.Type != reflect.TypeOf("") {
		t.Fatalf("DebitBalanceReq.Currency 必须是字符串")
	}

	// RefundBalance 以原交易 ID 为唯一金额来源，不得携带金额。
	refund := reflect.TypeOf(walletpb.RefundBalanceReq{})
	if _, ok := refund.FieldByName("Amount"); ok {
		t.Fatalf("RefundBalanceReq 不得包含 Amount，退款金额必须从原流水派生")
	}
}

// TestUserViewsExposeNoIdentity 冻结 §11.6 视图隔离：
// 用户视图不得出现 wallet_id / user_id / request_id / operator_id / Provider 内部标识。
func TestUserViewsExposeNoIdentity(t *testing.T) {
	forbidden := []string{
		"WalletId", "UserId", "OperatorId", "RequestId",
		"MerchantOrderNo", "PaymentProfileId", "ProviderProductId",
		"ProviderCheckoutId", "ProviderOrderId", "ProviderTransactionId",
		"ReviewReason", "ReviewBlocksDebit", "ReviewedBy", "ReviewedAt",
		"RefundReason", "RefundRequestedBy",
	}

	// 用户视图与用户响应。
	userTypes := []reflect.Type{
		reflect.TypeOf(walletpb.WalletView{}),
		reflect.TypeOf(walletpb.TransactionView{}),
		reflect.TypeOf(walletpb.RechargeView{}),
		reflect.TypeOf(walletpb.GetWalletResp{}),
		reflect.TypeOf(walletpb.ListTransactionsResp{}),
		reflect.TypeOf(walletpb.ListRechargesResp{}),
		reflect.TypeOf(walletpb.CreateRechargeResp{}),
		reflect.TypeOf(walletpb.GetRechargeResp{}),
		reflect.TypeOf(walletpb.CancelRechargeResp{}),
		reflect.TypeOf(walletpb.RedeemCodeResp{}),
	}

	for _, typ := range userTypes {
		for i := 0; i < typ.NumField(); i++ {
			name := typ.Field(i).Name
			for _, bad := range forbidden {
				if name == bad {
					t.Fatalf("%s.%s 属于管理员/内部字段，不得出现在用户视图", typ.Name(), name)
				}
			}
		}
	}

	// 用户 RPC 请求只允许服务端注入的 user_id，不接受 wallet_id。
	for _, typ := range []reflect.Type{
		reflect.TypeOf(walletpb.GetRechargeReq{}),
		reflect.TypeOf(walletpb.CancelRechargeReq{}),
		reflect.TypeOf(walletpb.ListRechargesReq{}),
	} {
		if _, ok := typ.FieldByName("WalletId"); ok {
			t.Fatalf("%s 不得包含 WalletId，钱包归属只能来自 user_id + currency", typ.Name())
		}
	}

	// 管理员视图必须显式携带归属信息，形成对照。
	admin := reflect.TypeOf(walletpb.AdminRechargeView{})
	for _, name := range []string{"UserId", "WalletId", "RequestId", "MerchantOrderNo", "ReviewReason", "RefundRequestedBy"} {
		if _, ok := admin.FieldByName(name); !ok {
			t.Fatalf("AdminRechargeView 缺少管理员字段 %s", name)
		}
	}
}

// TestUserRechargeIntentIsCNYOnly 冻结 §8.1：创建充值只接受 CNY 意图，
// USD 金额与汇率由服务端快照换算并冻结。
func TestUserRechargeIntentIsCNYOnly(t *testing.T) {
	req := reflect.TypeOf(walletpb.CreateRechargeReq{})

	if _, ok := req.FieldByName("ProviderAmount"); ok {
		t.Fatalf("CreateRechargeReq 不得接受 Provider 侧 USD 金额")
	}
	if _, ok := req.FieldByName("CnyPerUsdRate"); ok {
		t.Fatalf("CreateRechargeReq 不得接受客户端汇率")
	}

	for _, name := range []string{"UserId", "RequestId"} {
		field, ok := req.FieldByName(name)
		if !ok {
			t.Fatalf("CreateRechargeReq 缺少字段 %s", name)
		}
		if field.Type != reflect.TypeOf("") {
			t.Fatalf("CreateRechargeReq.%s 必须是字符串", name)
		}
	}

	provider, ok := req.FieldByName("Provider")
	if !ok {
		t.Fatalf("CreateRechargeReq 缺少字段 Provider")
	}
	if provider.Type != reflect.TypeOf(walletpb.PaymentProvider(0)) {
		t.Fatalf("CreateRechargeReq.Provider 必须是 PaymentProvider 枚举，实际 %s", provider.Type)
	}

	amount, ok := req.FieldByName("WalletAmount")
	if !ok || amount.Type != reflect.TypeOf(int64(0)) {
		t.Fatalf("CreateRechargeReq.WalletAmount 必须是 int64 CNY 分")
	}
}
