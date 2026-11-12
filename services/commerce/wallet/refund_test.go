package wallet

import (
	"context"
	"strings"
	"testing"

	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

func TestParseOperator(t *testing.T) {
	valid := []string{"9001", "admin-1", " service:finance "}
	for _, raw := range valid {
		got, err := parseOperator(raw)
		if err != nil {
			t.Fatalf("%q 应被接受: %v", raw, err)
		}
		if got != strings.TrimSpace(raw) {
			t.Fatalf("操作人应去除首尾空白: %q -> %q", raw, got)
		}
	}

	if _, err := parseOperator("   "); err == nil {
		t.Fatal("空操作人必须拒绝")
	}
	if _, err := parseOperator(strings.Repeat("a", 129)); err == nil {
		t.Fatal("超过 128 字符的操作人必须拒绝")
	}
	if _, err := parseOperator(strings.Repeat("a", 128)); err != nil {
		t.Fatalf("128 字符操作人应被接受: %v", err)
	}
}

// TestRefundValidatesBeforeTouchingProvider 验证参数校验先于任何 Provider 调用与数据库访问。
func TestRefundValidatesBeforeTouchingProvider(t *testing.T) {
	svc := &CommerceWalletService{}
	ctx := context.Background()

	cases := []struct {
		name string
		req  *wallet.AdminRefundRechargeReq
	}{
		{"missing operator", &wallet.AdminRefundRechargeReq{RechargeId: "1", Reason: "reason"}},
		{"invalid recharge id", &wallet.AdminRefundRechargeReq{
			OperatorId: "9001", RechargeId: "0", Reason: "reason",
		}},
		{"missing reason", &wallet.AdminRefundRechargeReq{
			OperatorId: "9001", RechargeId: "1", Reason: "   ",
		}},
		{"reason too long", &wallet.AdminRefundRechargeReq{
			OperatorId: "9001", RechargeId: "1", Reason: strings.Repeat("x", 513),
		}},
	}
	for _, tc := range cases {
		if _, err := svc.AdminRefundRecharge(ctx, tc.req); err == nil {
			t.Fatalf("%s: 必须拒绝", tc.name)
		}
	}
}

// TestRefreshValidatesBeforeTouch 验证刷新入口的入参校验同样先失败。
func TestRefreshValidatesBeforeTouch(t *testing.T) {
	svc := &CommerceWalletService{}
	ctx := context.Background()

	if _, err := svc.AdminRefreshRecharge(ctx, &wallet.AdminRefreshRechargeReq{
		OperatorId: "9001",
		RechargeId: "not-a-number",
	}); err == nil {
		t.Fatal("非法 recharge_id 必须拒绝")
	}
	if _, err := svc.AdminRefreshRecharge(ctx, &wallet.AdminRefreshRechargeReq{
		RechargeId: "1",
	}); err == nil {
		t.Fatal("缺少 operator_id 必须拒绝")
	}
}
