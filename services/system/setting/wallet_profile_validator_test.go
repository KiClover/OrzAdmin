package setting

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/internal/creem"
)

// fakeDoer 按请求路径返回预设响应，用于验证远程校验逻辑。
type fakeDoer struct {
	responses map[string]fakeResponse
	seen      []string
}

type fakeResponse struct {
	status int
	body   string
}

func (d *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	d.seen = append(d.seen, req.URL.Path+"|"+req.Header.Get("x-api-key"))
	// Creem BaseURL 形如 https://test-api.creem.io/v1，测试按去掉版本前缀的路径匹配。
	path := strings.TrimPrefix(req.URL.Path, "/v1")
	resp, ok := d.responses[path]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}
	return &http.Response{
		StatusCode: resp.status,
		Body:       io.NopCloser(strings.NewReader(resp.body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

// validProductJSON 返回可通过校验的 Product 响应。
func validProductJSON() string {
	payload := map[string]any{
		"id":                   "prod_123",
		"name":                 "Wallet Top Up",
		"description":          "",
		"status":               "active",
		"billing_type":         "onetime",
		"currency":             "USD",
		"tax_mode":             "inclusive",
		"price":                500,
		"default_success_url":  "https://pay.example.com/success",
		"business_net_pricing": false,
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func TestCreemProfileValidatorAcceptsValidProfile(t *testing.T) {
	doer := &fakeDoer{responses: map[string]fakeResponse{
		"/products/prod_123":      {status: 200, body: validProductJSON()},
		"/webhooks/wh_123":        {status: 200, body: `{"id":"wh_123","url":"https://pay.example.com/creem","events":["checkout.completed"]}`},
		"/webhooks/wh_123/secret": {status: 200, body: `{"secret":"whsec_abc"}`},
	}}
	validator := &CreemProfileValidator{
		cfg: &conf.Config{Wallet: conf.WalletConf{
			ProductSuccessURLAllowedOrigins: []string{"https://pay.example.com"},
		}},
		newClient: creem.NewClient,
		doer:      doer,
	}

	err := validator.ValidateWalletProfile(context.Background(), WalletProfileValidationInput{
		Environment: "TEST",
		APIKey:      "creem_key_abc",
		ProductID:   "prod_123",
		WebhookID:   "wh_123",
	})
	if err != nil {
		t.Fatalf("合法 Profile 应通过校验: %v", err)
	}
	if len(doer.seen) != 3 {
		t.Fatalf("应依次查询 product/webhook/secret，实际 %v", doer.seen)
	}
	for _, seen := range doer.seen {
		if !strings.HasSuffix(seen, "|creem_key_abc") {
			t.Fatalf("请求未携带 x-api-key: %v", seen)
		}
	}
}

func TestCreemProfileValidatorRejectsBadProducts(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"inactive", func(m map[string]any) { m["status"] = "inactive" }},
		{"recurring", func(m map[string]any) { m["billing_type"] = "recurring" }},
		{"wrong_currency", func(m map[string]any) { m["currency"] = "eur" }},
		{"exclusive_tax", func(m map[string]any) { m["tax_mode"] = "exclusive" }},
		{"net_pricing", func(m map[string]any) { m["business_net_pricing"] = true }},
		{"missing_success_url", func(m map[string]any) { m["default_success_url"] = "" }},
		{"http_success_url", func(m map[string]any) { m["default_success_url"] = "http://pay.example.com/success" }},
		{"foreign_success_url", func(m map[string]any) { m["default_success_url"] = "https://evil.example.com/success" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal([]byte(validProductJSON()), &payload); err != nil {
				t.Fatalf("准备数据失败: %v", err)
			}
			tc.mutate(payload)
			raw, _ := json.Marshal(payload)

			validator := &CreemProfileValidator{
				cfg: &conf.Config{Wallet: conf.WalletConf{
					ProductSuccessURLAllowedOrigins: []string{"https://pay.example.com"},
				}},
				newClient: creem.NewClient,
				doer: &fakeDoer{responses: map[string]fakeResponse{
					"/products/prod_123": {status: 200, body: string(raw)},
				}},
			}
			err := validator.ValidateWalletProfile(context.Background(), WalletProfileValidationInput{
				Environment: "TEST",
				APIKey:      "creem_key_abc",
				ProductID:   "prod_123",
				WebhookID:   "wh_123",
			})
			if bizCode(err) != WalletSettingCodeProviderRejected {
				t.Fatalf("期望 ProviderRejected，实际 %v", err)
			}
		})
	}
}

func TestCreemProfileValidatorMapsUpstreamErrors(t *testing.T) {
	validProduct := validProductJSON()
	cases := []struct {
		name     string
		status   int
		body     string
		wantCode int32
	}{
		{"not_found", 404, `{"error":"not found"}`, WalletSettingCodeProviderRejected},
		{"unauthorized", 401, `{"error":"bad key"}`, WalletSettingCodeProviderRejected},
		{"server_error", 500, `{"error":"boom"}`, WalletSettingCodeUnavailable},
		{"garbage", 200, `not-json`, WalletSettingCodeUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			validator := &CreemProfileValidator{
				cfg: &conf.Config{Wallet: conf.WalletConf{
					ProductSuccessURLAllowedOrigins: []string{"https://pay.example.com"},
				}},
				newClient: creem.NewClient,
				doer: &fakeDoer{responses: map[string]fakeResponse{
					"/products/prod_123": {status: tc.status, body: tc.body},
				}},
			}
			err := validator.ValidateWalletProfile(context.Background(), WalletProfileValidationInput{
				Environment: "TEST",
				APIKey:      "creem_key_abc",
				ProductID:   "prod_123",
				WebhookID:   "wh_123",
			})
			if bizCode(err) != tc.wantCode {
				t.Fatalf("错误码 = %d，期望 %d（err=%v）", bizCode(err), tc.wantCode, err)
			}
			if strings.Contains(err.Error(), "creem_key_abc") || strings.Contains(err.Error(), "not-json") {
				t.Fatalf("错误信息泄漏了密钥或上游响应体: %v", err)
			}
		})
	}
	_ = validProduct
}

func TestCreemProfileValidatorRejectsDuplicateWebhookSecret(t *testing.T) {
	validator := &CreemProfileValidator{
		cfg: &conf.Config{Wallet: conf.WalletConf{
			ProductSuccessURLAllowedOrigins: []string{"https://pay.example.com"},
		}},
		newClient: creem.NewClient,
		doer: &fakeDoer{responses: map[string]fakeResponse{
			"/products/prod_123":      {status: 200, body: validProductJSON()},
			"/webhooks/wh_123":        {status: 200, body: `{"id":"wh_123"}`},
			"/webhooks/wh_123/secret": {status: 200, body: `{"secret":"whsec_abc"}`},
		}},
	}
	err := validator.ValidateWalletProfile(context.Background(), WalletProfileValidationInput{
		Environment:              "PRODUCTION",
		APIKey:                   "creem_key_abc",
		ProductID:                "prod_123",
		WebhookID:                "wh_123",
		OtherWebhookFingerprints: []string{FingerprintSecret("whsec_abc")},
	})
	if bizCode(err) != WalletSettingCodeProviderRejected {
		t.Fatalf("重复 Secret 应被拒绝，实际 %v", err)
	}
	if strings.Contains(err.Error(), "whsec_abc") {
		t.Fatalf("错误信息泄漏了 Secret: %v", err)
	}
}

func TestCreemProfileValidatorChecksPreviousKeyOnRotation(t *testing.T) {
	cases := []struct {
		name      string
		previousR *fakeResponse
		wantCode  int32
	}{
		{"previous_key_works", &fakeResponse{status: 200, body: validProductJSON()}, 0},
		{"previous_key_rejected", &fakeResponse{status: 401, body: `{"error":"bad"}`}, WalletSettingCodeProviderRejected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			responses := map[string]fakeResponse{
				"/products/prod_123":      {status: 200, body: validProductJSON()},
				"/webhooks/wh_123":        {status: 200, body: `{"id":"wh_123"}`},
				"/webhooks/wh_123/secret": {status: 200, body: `{"secret":"whsec_abc"}`},
				"/products/prod_old":      *tc.previousR,
			}
			validator := &CreemProfileValidator{
				cfg: &conf.Config{Wallet: conf.WalletConf{
					ProductSuccessURLAllowedOrigins: []string{"https://pay.example.com"},
				}},
				newClient: creem.NewClient,
				doer:      &fakeDoer{responses: responses},
			}
			err := validator.ValidateWalletProfile(context.Background(), WalletProfileValidationInput{
				Environment:       "TEST",
				APIKey:            "creem_key_new",
				ProductID:         "prod_123",
				WebhookID:         "wh_123",
				PreviousAPIKey:    "creem_key_old",
				PreviousProductID: "prod_old",
			})
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatalf("旧 Key 仍可读取时应通过: %v", err)
				}
				return
			}
			if bizCode(err) != tc.wantCode {
				t.Fatalf("错误码 = %d，期望 %d（err=%v）", bizCode(err), tc.wantCode, err)
			}
		})
	}
}

func TestCreemProfileValidatorRejectsInvalidInput(t *testing.T) {
	validator := &CreemProfileValidator{cfg: &conf.Config{}, newClient: creem.NewClient}

	cases := []WalletProfileValidationInput{
		{Environment: "STAGING", APIKey: "k", ProductID: "p", WebhookID: "w"},
		{Environment: "TEST", APIKey: "", ProductID: "p", WebhookID: "w"},
		{Environment: "TEST", APIKey: "k", ProductID: "", WebhookID: "w"},
		{Environment: "TEST", APIKey: "k", ProductID: "p", WebhookID: ""},
	}
	for _, input := range cases {
		err := validator.ValidateWalletProfile(context.Background(), input)
		if bizCode(err) != WalletSettingCodeInvalidArgument {
			t.Fatalf("%+v 应返回参数错误，实际 %v", input, err)
		}
	}
}

func TestCreemEnvironmentMapping(t *testing.T) {
	if env, err := creemEnvironment("test"); err != nil || env != creem.EnvironmentTest {
		t.Fatalf("TEST 映射失败: %v %v", env, err)
	}
	if env, err := creemEnvironment(" production "); err != nil || env != creem.EnvironmentProduction {
		t.Fatalf("PRODUCTION 映射失败: %v %v", env, err)
	}
	if _, err := creemEnvironment("sandbox"); err == nil {
		t.Fatalf("未知环境应报错")
	}
}
