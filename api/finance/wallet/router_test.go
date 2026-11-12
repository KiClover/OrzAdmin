package wallet

import (
	"net/http"
	"strings"
	"testing"

	walletpb "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestFinanceWalletRoutesAreRegistered(t *testing.T) {
	cfg := testConfig()
	h := newTestServer(t, cfg, &fakeWalletClient{}, nil)
	auth := ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 7001)}
	contentType := ut.Header{Key: "Content-Type", Value: "application/json"}

	cases := []struct {
		method  string
		path    string
		body    *ut.Body
		headers []ut.Header
	}{
		{http.MethodGet, "/wallet", nil, []ut.Header{auth}},
		{http.MethodGet, "/wallet/transactions", nil, []ut.Header{auth}},
		{http.MethodPost, "/wallet/recharges", rechargeBody(), []ut.Header{auth, contentType}},
		{http.MethodGet, "/wallet/recharges", nil, []ut.Header{auth}},
		{http.MethodGet, "/wallet/recharges/123", nil, []ut.Header{auth}},
		{http.MethodPost, "/wallet/recharges/123/cancel", nil, []ut.Header{auth}},
		{http.MethodPost, "/wallet/redemption-codes/redeem", redeemBody(), []ut.Header{auth, contentType}},
	}

	for _, tc := range cases {
		w := ut.PerformRequest(h.Engine, tc.method, tc.path, tc.body, tc.headers...)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s: expected 200, got %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

func TestFinanceCallbackRouteNeedsNoToken(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{webhookResp: &walletpb.ProcessPaymentWebhookResp{Accepted: true}}
	h := newTestServer(t, cfg, fake, nil)

	raw := `{"eventType":"transaction.paid"}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/payment-callbacks/CREEM",
		&ut.Body{Body: strings.NewReader(raw), Len: len(raw)})
	if w.Code != http.StatusOK {
		t.Fatalf("expected callback without token to reach the handler, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFinanceExposesNoSettingRoutes(t *testing.T) {
	cfg := testConfig()
	h := newTestServer(t, cfg, &fakeWalletClient{}, nil)
	token := bearerToken(t, cfg, 7001)

	for _, path := range []string{"/setting/wallet", "/admin/wallets", "/init/services"} {
		w := ut.PerformRequest(h.Engine, http.MethodGet, path, nil,
			ut.Header{Key: "Authorization", Value: token})
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected %s to be absent from Finance, got %d", path, w.Code)
		}
	}
}

func TestFinanceHealthCheck(t *testing.T) {
	cfg := testConfig()
	h := newTestServer(t, cfg, &fakeWalletClient{}, nil)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d: %s", w.Code, w.Body.String())
	}
}
