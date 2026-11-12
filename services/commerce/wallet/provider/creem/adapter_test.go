package creem

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	creemapi "github.com/KiClover/orzAdmin/internal/creem"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
)

const (
	testAPIKey = "creem_test_key_must_never_leak"
	leakyBody  = "LEAKY_BODY_MARKER_9d2f"
	testProdID = "prod_wallet_usd"
	testURL    = "https://shop.example/pay/success"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func testCredentials(env wallet.ProviderEnvironment) payment.GatewayCredentials {
	return payment.GatewayCredentials{
		Environment: env,
		APIKey:      testAPIKey,
		ProductID:   testProdID,
		SuccessURL:  testURL,
	}
}

func newTestAdapter(t *testing.T, env wallet.ProviderEnvironment, doer creemapi.Doer) *Adapter {
	t.Helper()
	adapter, err := New(Options{
		Credentials:      testCredentials(env),
		Timeout:          2 * time.Second,
		MaxResponseBytes: 64 << 10,
		Doer:             doer,
	})
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	return adapter
}

func TestCreemAdapterNewValidatesCredentials(t *testing.T) {
	t.Parallel()

	valid := testCredentials(wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST)
	cases := []struct {
		name   string
		mutate func(*payment.GatewayCredentials)
	}{
		{name: "env", mutate: func(c *payment.GatewayCredentials) {
			c.Environment = wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_UNSPECIFIED
		}},
		{name: "api_key", mutate: func(c *payment.GatewayCredentials) { c.APIKey = "" }},
		{name: "product", mutate: func(c *payment.GatewayCredentials) { c.ProductID = "" }},
		{name: "success_url_http", mutate: func(c *payment.GatewayCredentials) { c.SuccessURL = "http://shop.example/success" }},
		{name: "success_url_relative", mutate: func(c *payment.GatewayCredentials) { c.SuccessURL = "/success" }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			credentials := valid
			tc.mutate(&credentials)
			if _, err := New(Options{Credentials: credentials}); err == nil {
				t.Fatalf("%s 非法凭据应报错", tc.name)
			}
		})
	}
}

func TestCreemAdapterQueryGatewayAllowsMissingSuccessURL(t *testing.T) {
	t.Parallel()

	credentials := testCredentials(wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST)
	credentials.SuccessURL = ""
	adapter, err := New(Options{
		Credentials: credentials,
		Doer: doerFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("查询类网关不应在建单路径发出请求")
			return nil, nil
		}),
	})
	if err != nil {
		t.Fatalf("查询网关允许缺少成功跳转地址: %v", err)
	}
	_, err = adapter.CreateCheckout(context.Background(), payment.CheckoutRequest{
		CustomPrice: 100,
		RequestID:   "req-1",
	})
	if !errors.Is(err, payment.ErrGatewayInvalidResponse) {
		t.Fatalf("缺少成功跳转地址时建单必须失败，实际 %v", err)
	}
}

func TestCreemAdapterCreateCheckoutUsesFrozenProductAndAmount(t *testing.T) {
	t.Parallel()

	var gotURL, gotMethod, gotAPIKey string
	var gotBody map[string]any
	adapter := newTestAdapter(t, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST, doerFunc(func(req *http.Request) (*http.Response, error) {
		gotURL, gotMethod, gotAPIKey = req.URL.String(), req.Method, req.Header.Get("x-api-key")
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("读取请求体失败: %v", err)
		}
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("请求体不是合法 JSON: %v", err)
		}
		return jsonResponse(`{
			"id":"chk_1","mode":"test","status":"pending","object":"checkout",
			"product":{"id":"` + testProdID + `"},
			"request_id":"req-1","units":1,"custom_price":1389,
			"checkout_url":"https://checkout.creem.io/chk_1",
			"success_url":"` + testURL + `",
			"order":{"id":"ord_1","product":"` + testProdID + `","amount":1389,"currency":"USD",
				"status":"pending","amount_paid":null,"discount_amount":0,"transaction":"txn_1"},
			"metadata":{"recharge_id":"42"}
		}`), nil
	}))

	result, err := adapter.CreateCheckout(context.Background(), payment.CheckoutRequest{
		CustomPrice: 1389,
		RequestID:   "req-1",
		Metadata:    map[string]string{"recharge_id": "42"},
	})
	if err != nil {
		t.Fatalf("CreateCheckout 返回错误: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s，期望 POST", gotMethod)
	}
	if !strings.HasPrefix(gotURL, "https://test-api.creem.io/v1/checkouts") {
		t.Fatalf("必须固定 TEST 主机: %s", gotURL)
	}
	if gotAPIKey != testAPIKey {
		t.Fatalf("x-api-key 不符: %q", gotAPIKey)
	}
	if gotBody["units"].(float64) != 1 {
		t.Fatalf("units 必须固定为 1: %v", gotBody["units"])
	}
	if gotBody["custom_price"].(float64) != 1389 || gotBody["product_id"] != testProdID {
		t.Fatalf("冻结金额/产品不符: %v", gotBody)
	}
	if gotBody["success_url"] != testURL {
		t.Fatalf("success_url 必须来自 Profile: %v", gotBody["success_url"])
	}
	if _, ok := gotBody["discount_code"]; ok {
		t.Fatalf("不应发送折扣字段: %v", gotBody)
	}

	if result.Status != payment.CheckoutStatusPending || result.RawStatus != "pending" {
		t.Fatalf("状态映射不符: %+v", result)
	}
	if result.Order == nil || result.Order.TransactionID != "txn_1" {
		t.Fatalf("order 映射不符: %+v", result.Order)
	}
}

func TestCreemAdapterCreateCheckoutUsesProductionHost(t *testing.T) {
	t.Parallel()

	var gotURL string
	adapter := newTestAdapter(t, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_PRODUCTION, doerFunc(func(req *http.Request) (*http.Response, error) {
		gotURL = req.URL.String()
		return jsonResponse(`{"id":"chk_2","mode":"prod","status":"pending","product":"` + testProdID + `","units":1,"custom_price":1389}`), nil
	}))

	if _, err := adapter.CreateCheckout(context.Background(), payment.CheckoutRequest{CustomPrice: 1389, RequestID: "req-2"}); err != nil {
		t.Fatalf("CreateCheckout 返回错误: %v", err)
	}
	if !strings.HasPrefix(gotURL, "https://api.creem.io/v1/checkouts") {
		t.Fatalf("必须固定 PRODUCTION 主机: %s", gotURL)
	}
}

func TestCreemAdapterCreateCheckoutRejectsContractViolations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "mode", body: `{"id":"c","mode":"prod","status":"pending","product":"` + testProdID + `","units":1,"custom_price":1389}`},
		{name: "product", body: `{"id":"c","mode":"test","status":"pending","product":"prod_other","units":1,"custom_price":1389}`},
		{name: "units", body: `{"id":"c","mode":"test","status":"pending","product":"` + testProdID + `","units":2,"custom_price":1389}`},
		{name: "price", body: `{"id":"c","mode":"test","status":"pending","product":"` + testProdID + `","units":1,"custom_price":1400}`},
		{name: "request_id", body: `{"id":"c","mode":"test","status":"pending","product":"` + testProdID + `","units":1,"custom_price":1389,"request_id":"req-other"}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			adapter := newTestAdapter(t, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST, doerFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(tc.body), nil
			}))
			_, err := adapter.CreateCheckout(context.Background(), payment.CheckoutRequest{CustomPrice: 1389, RequestID: "req-1"})
			if !errors.Is(err, payment.ErrGatewayInvalidResponse) {
				t.Fatalf("期望 ErrGatewayInvalidResponse，实际 %v", err)
			}
		})
	}
}

func TestCreemAdapterCreateCheckoutRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST, doerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("非法输入不应发出请求")
		return nil, nil
	}))
	if _, err := adapter.CreateCheckout(context.Background(), payment.CheckoutRequest{RequestID: "req"}); !errors.Is(err, payment.ErrGatewayInvalidResponse) {
		t.Fatalf("金额非正应报契约错误: %v", err)
	}
	if _, err := adapter.CreateCheckout(context.Background(), payment.CheckoutRequest{CustomPrice: 100}); !errors.Is(err, payment.ErrGatewayInvalidResponse) {
		t.Fatalf("缺少请求号应报契约错误: %v", err)
	}
}

func TestCreemAdapterGetCheckoutPreservesUnknownStatus(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST, doerFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("checkout_id") != "chk_9" {
			t.Fatalf("checkout_id 查询参数不符: %s", req.URL.RawQuery)
		}
		return jsonResponse(`{"id":"chk_9","mode":"test","status":"refunded","product":"` + testProdID + `"}`), nil
	}))

	result, err := adapter.GetCheckout(context.Background(), "chk_9")
	if err != nil {
		t.Fatalf("GetCheckout 返回错误: %v", err)
	}
	if result.Status != payment.CheckoutStatusUnknown || result.RawStatus != "refunded" {
		t.Fatalf("未知状态必须保留原文并映射为 Unknown: %+v", result)
	}
}

func TestCreemAdapterGetCheckoutRejectsModeMismatch(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST, doerFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"id":"chk_1","mode":"prod","status":"pending","product":"` + testProdID + `"}`), nil
	}))
	if _, err := adapter.GetCheckout(context.Background(), "chk_1"); !errors.Is(err, payment.ErrGatewayInvalidResponse) {
		t.Fatalf("环境不一致应报契约错误: %v", err)
	}
	if _, err := adapter.GetCheckout(context.Background(), ""); !errors.Is(err, payment.ErrGatewayInvalidResponse) {
		t.Fatalf("空 ID 应报契约错误: %v", err)
	}
}

func TestCreemAdapterGetTransactionMapsAmounts(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST, doerFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("transaction_id") != "txn_1" {
			t.Fatalf("transaction_id 查询参数不符: %s", req.URL.RawQuery)
		}
		return jsonResponse(`{
			"id":"txn_1","mode":"test","amount":1389,"currency":"USD","type":"payment",
			"status":"partialRefund","amount_paid":1389,"discount_amount":0,
			"refunded_amount":500,"order":"ord_1"
		}`), nil
	}))

	result, err := adapter.GetTransaction(context.Background(), "txn_1")
	if err != nil {
		t.Fatalf("GetTransaction 返回错误: %v", err)
	}
	if result.Status != payment.TransactionStatusPartial {
		t.Fatalf("状态映射不符: %+v", result.Status)
	}
	if result.RefundedAmount == nil || *result.RefundedAmount != 500 {
		t.Fatalf("refunded_amount 映射不符: %v", result.RefundedAmount)
	}
	if result.OrderID != "ord_1" {
		t.Fatalf("order 映射不符: %q", result.OrderID)
	}
	if _, err := adapter.GetTransaction(context.Background(), ""); !errors.Is(err, payment.ErrGatewayInvalidResponse) {
		t.Fatalf("空 ID 应报契约错误: %v", err)
	}
}

func TestCreemAdapterCreateFullRefundMapsPending(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	adapter := newTestAdapter(t, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST, doerFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/refunds" {
			t.Fatalf("path = %s，期望 /v1/refunds", req.URL.Path)
		}
		raw, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(raw, &gotBody)
		return jsonResponse(`{"id":"ref_1","mode":"test","status":"pending","transaction":"txn_1","amount":1389}`), nil
	}))

	result, err := adapter.CreateFullRefund(context.Background(), "txn_1")
	if err != nil {
		t.Fatalf("CreateFullRefund 返回错误: %v", err)
	}
	if gotBody["transaction_id"] != "txn_1" {
		t.Fatalf("退款请求体不符: %v", gotBody)
	}
	if result.Status != payment.RefundStatusPending || result.TransactionID != "txn_1" {
		t.Fatalf("退款映射不符: %+v", result)
	}
	if _, err := adapter.CreateFullRefund(context.Background(), ""); !errors.Is(err, payment.ErrGatewayInvalidResponse) {
		t.Fatalf("空 ID 应报契约错误: %v", err)
	}
}

func TestCreemAdapterGatewayErrorClassification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		doer    creemapi.Doer
		want    error
		checked bool
	}{
		{
			name: "rejected",
			doer: doerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusUnprocessableEntity, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(leakyBody))}, nil
			}),
			want: payment.ErrGatewayRejected, checked: true,
		},
		{
			name: "unauthorized",
			doer: doerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(leakyBody))}, nil
			}),
			want: payment.ErrGatewayUnauthorized, checked: true,
		},
		{
			name: "not_found",
			doer: doerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(leakyBody))}, nil
			}),
			want: payment.ErrGatewayNotFound, checked: true,
		},
		{
			name: "unavailable",
			doer: doerFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial tcp: connection refused")
			}),
			want: payment.ErrGatewayUnavailable, checked: true,
		},
		{
			name: "corrupt_json",
			doer: doerFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(`{"id":`), nil
			}),
			want: payment.ErrGatewayInvalidResponse, checked: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			adapter := newTestAdapter(t, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST, tc.doer)
			_, err := adapter.GetCheckout(context.Background(), "chk_1")
			if !errors.Is(err, tc.want) {
				t.Fatalf("errors.Is(%v, %v) = false", err, tc.want)
			}
			if strings.Contains(err.Error(), testAPIKey) || strings.Contains(err.Error(), leakyBody) {
				t.Fatalf("错误信息泄露密钥或响应体: %v", err)
			}
		})
	}
}
