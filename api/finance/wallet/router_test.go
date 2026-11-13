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
		{http.MethodPost, "/wallet/transactions/list", emptyJSONBody(), []ut.Header{auth, contentType}},
		{http.MethodPost, "/wallet/recharges", rechargeBody(), []ut.Header{auth, contentType}},
		{http.MethodPost, "/wallet/recharges/list", emptyJSONBody(), []ut.Header{auth, contentType}},
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

	for _, path := range []string{"/setting/wallet", "/init/services"} {
		w := ut.PerformRequest(h.Engine, http.MethodGet, path, nil,
			ut.Header{Key: "Authorization", Value: token})
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected %s to be absent from Finance, got %d", path, w.Code)
		}
	}
}

// TestFinanceLegacyQueryRoutesAreGone 保证只读接口已经全部 POST 化，
// 旧的 GET + query 形式不再可达。
func TestFinanceLegacyQueryRoutesAreGone(t *testing.T) {
	cfg := testConfig()
	h := newTestServer(t, cfg, &fakeWalletClient{}, nil)
	token := bearerToken(t, cfg, 7001)

	legacy := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/wallet/transactions"},
		{http.MethodGet, "/wallet/recharges"},
		{http.MethodGet, "/admin/wallets"},
		{http.MethodGet, "/admin/wallets/123"},
		{http.MethodGet, "/admin/wallets/123/transactions"},
		{http.MethodGet, "/admin/recharges"},
		{http.MethodGet, "/admin/redemption-codes"},
	}
	for _, tc := range legacy {
		w := ut.PerformRequest(h.Engine, tc.method, tc.path, nil,
			ut.Header{Key: "Authorization", Value: token})
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s %s should be removed, got %d", tc.method, tc.path, w.Code)
		}
	}
}

func emptyJSONBody() *ut.Body {
	const raw = `{}`
	return &ut.Body{Body: strings.NewReader(raw), Len: len(raw)}
}

func TestFinanceAdminRoutesRequireAuthorization(t *testing.T) {
	cfg := testConfig()
	h := newTestServer(t, cfg, &fakeWalletClient{}, nil)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/admin/wallets/list"},
		{http.MethodPost, "/admin/wallets/get"},
		{http.MethodPost, "/admin/transactions/list"},
		{http.MethodPost, "/admin/recharges/list"},
		{http.MethodGet, "/admin/recharges/123"},
		{http.MethodPost, "/admin/recharges/123/refresh"},
		{http.MethodPost, "/admin/recharges/123/refund"},
		{http.MethodPost, "/admin/recharges/123/checkout"},
		{http.MethodPost, "/admin/redemption-codes"},
		{http.MethodPost, "/admin/redemption-codes/list"},
		{http.MethodPost, "/admin/redemption-codes/123/status"},
		{http.MethodPost, "/admin/wallets/adjustments"},
	}

	for _, tc := range cases {
		// 无令牌：在 JWT 层就被拒绝。
		w := ut.PerformRequest(h.Engine, tc.method, tc.path, nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without token: expected 401, got %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}

		// 有令牌但没有策略：Casbin 层拒绝，不会进入 handler。
		w = ut.PerformRequest(h.Engine, tc.method, tc.path, nil,
			ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 7001)})
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s without policy: expected 403, got %d: %s", tc.method, tc.path, w.Code, w.Body.String())
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
