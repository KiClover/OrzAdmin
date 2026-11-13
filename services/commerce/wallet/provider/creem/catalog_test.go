package creem

import (
	"context"
	"net/http"
	"strings"
	"testing"

	creemapi "github.com/KiClover/orzAdmin/internal/creem"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// productJSON 返回一份可售 Product 的最小 JSON，字段大小写由参数控制。
func productJSON(status, currency, successURL string) string {
	return `{"id":"` + testProdID + `","status":"` + status + `","currency":"` + currency +
		`","billing_type":"onetime","default_success_url":"` + successURL + `"}`
}

func newTestCatalog(t *testing.T, doer creemapi.Doer) *Catalog {
	t.Helper()
	catalog, err := NewCatalog(CatalogOptions{Doer: doer})
	if err != nil {
		t.Fatalf("NewCatalog 返回错误: %v", err)
	}
	return catalog
}

// TestProductSuccessURLAcceptsProviderCasing 冻结「Provider 可能返回大写」的兼容：
// Creem 实际返回 "USD"/"Active"，不能因此判定 Product 不可用。
func TestProductSuccessURLAcceptsProviderCasing(t *testing.T) {
	cases := []struct{ status, currency string }{
		{"active", "usd"},
		{"active", "USD"},
		{"Active", "USD"},
	}
	for _, tc := range cases {
		doer := doerFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(productJSON(tc.status, tc.currency, testURL)), nil
		})
		got, err := newTestCatalog(t, doer).ProductSuccessURL(context.Background(), testCredentials(wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST))
		if err != nil {
			t.Fatalf("status=%q currency=%q 应被接受: %v", tc.status, tc.currency, err)
		}
		if got != testURL {
			t.Fatalf("跳转地址 = %q，期望 %q", got, testURL)
		}
	}
}

// TestProductSuccessURLRejectsUnusableProduct 确认真正不可售的情况仍被拒绝。
func TestProductSuccessURLRejectsUnusableProduct(t *testing.T) {
	cases := []struct{ name, status, currency string }{
		{"inactive", "archived", "USD"},
		{"non_usd", "active", "EUR"},
		{"empty_currency", "active", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doer := doerFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(productJSON(tc.status, tc.currency, testURL)), nil
			})
			_, err := newTestCatalog(t, doer).ProductSuccessURL(context.Background(), testCredentials(wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST))
			if err == nil {
				t.Fatal("不可售 Product 应返回错误")
			}
			if strings.Contains(err.Error(), testAPIKey) {
				t.Fatalf("错误信息泄漏了 API Key: %v", err)
			}
		})
	}
}
