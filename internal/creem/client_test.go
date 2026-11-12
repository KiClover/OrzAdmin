package creem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	// testAPIKey 是假 API Key，任何错误信息里都不允许出现。
	testAPIKey = "creem_test_key_must_never_leak"
	// testSecret 是假 Webhook 密钥，任何错误信息里都不允许出现。
	testSecret = "whsec_must_never_leak"
	// leakyBody 出现在被拒绝的响应体里，用于断言错误信息不夹带响应内容。
	leakyBody = "LEAKY_BODY_MARKER_4f9c"
)

// doerFunc 让测试用普通函数实现 Doer。
type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

// jsonResponse 构造一个 200 响应，供注入的 Doer 使用。
func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// fixedDoer 返回一个总是给出固定响应的 Doer。
func fixedDoer(resp *http.Response) Doer {
	return doerFunc(func(*http.Request) (*http.Response, error) { return resp, nil })
}

// stubDoer 是可比较的 Doer 实现，用来断言注入的 Doer 被原样保留。
// doerFunc 是函数类型，不能直接用 == 比较（会 panic）。
type stubDoer struct {
	body string
}

func (d *stubDoer) Do(*http.Request) (*http.Response, error) { return jsonResponse(d.body), nil }

// newTestClient 起一个 httptest 服务端，并把未导出的 baseURL 指向它。
// 生产代码的 baseURL 固定为官方主机，这里借助同包白盒测试才能指向本地服务端。
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{
		baseURL:          srv.URL,
		apiKey:           testAPIKey,
		maxResponseBytes: defaultMaxResponseBytes,
		doer:             srv.Client(),
	}
}

func TestEnvironmentBaseURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		env     Environment
		want    string
		wantErr bool
	}{
		{name: "test", env: EnvironmentTest, want: "https://test-api.creem.io/v1"},
		{name: "production", env: EnvironmentProduction, want: "https://api.creem.io/v1"},
		{name: "empty", env: Environment(""), wantErr: true},
		{name: "unknown", env: Environment("STAGING"), wantErr: true},
		{name: "lowercase", env: Environment("test"), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.env.BaseURL()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("BaseURL(%q) 应该返回错误", string(tc.env))
				}
				if got != "" {
					t.Fatalf("BaseURL(%q) 出错时应返回空字符串，实际 %q", string(tc.env), got)
				}
				return
			}
			if err != nil {
				t.Fatalf("BaseURL(%q) 返回错误: %v", string(tc.env), err)
			}
			if got != tc.want {
				t.Fatalf("BaseURL(%q) = %q，期望 %q", string(tc.env), got, tc.want)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	t.Run("默认值", func(t *testing.T) {
		t.Parallel()
		client, err := NewClient(Options{Environment: EnvironmentTest, APIKey: testAPIKey})
		if err != nil {
			t.Fatalf("NewClient 返回错误: %v", err)
		}
		if client.baseURL != testBaseURL {
			t.Fatalf("baseURL = %q，期望 %q", client.baseURL, testBaseURL)
		}
		if client.maxResponseBytes != defaultMaxResponseBytes {
			t.Fatalf("maxResponseBytes = %d，期望 %d", client.maxResponseBytes, defaultMaxResponseBytes)
		}
		if client.timeout != 10*time.Second {
			t.Fatalf("默认超时 = %v，期望 10s", client.timeout)
		}
		if client.doer != nil {
			t.Fatalf("未注入 Doer 时应使用 resty 默认传输，实际 %T", client.doer)
		}
		if got := client.restyClient().GetClient().Timeout; got != 10*time.Second {
			t.Fatalf("resty 客户端超时 = %v，期望 10s", got)
		}
	})

	t.Run("自定义值", func(t *testing.T) {
		t.Parallel()
		injected := &stubDoer{body: `{}`}
		client, err := NewClient(Options{
			Environment:      EnvironmentProduction,
			APIKey:           testAPIKey,
			Timeout:          3 * time.Second,
			MaxResponseBytes: 128,
			Doer:             injected,
		})
		if err != nil {
			t.Fatalf("NewClient 返回错误: %v", err)
		}
		if client.baseURL != productionBaseURL {
			t.Fatalf("baseURL = %q，期望 %q", client.baseURL, productionBaseURL)
		}
		if client.maxResponseBytes != 128 {
			t.Fatalf("maxResponseBytes = %d，期望 128", client.maxResponseBytes)
		}
		if client.doer != injected {
			t.Fatal("注入的 Doer 未被采用")
		}
	})

	t.Run("空 API Key", func(t *testing.T) {
		t.Parallel()
		for _, key := range []string{"", "   ", "\t\n"} {
			client, err := NewClient(Options{Environment: EnvironmentTest, APIKey: key})
			if err == nil {
				t.Fatalf("APIKey %q 应该返回错误", key)
			}
			if client != nil {
				t.Fatalf("APIKey %q 出错时应返回 nil client", key)
			}
			if strings.Contains(err.Error(), testAPIKey) {
				t.Fatalf("错误信息不应包含 API Key: %q", err)
			}
		}
	})

	t.Run("非法环境", func(t *testing.T) {
		t.Parallel()
		client, err := NewClient(Options{Environment: Environment("DEV"), APIKey: testAPIKey})
		if err == nil {
			t.Fatal("非法环境应该返回错误")
		}
		if client != nil {
			t.Fatal("非法环境出错时应返回 nil client")
		}
	})
}

func TestRequestShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		env     Environment
		call    func(*Client) error
		wantURL string
	}{
		{
			name: "product test",
			env:  EnvironmentTest,
			call: func(c *Client) error {
				_, err := c.GetProduct(context.Background(), "prod_1")
				return err
			},
			wantURL: testBaseURL + "/products/prod_1",
		},
		{
			name: "product production",
			env:  EnvironmentProduction,
			call: func(c *Client) error {
				_, err := c.GetProduct(context.Background(), "prod_1")
				return err
			},
			wantURL: productionBaseURL + "/products/prod_1",
		},
		{
			name: "webhook",
			env:  EnvironmentTest,
			call: func(c *Client) error {
				_, err := c.GetWebhook(context.Background(), "wh_1")
				return err
			},
			wantURL: testBaseURL + "/webhooks/wh_1",
		},
		{
			name: "webhook secret",
			env:  EnvironmentTest,
			call: func(c *Client) error {
				_, err := c.GetWebhookSecret(context.Background(), "wh_1")
				return err
			},
			wantURL: testBaseURL + "/webhooks/wh_1/secret",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got *http.Request
			client, err := NewClient(Options{
				Environment: tc.env,
				APIKey:      testAPIKey,
				Doer: doerFunc(func(req *http.Request) (*http.Response, error) {
					got = req
					return jsonResponse(`{}`), nil
				}),
			})
			if err != nil {
				t.Fatalf("NewClient 返回错误: %v", err)
			}
			if err := tc.call(client); err != nil {
				t.Fatalf("请求返回错误: %v", err)
			}
			if got == nil {
				t.Fatal("注入的 Doer 未被调用")
			}
			if got.Method != http.MethodGet {
				t.Fatalf("method = %q，期望 GET", got.Method)
			}
			if got.URL.String() != tc.wantURL {
				t.Fatalf("URL = %q，期望 %q", got.URL.String(), tc.wantURL)
			}
			if key := got.Header.Get(apiKeyHeader); key != testAPIKey {
				t.Fatalf("x-api-key = %q，期望测试 Key", key)
			}
			if accept := got.Header.Get("Accept"); accept != "application/json" {
				t.Fatalf("Accept = %q，期望 application/json", accept)
			}
		})
	}
}

func TestRequestPathEscaping(t *testing.T) {
	t.Parallel()

	var got *http.Request
	client, err := NewClient(Options{
		Environment: EnvironmentTest,
		APIKey:      testAPIKey,
		Doer: doerFunc(func(req *http.Request) (*http.Response, error) {
			got = req
			return jsonResponse(`{}`), nil
		}),
	})
	if err != nil {
		t.Fatalf("NewClient 返回错误: %v", err)
	}
	// 恶意 ID 不能改变主机，也不能逃出 /v1/products/ 前缀。
	if _, err := client.GetProduct(context.Background(), "../../admin"); err != nil {
		t.Fatalf("请求返回错误: %v", err)
	}
	if got.URL.Host != "test-api.creem.io" {
		t.Fatalf("host = %q，期望 test-api.creem.io", got.URL.Host)
	}
	if path := got.URL.EscapedPath(); !strings.HasPrefix(path, "/v1/products/") {
		t.Fatalf("path = %q，应以 /v1/products/ 开头", path)
	}
	if !strings.Contains(got.URL.EscapedPath(), "%2F") {
		t.Fatalf("path = %q，恶意斜杠应被转义", got.URL.EscapedPath())
	}
}

func TestGetProduct(t *testing.T) {
	t.Parallel()

	const body = `{
		"ID": "prod_1",
		"mode": "test",
		"object": "product",
		"NAME": "Starter",
		"description": "starter plan",
		"status": "active",
		"billing_type": "onetime",
		"currency": "USD",
		"tax_mode": "inclusive",
		"price": 499,
		"default_success_url": "https://example.com/ok",
		"business_net_pricing": true,
		"features": [{"id": "feat_1"}],
		"unknown_field": {"nested": [1, 2, 3]}
	}`

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products/prod_1" {
			t.Errorf("path = %q，期望 /products/prod_1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	product, err := client.GetProduct(context.Background(), "prod_1")
	if err != nil {
		t.Fatalf("GetProduct 返回错误: %v", err)
	}
	want := Product{
		ID:                 "prod_1",
		Name:               "Starter",
		Description:        "starter plan",
		Status:             "active",
		BillingType:        "onetime",
		Currency:           "USD",
		TaxMode:            "inclusive",
		Price:              499,
		DefaultSuccessURL:  "https://example.com/ok",
		BusinessNetPricing: true,
	}
	if *product != want {
		t.Fatalf("product = %+v，期望 %+v", *product, want)
	}
}

func TestGetWebhook(t *testing.T) {
	t.Parallel()

	// 官方响应里带 secret，本包刻意不把它放进 Webhook。
	const body = `{
		"id": "wh_1",
		"object": "webhook",
		"url": "https://example.com/hooks/creem",
		"events": ["checkout.completed", "subscription.paid"],
		"status": "enabled",
		"secret": "` + testSecret + `",
		"delivery_mode": "http"
	}`

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/webhooks/wh_1" {
			t.Errorf("path = %q，期望 /webhooks/wh_1", r.URL.Path)
		}
		_, _ = io.WriteString(w, body)
	})

	webhook, err := client.GetWebhook(context.Background(), "wh_1")
	if err != nil {
		t.Fatalf("GetWebhook 返回错误: %v", err)
	}
	if webhook.ID != "wh_1" {
		t.Fatalf("ID = %q，期望 wh_1", webhook.ID)
	}
	if webhook.URL != "https://example.com/hooks/creem" {
		t.Fatalf("URL = %q", webhook.URL)
	}
	wantEvents := []string{"checkout.completed", "subscription.paid"}
	if !reflect.DeepEqual(webhook.Events, wantEvents) {
		t.Fatalf("Events = %v，期望 %v", webhook.Events, wantEvents)
	}
}

func TestGetWebhookSecret(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/webhooks/wh_1/secret" {
			t.Errorf("path = %q，期望 /webhooks/wh_1/secret", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"secret":"`+testSecret+`"}`)
	})

	secret, err := client.GetWebhookSecret(context.Background(), "wh_1")
	if err != nil {
		t.Fatalf("GetWebhookSecret 返回错误: %v", err)
	}
	if secret.Secret != testSecret {
		t.Fatalf("Secret = %q，期望测试 Secret", secret.Secret)
	}
}

// TestResponseTypesAreMinimal 锁定响应结构体只有契约规定的字段，防止后续改动
// 顺手把 Webhook 的 secret 之类的字段加回来。
func TestResponseTypesAreMinimal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
		want  int
	}{
		{name: "Product", value: Product{}, want: 10},
		{name: "Webhook", value: Webhook{}, want: 3},
		{name: "WebhookSecret", value: WebhookSecret{}, want: 1},
	}
	for _, tc := range cases {
		typ := reflect.TypeOf(tc.value)
		if got := typ.NumField(); got != tc.want {
			t.Fatalf("%s 有 %d 个字段，期望 %d", tc.name, got, tc.want)
		}
	}
}

func TestStatusMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		want   error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, want: ErrUnauthorized},
		{name: "forbidden", status: http.StatusForbidden, want: ErrUnauthorized},
		{name: "not found", status: http.StatusNotFound, want: ErrNotFound},
		{name: "bad request", status: http.StatusBadRequest, want: ErrRejected},
		{name: "unprocessable", status: http.StatusUnprocessableEntity, want: ErrRejected},
		{name: "rate limited", status: http.StatusTooManyRequests, want: ErrUnavailable},
		{name: "server error", status: http.StatusInternalServerError, want: ErrUnavailable},
		{name: "bad gateway", status: http.StatusBadGateway, want: ErrUnavailable},
		{name: "unavailable", status: http.StatusServiceUnavailable, want: ErrUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"message":"`+leakyBody+`","secret":"`+testSecret+`"}`)
			})
			_, err := client.GetProduct(context.Background(), "prod_1")
			if err == nil {
				t.Fatalf("状态码 %d 应该返回错误", tc.status)
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("状态码 %d 错误 = %v，期望 errors.Is(..., %v)", tc.status, err, tc.want)
			}
			// 其它 sentinel 不应被误命中。
			for _, other := range []error{ErrNotFound, ErrUnauthorized, ErrUnavailable, ErrResponseTooLarge, ErrInvalidResponse} {
				if other == tc.want {
					continue
				}
				if errors.Is(err, other) {
					t.Fatalf("状态码 %d 错误同时命中了 %v", tc.status, other)
				}
			}
			if !strings.Contains(err.Error(), strconv.Itoa(tc.status)) {
				t.Fatalf("错误信息应包含状态码 %d: %v", tc.status, err)
			}
		})
	}
}

func TestInvalidJSON(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":"prod_1","name":"`+leakyBody)
	})
	_, err := client.GetProduct(context.Background(), "prod_1")
	if err == nil {
		t.Fatal("非法 JSON 应该返回错误")
	}
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("错误 = %v，期望 ErrInvalidResponse", err)
	}
	if strings.Contains(err.Error(), leakyBody) {
		t.Fatalf("错误信息不应包含响应体: %v", err)
	}
}

func TestResponseTooLarge(t *testing.T) {
	t.Parallel()

	const body = `{"id":"prod_1","name":"ok"}`

	t.Run("恰好等于上限", func(t *testing.T) {
		t.Parallel()
		client := &Client{
			baseURL:          "https://example.invalid/v1",
			apiKey:           testAPIKey,
			maxResponseBytes: int64(len(body)),
			doer:             fixedDoer(jsonResponse(body)),
		}
		product, err := client.GetProduct(context.Background(), "prod_1")
		if err != nil {
			t.Fatalf("等于上限的响应不应报错: %v", err)
		}
		if product.ID != "prod_1" {
			t.Fatalf("ID = %q，期望 prod_1", product.ID)
		}
	})

	t.Run("超过一个字节", func(t *testing.T) {
		t.Parallel()
		client := &Client{
			baseURL:          "https://example.invalid/v1",
			apiKey:           testAPIKey,
			maxResponseBytes: int64(len(body)) - 1,
			doer:             fixedDoer(jsonResponse(body)),
		}
		_, err := client.GetProduct(context.Background(), "prod_1")
		if !errors.Is(err, ErrResponseTooLarge) {
			t.Fatalf("错误 = %v，期望 ErrResponseTooLarge", err)
		}
		if strings.Contains(err.Error(), body) || strings.Contains(err.Error(), "prod_1") {
			t.Fatalf("错误信息不应包含响应体: %v", err)
		}
	})
}

// TestErrorHygiene 断言所有错误路径都不把响应体、API Key 或 Secret 带进错误信息。
func TestErrorHygiene(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		handler http.HandlerFunc
		patch   func(*Client)
	}{
		{
			name: "unauthorized",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"message":"`+leakyBody+`"}`)
			},
		},
		{
			name: "not found",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"message":"`+leakyBody+`"}`)
			},
		},
		{
			name: "server error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"message":"`+leakyBody+`"}`)
			},
		},
		{
			name: "invalid json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{"secret":"`+testSecret+`","x":`+leakyBody)
			},
		},
		{
			name: "too large",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{"message":"`+strings.Repeat(leakyBody, 8)+`"}`)
			},
			patch: func(c *Client) { c.maxResponseBytes = 16 },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, tc.handler)
			if tc.patch != nil {
				tc.patch(client)
			}
			_, err := client.GetWebhookSecret(context.Background(), "wh_1")
			if err == nil {
				t.Fatal("应该返回错误")
			}
			msg := err.Error()
			if strings.Contains(msg, leakyBody) {
				t.Fatalf("错误信息包含响应体: %v", err)
			}
			if strings.Contains(msg, testAPIKey) {
				t.Fatalf("错误信息包含 API Key: %v", err)
			}
			if strings.Contains(msg, testSecret) {
				t.Fatalf("错误信息包含 Secret: %v", err)
			}
		})
	}
}

func TestContextCanceled(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":"prod_1"}`)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.GetProduct(ctx, "prod_1")
	if err == nil {
		t.Fatal("已取消的 context 应该返回错误")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("错误 = %v，期望 errors.Is(..., context.Canceled)", err)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("错误 = %v，期望同时命中 ErrUnavailable", err)
	}
}

func TestDoerError(t *testing.T) {
	t.Parallel()

	client := &Client{
		baseURL:          "https://example.invalid/v1",
		apiKey:           testAPIKey,
		maxResponseBytes: defaultMaxResponseBytes,
		doer: doerFunc(func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("transport exploded: %w", context.DeadlineExceeded)
		}),
	}

	_, err := client.GetProduct(context.Background(), "prod_1")
	if err == nil {
		t.Fatal("Doer 出错时应该返回错误")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("错误 = %v，期望保留 context.DeadlineExceeded", err)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("错误 = %v，期望命中 ErrUnavailable", err)
	}
}

func TestDoerReturnsNilResponse(t *testing.T) {
	t.Parallel()

	client := &Client{
		baseURL:          "https://example.invalid/v1",
		apiKey:           testAPIKey,
		maxResponseBytes: defaultMaxResponseBytes,
		doer:             doerFunc(func(*http.Request) (*http.Response, error) { return nil, nil }),
	}

	_, err := client.GetProduct(context.Background(), "prod_1")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("错误 = %v，期望 ErrUnavailable", err)
	}
}
