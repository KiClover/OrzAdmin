package creem

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func readBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("读取请求体失败: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("请求体不是合法 JSON: %v", err)
	}
	return body
}

func TestCreemPaymentCreateCheckoutSendsExactFields(t *testing.T) {
	t.Parallel()

	var got map[string]any
	var gotPath, gotMethod, gotAPIKey string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotAPIKey = r.URL.Path, r.Method, r.Header.Get("x-api-key")
		got = readBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chk_1","mode":"test","object":"checkout","status":"pending",
			"product":{"id":"prod_1","name":"Wallet top-up"},
			"request_id":"req-1","units":1,"custom_price":1389,
			"checkout_url":"https://checkout.creem.io/chk_1",
			"order":{"id":"ord_1","product":"prod_1","amount":1389,"currency":"USD",
				"status":"pending","amount_paid":null,"discount_amount":0,
				"transaction":{"id":"txn_1"}},
			"metadata":{"recharge_id":"42","rate_revision":"7"}
		}`))
	})

	checkout, err := client.CreateCheckout(context.Background(), CheckoutRequest{
		ProductID:   "prod_1",
		RequestID:   "req-1",
		Units:       1,
		CustomPrice: 1389,
		SuccessURL:  "https://shop.example/success",
		Metadata:    map[string]string{"recharge_id": "42", "rate_revision": "7"},
	})
	if err != nil {
		t.Fatalf("CreateCheckout 返回错误: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/checkouts" {
		t.Fatalf("请求 = %s %s，期望 POST /checkouts", gotMethod, gotPath)
	}
	if gotAPIKey != testAPIKey {
		t.Fatalf("x-api-key = %q，期望测试密钥", gotAPIKey)
	}
	// 精确字段：只允许钱包需要的键，不得携带 discount/affiliate/customer。
	wantKeys := map[string]bool{
		"product_id": true, "request_id": true, "units": true,
		"custom_price": true, "success_url": true, "metadata": true,
	}
	for key := range got {
		if !wantKeys[key] {
			t.Fatalf("请求中出现未预期字段 %q: %v", key, got)
		}
	}
	for key := range wantKeys {
		if _, ok := got[key]; !ok {
			t.Fatalf("请求缺少字段 %q: %v", key, got)
		}
	}
	if got["product_id"] != "prod_1" || got["request_id"] != "req-1" {
		t.Fatalf("product_id/request_id 不符: %v", got)
	}
	if got["units"].(float64) != 1 || got["custom_price"].(float64) != 1389 {
		t.Fatalf("units/custom_price 不符: %v", got)
	}
	if got["success_url"] != "https://shop.example/success" {
		t.Fatalf("success_url 不符: %v", got)
	}

	if checkout.ID != "chk_1" || checkout.Status != "pending" {
		t.Fatalf("Checkout 映射不符: %+v", checkout)
	}
	if checkout.Product.ID != "prod_1" {
		t.Fatalf("product 对象形态未解析出 ID: %+v", checkout.Product)
	}
	if checkout.Order == nil || checkout.Order.Transaction.ID != "txn_1" {
		t.Fatalf("order.transaction 未解析: %+v", checkout.Order)
	}
	if checkout.Order.AmountPaid != nil {
		t.Fatalf("amount_paid 为 null 时应保持 nil: %v", *checkout.Order.AmountPaid)
	}
	if checkout.Metadata["rate_revision"] != "7" {
		t.Fatalf("metadata 未保留: %v", checkout.Metadata)
	}
}

func TestCreemPaymentCheckoutProductStringForm(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"chk_2","status":"completed","product":"prod_abc","units":1,"custom_price":50}`))
	})
	checkout, err := client.CreateCheckout(context.Background(), CheckoutRequest{
		ProductID: "prod_abc", Units: 1, CustomPrice: 50,
	})
	if err != nil {
		t.Fatalf("CreateCheckout 返回错误: %v", err)
	}
	if checkout.Product.ID != "prod_abc" {
		t.Fatalf("字符串形态 product 未解析: %+v", checkout.Product)
	}
}

func TestCreemPaymentGetCheckoutUsesQueryParameter(t *testing.T) {
	t.Parallel()

	var gotQuery, gotPath string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"id":"chk_9","mode":"test","status":"expired","product":"prod_1"}`))
	})

	checkout, err := client.GetCheckout(context.Background(), "chk_9")
	if err != nil {
		t.Fatalf("GetCheckout 返回错误: %v", err)
	}
	if gotPath != "/checkouts" || gotQuery != "checkout_id=chk_9" {
		t.Fatalf("请求 = %s?%s，期望 /checkouts?checkout_id=chk_9", gotPath, gotQuery)
	}
	if checkout.Status != "expired" {
		t.Fatalf("status = %q，期望 expired", checkout.Status)
	}
}

func TestCreemPaymentGetTransactionMapsNullableAmounts(t *testing.T) {
	t.Parallel()

	var gotQuery string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{
			"id":"txn_1","mode":"test","amount":1389,"currency":"USD","type":"payment",
			"status":"refunded","created_at":1700000000,"amount_paid":1389,
			"discount_amount":0,"tax_amount":120,"refunded_amount":1389,
			"order":"ord_1"
		}`))
	})

	transaction, err := client.GetTransaction(context.Background(), "txn_1")
	if err != nil {
		t.Fatalf("GetTransaction 返回错误: %v", err)
	}
	if gotQuery != "transaction_id=txn_1" {
		t.Fatalf("query = %q", gotQuery)
	}
	if transaction.Status != "refunded" || transaction.Amount != 1389 {
		t.Fatalf("交易映射不符: %+v", transaction)
	}
	if transaction.AmountPaid == nil || *transaction.AmountPaid != 1389 {
		t.Fatalf("amount_paid 未解析: %+v", transaction.AmountPaid)
	}
	if transaction.RefundedAmount == nil || *transaction.RefundedAmount != 1389 {
		t.Fatalf("refunded_amount 未解析: %+v", transaction.RefundedAmount)
	}
	if transaction.Order.ID != "ord_1" {
		t.Fatalf("order 未解析: %+v", transaction.Order)
	}
}

func TestCreemPaymentCreateFullRefundPostsTransactionID(t *testing.T) {
	t.Parallel()

	var gotPath string
	var got map[string]any
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		got = readBody(t, r)
		_, _ = w.Write([]byte(`{"id":"ref_1","mode":"test","status":"pending","transaction":"txn_1","amount":1389}`))
	})

	refund, err := client.CreateFullRefund(context.Background(), "txn_1")
	if err != nil {
		t.Fatalf("CreateFullRefund 返回错误: %v", err)
	}
	if gotPath != "/refunds" {
		t.Fatalf("path = %q，期望 /refunds", gotPath)
	}
	if got["transaction_id"] != "txn_1" {
		t.Fatalf("transaction_id 不符: %v", got)
	}
	if len(got) != 1 {
		t.Fatalf("退款请求不应包含其它字段: %v", got)
	}
	if refund.Status != "pending" || refund.Transaction.ID != "txn_1" {
		t.Fatalf("退款映射不符: %+v", refund)
	}
}

func TestCreemPaymentClientRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	client, err := NewClient(Options{Environment: EnvironmentTest, APIKey: testAPIKey})
	if err != nil {
		t.Fatalf("NewClient 返回错误: %v", err)
	}
	if _, err := client.CreateCheckout(context.Background(), CheckoutRequest{Units: 1}); err == nil {
		t.Fatal("缺少 product_id 应报错")
	}
	if _, err := client.CreateCheckout(context.Background(), CheckoutRequest{ProductID: "prod_1"}); err == nil {
		t.Fatal("units 必须为正")
	}
	if _, err := client.GetCheckout(context.Background(), ""); err == nil {
		t.Fatal("空 checkout_id 应报错")
	}
	if _, err := client.GetTransaction(context.Background(), ""); err == nil {
		t.Fatal("空 transaction_id 应报错")
	}
	if _, err := client.CreateFullRefund(context.Background(), ""); err == nil {
		t.Fatal("空退款 transaction_id 应报错")
	}
}

func TestCreemPaymentErrorClassification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		want   error
	}{
		{name: "rejected", status: http.StatusUnprocessableEntity, want: ErrRejected},
		{name: "conflict", status: http.StatusConflict, want: ErrRejected},
		{name: "unauthorized", status: http.StatusUnauthorized, want: ErrUnauthorized},
		{name: "not_found", status: http.StatusNotFound, want: ErrNotFound},
		{name: "unavailable", status: http.StatusBadGateway, want: ErrUnavailable},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"error":"`+leakyBody+`"}`)
			})
			_, err := client.GetCheckout(context.Background(), "chk_1")
			if !errors.Is(err, tc.want) {
				t.Fatalf("errors.Is(%v, %v) = false", err, tc.want)
			}
			if strings.Contains(err.Error(), leakyBody) {
				t.Fatalf("错误信息不得夹带响应体: %v", err)
			}
			if strings.Contains(err.Error(), testAPIKey) {
				t.Fatalf("错误信息不得夹带 API Key: %v", err)
			}
		})
	}
}
