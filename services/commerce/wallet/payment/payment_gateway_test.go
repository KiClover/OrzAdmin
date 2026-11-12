package payment

import (
	"errors"
	"testing"

	walletpb "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

func TestPaymentGatewayCheckoutStatusMapping(t *testing.T) {
	t.Parallel()

	cases := map[string]CheckoutStatus{
		"pending":    CheckoutStatusPending,
		"processing": CheckoutStatusProcessing,
		"completed":  CheckoutStatusCompleted,
		"expired":    CheckoutStatusExpired,
		"":           CheckoutStatusUnknown,
		"refunded":   CheckoutStatusUnknown,
		"PENDING":    CheckoutStatusUnknown,
	}
	for raw, want := range cases {
		if got := ParseCheckoutStatus(raw); got != want {
			t.Fatalf("ParseCheckoutStatus(%q) = %q，期望 %q", raw, got, want)
		}
	}
}

func TestPaymentGatewayTransactionStatusMapping(t *testing.T) {
	t.Parallel()

	cases := map[string]TransactionStatus{
		"pending":       TransactionStatusPending,
		"paid":          TransactionStatusPaid,
		"refunded":      TransactionStatusRefunded,
		"partialRefund": TransactionStatusPartial,
		"chargedBack":   TransactionStatusChargeback,
		"uncollectible": TransactionStatusUncollect,
		"declined":      TransactionStatusDeclined,
		"canceled":      TransactionStatusCanceled,
		"void":          TransactionStatusVoided,
		"succeeded":     TransactionStatusUnknown,
		"":              TransactionStatusUnknown,
	}
	for raw, want := range cases {
		if got := ParseTransactionStatus(raw); got != want {
			t.Fatalf("ParseTransactionStatus(%q) = %q，期望 %q", raw, got, want)
		}
	}
}

func TestPaymentGatewayRefundStatusMapping(t *testing.T) {
	t.Parallel()

	cases := map[string]RefundStatus{
		"pending":   RefundStatusPending,
		"succeeded": RefundStatusSucceeded,
		"failed":    RefundStatusFailed,
		"canceled":  RefundStatusCanceled,
		"unknown":   RefundStatusUnknown,
	}
	for raw, want := range cases {
		if got := ParseRefundStatus(raw); got != want {
			t.Fatalf("ParseRefundStatus(%q) = %q，期望 %q", raw, got, want)
		}
	}
}

func TestPaymentGatewayExpectedProviderMode(t *testing.T) {
	t.Parallel()

	if mode, err := expectedProviderMode(walletpb.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST); err != nil || mode != "test" {
		t.Fatalf("TEST 环境 mode = %q, err = %v", mode, err)
	}
	if mode, err := expectedProviderMode(walletpb.ProviderEnvironment_PROVIDER_ENVIRONMENT_PRODUCTION); err != nil || mode != "prod" {
		t.Fatalf("PRODUCTION 环境 mode = %q, err = %v", mode, err)
	}
	if _, err := expectedProviderMode(walletpb.ProviderEnvironment_PROVIDER_ENVIRONMENT_UNSPECIFIED); err == nil {
		t.Fatal("未指定环境应报错")
	}
}

func validCheckout() CheckoutResult {
	return CheckoutResult{
		ID:          "chk_1",
		URL:         "https://checkout.creem.io/chk_1",
		Status:      CheckoutStatusPending,
		RawStatus:   "pending",
		Mode:        "test",
		ProductID:   "prod_1",
		Units:       1,
		CustomPrice: 1389,
		RequestID:   "req-1",
		Order: &ProviderOrder{
			ID:            "ord_1",
			ProductID:     "prod_1",
			Currency:      "USD",
			Status:        "pending",
			Amount:        1389,
			TransactionID: "txn_1",
		},
	}
}

func checkoutExpectation() CheckoutExpectation {
	return CheckoutExpectation{
		Environment:    walletpb.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST,
		ProductID:      "prod_1",
		ProviderAmount: 1389,
		RequestID:      "req-1",
	}
}

func TestPaymentGatewayValidateCheckoutAcceptsFrozenQuote(t *testing.T) {
	t.Parallel()

	result := validCheckout()
	if err := ValidateCheckout(result, checkoutExpectation()); err != nil {
		t.Fatalf("合法 Checkout 应通过: %v", err)
	}

	// order 尚不存在时允许，但不能凭空携带其它币种/金额。
	result.Order = nil
	if err := ValidateCheckout(result, checkoutExpectation()); err != nil {
		t.Fatalf("无 order 的 pending Checkout 应通过: %v", err)
	}
}

func TestPaymentGatewayValidateCheckoutRejectsContractViolations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*CheckoutResult)
	}{
		{name: "mode", mutate: func(r *CheckoutResult) { r.Mode = "prod" }},
		{name: "product", mutate: func(r *CheckoutResult) { r.ProductID = "prod_2" }},
		{name: "request_id", mutate: func(r *CheckoutResult) { r.RequestID = "req-2" }},
		{name: "custom_price", mutate: func(r *CheckoutResult) { r.CustomPrice = 1400 }},
		{name: "unknown_status", mutate: func(r *CheckoutResult) {
			r.Status = CheckoutStatusUnknown
			r.RawStatus = "refunded"
		}},
		{name: "order_currency", mutate: func(r *CheckoutResult) { r.Order.Currency = "CNY" }},
		{name: "order_amount", mutate: func(r *CheckoutResult) { r.Order.Amount = 1400 }},
		{name: "order_product", mutate: func(r *CheckoutResult) { r.Order.ProductID = "prod_9" }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := validCheckout()
			tc.mutate(&result)
			err := ValidateCheckout(result, checkoutExpectation())
			if !errors.Is(err, errProviderContract) {
				t.Fatalf("期望 errProviderContract，实际 %v", err)
			}
		})
	}
}

func validPaidTransaction() TransactionResult {
	amount := int64(1389)
	return TransactionResult{
		ID:         "txn_1",
		Mode:       "test",
		Currency:   "USD",
		Type:       "payment",
		Status:     TransactionStatusPaid,
		RawStatus:  "paid",
		Amount:     1389,
		AmountPaid: &amount,
		OrderID:    "ord_1",
	}
}

func transactionExpectation() TransactionExpectation {
	return TransactionExpectation{
		Environment:    walletpb.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST,
		ProviderAmount: 1389,
	}
}

func TestPaymentGatewayValidatePaidTransactionAcceptsExactPayment(t *testing.T) {
	t.Parallel()

	result := validPaidTransaction()
	if err := ValidatePaidTransaction(result, transactionExpectation()); err != nil {
		t.Fatalf("合法已支付交易应通过: %v", err)
	}

	// 税包含在 custom_price 内，允许非零；折扣必须为 0 或缺失。
	tax := int64(120)
	zero := int64(0)
	result.TaxAmount = &tax
	result.DiscountAmount = &zero
	if err := ValidatePaidTransaction(result, transactionExpectation()); err != nil {
		t.Fatalf("含税且零折扣应通过: %v", err)
	}
}

func TestPaymentGatewayValidatePaidTransactionRejectsContractViolations(t *testing.T) {
	t.Parallel()

	other := int64(1400)
	discount := int64(5)
	cases := []struct {
		name   string
		mutate func(*TransactionResult)
	}{
		{name: "mode", mutate: func(r *TransactionResult) { r.Mode = "prod" }},
		{name: "type", mutate: func(r *TransactionResult) { r.Type = "invoice" }},
		{name: "unpaid", mutate: func(r *TransactionResult) {
			r.Status = TransactionStatusPending
			r.RawStatus = "pending"
		}},
		{name: "currency", mutate: func(r *TransactionResult) { r.Currency = "EUR" }},
		{name: "amount", mutate: func(r *TransactionResult) { r.Amount = 1400 }},
		{name: "amount_paid_missing", mutate: func(r *TransactionResult) { r.AmountPaid = nil }},
		{name: "amount_paid_other", mutate: func(r *TransactionResult) { r.AmountPaid = &other }},
		{name: "discount", mutate: func(r *TransactionResult) { r.DiscountAmount = &discount }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := validPaidTransaction()
			tc.mutate(&result)
			err := ValidatePaidTransaction(result, transactionExpectation())
			if !errors.Is(err, errProviderContract) {
				t.Fatalf("期望 errProviderContract，实际 %v", err)
			}
		})
	}
}
