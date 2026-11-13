package creem

import (
	"context"
	"net/http"
	"testing"

	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
)

// TestCreemAdapterCreateCheckoutAcceptsProviderShapeWithoutRequestIDEcho 冻结
// Creem 真实响应形态：product 是字符串、**不返回 request_id**、pending 时没有 order。
// 该形态必须被适配器与 payment.ValidateCheckout 同时接受，否则订单会静默进入 REVIEW。
func TestCreemAdapterCreateCheckoutAcceptsProviderShapeWithoutRequestIDEcho(t *testing.T) {
	adapter := newTestAdapter(t, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST, doerFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"id":"ch_4cm3tmVsDexxRR9OGMjeKm","object":"checkout",
			"product":"` + testProdID + `","units":1,"custom_price":1389,"status":"pending",
			"checkout_url":"https://creem.io/test/checkout/` + testProdID + `/ch_4cm3tmVsDexxRR9OGMjeKm",
			"success_url":"` + testURL + `","metadata":{"recharge_id":"42"},"mode":"test"}`), nil
	}))

	result, err := adapter.CreateCheckout(context.Background(), payment.CheckoutRequest{
		CustomPrice: 1389,
		RequestID:   "cw95735531dc41de82bb390688",
		Metadata:    map[string]string{"recharge_id": "42"},
	})
	if err != nil {
		t.Fatalf("真实响应形态应通过适配器校验: %v", err)
	}
	if result.RequestID != "" {
		t.Fatalf("Provider 未回显 request_id，结果应为空，实际 %q", result.RequestID)
	}
	if result.URL == "" || result.ProductID != testProdID {
		t.Fatalf("跳转地址/产品解析错误: %+v", result)
	}
	if err := payment.ValidateCheckout(result, payment.CheckoutExpectation{
		Environment:    wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST,
		ProductID:      testProdID,
		ProviderAmount: 1389,
		RequestID:      "cw95735531dc41de82bb390688",
	}); err != nil {
		t.Fatalf("未回显 request_id 不得判为契约违例: %v", err)
	}
}
