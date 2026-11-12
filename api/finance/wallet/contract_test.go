package wallet_test

import (
	"reflect"
	"strings"
	"testing"

	financewallet "github.com/KiClover/orzAdmin/api/model/finance/wallet"
)

// tagValue 返回结构体字段上指定 tag 键的值。
func tagValue(field reflect.StructField, key string) string {
	return field.Tag.Get(key)
}

// findField 按 thrift 字段名（snake_case，即生成代码中 thrift tag 的第一段）定位字段。
func findField(t *testing.T, typ reflect.Type, thriftName string) reflect.StructField {
	t.Helper()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if strings.SplitN(f.Tag.Get("thrift"), ",", 2)[0] == thriftName {
			return f
		}
	}
	t.Fatalf("%s 缺少 thrift 字段 %s", typ.Name(), thriftName)
	return reflect.StructField{}
}

// TestPathBindingsFollowDesign 冻结 §11.2–11.4 中仍保留路径参数的绑定名。
//
// 只读查询已统一改为 POST + JSON body，因此这里只剩写操作与按 ID 读取的详情路由。
func TestPathBindingsFollowDesign(t *testing.T) {
	cases := []struct {
		typ        reflect.Type
		thriftName string
		path       string
	}{
		{reflect.TypeOf(financewallet.GetRechargeReq{}), "recharge_id", "rechargeId"},
		{reflect.TypeOf(financewallet.CancelRechargeReq{}), "recharge_id", "rechargeId"},
		{reflect.TypeOf(financewallet.AdminGetRechargeReq{}), "recharge_id", "rechargeId"},
		{reflect.TypeOf(financewallet.AdminRefreshRechargeReq{}), "recharge_id", "rechargeId"},
		{reflect.TypeOf(financewallet.AdminRefundRechargeReq{}), "recharge_id", "rechargeId"},
		{reflect.TypeOf(financewallet.AdminSetRedemptionCodeStatusReq{}), "code_id", "codeId"},
	}

	for _, tc := range cases {
		field := findField(t, tc.typ, tc.thriftName)
		if got := tagValue(field, "path"); got != tc.path {
			t.Fatalf("%s.%s 的 path 绑定应为 %q，实际 %q", tc.typ.Name(), tc.thriftName, tc.path, got)
		}
	}
}

// TestReadQueriesBindJSONBody 冻结只读查询的 POST + JSON body 约定：
// 这些请求不再接受 query 参数，所有过滤条件都来自请求体。
func TestReadQueriesBindJSONBody(t *testing.T) {
	cases := []struct {
		typ    reflect.Type
		fields []string
	}{
		{reflect.TypeOf(financewallet.ListTransactionsReq{}), []string{"type", "start_time", "end_time", "page_size", "cursor"}},
		{reflect.TypeOf(financewallet.ListRechargesReq{}), []string{"status", "page_size", "cursor"}},
		{reflect.TypeOf(financewallet.AdminListWalletsReq{}), []string{"user_id", "status", "page_size", "cursor"}},
		{reflect.TypeOf(financewallet.AdminGetWalletReq{}), []string{"wallet_id", "user_id"}},
		{reflect.TypeOf(financewallet.AdminListTransactionsReq{}), []string{"wallet_id", "user_id", "type", "start_time", "end_time", "page_size", "cursor"}},
		{reflect.TypeOf(financewallet.AdminListRechargesReq{}), []string{"user_id", "status", "provider", "page_size", "cursor"}},
		{reflect.TypeOf(financewallet.AdminListRedemptionCodesReq{}), []string{"type", "status", "page_size", "cursor"}},
	}

	for _, tc := range cases {
		for _, name := range tc.fields {
			field := findField(t, tc.typ, name)
			if got := tagValue(field, "path"); got != "" {
				t.Fatalf("%s.%s 不应再有 path 绑定，实际 %q", tc.typ.Name(), name, got)
			}
			if got := tagValue(field, "query"); got != "" {
				t.Fatalf("%s.%s 不应再有 query 绑定，实际 %q", tc.typ.Name(), name, got)
			}
			if got := tagValue(field, "form"); got != name {
				t.Fatalf("%s.%s 的 body 绑定应为 %q，实际 %q", tc.typ.Name(), name, name, got)
			}
		}
	}
}

// TestUserRequestsRejectIdentityOverride 冻结 §11.2/§12.1：
// 用户 HTTP 请求不得包含 user_id / wallet_id / request 归属等身份字段。
func TestUserRequestsRejectIdentityOverride(t *testing.T) {
	forbiddenBody := []string{
		`form:"user_id"`, `form:"wallet_id"`, `form:"operator_id"`,
		`query:"user_id"`, `query:"wallet_id"`, `query:"operator_id"`,
	}

	userRequests := []reflect.Type{
		reflect.TypeOf(financewallet.CreateRechargeReq{}),
		reflect.TypeOf(financewallet.ListTransactionsReq{}),
		reflect.TypeOf(financewallet.ListRechargesReq{}),
		reflect.TypeOf(financewallet.CancelRechargeReq{}),
		reflect.TypeOf(financewallet.RedeemCodeReq{}),
		reflect.TypeOf(financewallet.GetRechargeReq{}),
	}

	for _, typ := range userRequests {
		for i := 0; i < typ.NumField(); i++ {
			tag := string(typ.Field(i).Tag)
			for _, bad := range forbiddenBody {
				if strings.Contains(tag, bad) {
					t.Fatalf("%s.%s 不得绑定身份字段 %s", typ.Name(), typ.Field(i).Name, bad)
				}
			}
		}
	}
}

// TestUserViewsExposeNoIdentity 冻结 §11.6：
// 用户钱包/充值/流水视图不得暴露 wallet_id、user_id 或 Provider 内部标识。
func TestUserViewsExposeNoIdentity(t *testing.T) {
	forbidden := []string{
		"user_id", "wallet_id", "request_id", "operator_id",
		"merchant_order_no", "payment_profile_id", "provider_product_id",
		"provider_checkout_id", "provider_order_id", "provider_transaction_id",
		"review_reason", "refund_requested_by", "refund_reason",
	}

	userViews := []reflect.Type{
		reflect.TypeOf(financewallet.WalletView{}),
		reflect.TypeOf(financewallet.TransactionView{}),
		reflect.TypeOf(financewallet.RechargeView{}),
		reflect.TypeOf(financewallet.GetWalletResp{}),
		reflect.TypeOf(financewallet.ListTransactionsResp{}),
		reflect.TypeOf(financewallet.ListRechargesResp{}),
		reflect.TypeOf(financewallet.RedeemCodeResp{}),
	}

	for _, typ := range userViews {
		for i := 0; i < typ.NumField(); i++ {
			name := strings.SplitN(typ.Field(i).Tag.Get("thrift"), ",", 2)[0]
			for _, bad := range forbidden {
				if name == bad {
					t.Fatalf("%s 不得暴露字段 %s", typ.Name(), bad)
				}
			}
		}
	}
}

// TestAdminRedemptionCodeViewCarriesPlaintextCode 冻结 §9.1：
// 明文兑换码只出现在管理员视图，且该视图不含 Secret。
func TestAdminRedemptionCodeViewCarriesPlaintextCode(t *testing.T) {
	typ := reflect.TypeOf(financewallet.AdminRedemptionCodeView{})

	code := findField(t, typ, "code")
	if got := code.Type.Kind(); got != reflect.Ptr || code.Type.Elem().Kind() != reflect.String {
		t.Fatalf("AdminRedemptionCodeView.code 必须是 *string，实际 %s", code.Type)
	}

	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		if strings.Contains(name, "secret") || strings.Contains(name, "apikey") {
			t.Fatalf("AdminRedemptionCodeView 不得包含 Secret 字段 %s", typ.Field(i).Name)
		}
	}
}

// TestAdminWalletViewCarriesWalletID 冻结 §11.6：
// 钱包主键只出现在管理员视图，用户 WalletView 不得携带。
func TestAdminWalletViewCarriesWalletID(t *testing.T) {
	typ := reflect.TypeOf(financewallet.AdminWalletView{})
	field := findField(t, typ, "wallet_id")
	if field.Type.Kind() != reflect.Ptr || field.Type.Elem().Kind() != reflect.String {
		t.Fatalf("AdminWalletView.wallet_id 必须是 *string，实际 %s", field.Type)
	}

	userTyp := reflect.TypeOf(financewallet.WalletView{})
	for i := 0; i < userTyp.NumField(); i++ {
		if strings.SplitN(userTyp.Field(i).Tag.Get("thrift"), ",", 2)[0] == "wallet_id" {
			t.Fatalf("用户 WalletView 不得携带 wallet_id")
		}
	}
}

// TestRechargeViewKeepsDualCurrencyAmounts 冻结 §5.1/§11.6：
// 用户充值视图同时展示 CNY 与 USD 冻结金额，且均为整数最小单位。
func TestRechargeViewKeepsDualCurrencyAmounts(t *testing.T) {
	typ := reflect.TypeOf(financewallet.RechargeView{})

	for _, name := range []string{"wallet_amount", "provider_amount"} {
		field := findField(t, typ, name)
		if field.Type.Kind() != reflect.Ptr || field.Type.Elem().Kind() != reflect.Int64 {
			t.Fatalf("RechargeView.%s 必须是 *int64 最小单位，实际 %s", name, field.Type)
		}
	}
	for _, name := range []string{"wallet_currency", "provider_currency"} {
		field := findField(t, typ, name)
		if field.Type.Kind() != reflect.Ptr || field.Type.Elem().Kind() != reflect.String {
			t.Fatalf("RechargeView.%s 必须是 *string，实际 %s", name, field.Type)
		}
	}
}

// TestPaymentCallbackKeepsRawBodyContract 冻结 §11.3：
// 回调以原始 body 验签，因此不存在把 Provider 事件解析成固定请求 DTO 的类型，
// 只保留极简的确认响应。
func TestPaymentCallbackKeepsRawBodyContract(t *testing.T) {
	typ := reflect.TypeOf(financewallet.PaymentCallbackResp{})

	received := findField(t, typ, "received")
	if received.Type.Kind() != reflect.Ptr || received.Type.Elem().Kind() != reflect.Bool {
		t.Fatalf("PaymentCallbackResp.received 必须是 *bool，实际 %s", received.Type)
	}

	message := findField(t, typ, "message")
	if message.Type.Kind() != reflect.Ptr || message.Type.Elem().Kind() != reflect.String {
		t.Fatalf("PaymentCallbackResp.message 必须是 *string，实际 %s", message.Type)
	}
}
