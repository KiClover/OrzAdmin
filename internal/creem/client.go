package creem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

// apiKeyHeader 是 Creem 用于鉴权的请求头。
const apiKeyHeader = "x-api-key"

// Client 是 Creem API 的最小 HTTP 客户端，可并发使用。
//
// HTTP 传输统一走 resty；请求体大小、状态码与 JSON 解析结果在 get 里集中处理。
type Client struct {
	baseURL          string
	apiKey           string
	maxResponseBytes int64
	timeout          time.Duration
	doer             Doer

	once sync.Once
	http *resty.Client
}

// NewClient 校验配置并构造客户端。
//
// 环境非法或 APIKey 为空时返回错误；返回的 *Client 不可变，可被多个 goroutine
// 并发复用。
func NewClient(opts Options) (*Client, error) {
	baseURL, err := opts.Environment.BaseURL()
	if err != nil {
		return nil, err
	}
	// 粘贴配置时容易带进首尾空白，去掉后再判断是否为空。
	apiKey := strings.TrimSpace(opts.APIKey)
	if apiKey == "" {
		return nil, errors.New("creem: api key is empty")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxResponseBytes := opts.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	doer := opts.Doer
	return &Client{
		baseURL:          baseURL,
		apiKey:           apiKey,
		maxResponseBytes: maxResponseBytes,
		timeout:          timeout,
		doer:             doer,
	}, nil
}

// GetProduct 查询单个产品。
func (c *Client) GetProduct(ctx context.Context, productID string) (*Product, error) {
	var product Product
	if err := c.get(ctx, "get product", "/products/"+url.PathEscape(productID), &product); err != nil {
		return nil, err
	}
	return &product, nil
}

// GetWebhook 查询单个 Webhook 端点。响应里的 secret 不会被读取。
func (c *Client) GetWebhook(ctx context.Context, webhookID string) (*Webhook, error) {
	var webhook Webhook
	if err := c.get(ctx, "get webhook", "/webhooks/"+url.PathEscape(webhookID), &webhook); err != nil {
		return nil, err
	}
	return &webhook, nil
}

// GetWebhookSecret 查询 Webhook 的签名密钥，调用方必须避免把它写进日志或错误。
func (c *Client) GetWebhookSecret(ctx context.Context, webhookID string) (*WebhookSecret, error) {
	var secret WebhookSecret
	if err := c.get(ctx, "get webhook secret", "/webhooks/"+url.PathEscape(webhookID)+"/secret", &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

// restyClient 惰性构造 resty 客户端，使零值字段也能在测试里安全使用。
func (c *Client) restyClient() *resty.Client {
	c.once.Do(func() {
		timeout := c.timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		maxResponseBytes := c.maxResponseBytes
		if maxResponseBytes <= 0 {
			maxResponseBytes = defaultMaxResponseBytes
		}
		rc := resty.New().
			SetBaseURL(c.baseURL).
			SetTimeout(timeout).
			SetHeader(apiKeyHeader, c.apiKey).
			SetHeader("Accept", "application/json").
			// resty 用 int 表示上限；超过时返回 resty.ErrResponseBodyTooLarge。
			SetResponseBodyLimit(int(maxResponseBytes))
		if c.doer != nil {
			rc.SetTransport(doerTransport{doer: c.doer})
		}
		c.http = rc
	})
	return c.http
}

// doerTransport 把注入的 Doer 适配成 resty 需要的 RoundTripper。
type doerTransport struct {
	doer Doer
}

// RoundTrip 实现 http.RoundTripper。
func (t doerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.doer.Do(req)
}

// get 执行一次 GET 请求并处理状态码、响应体上限与 JSON 解析。
func (c *Client) get(ctx context.Context, op, path string, out any) error {
	return c.do(ctx, op, out, func(ctx context.Context, r *resty.Request) (*resty.Response, error) {
		return r.SetContext(ctx).Get(path)
	})
}

// post 执行一次 POST 请求；body 会被编码成精确的 JSON 字段。
func (c *Client) post(ctx context.Context, op, path string, body, out any) error {
	return c.do(ctx, op, out, func(ctx context.Context, r *resty.Request) (*resty.Response, error) {
		return r.SetContext(ctx).SetBody(body).Post(path)
	})
}

// do 执行请求并处理状态码、响应体上限与 JSON 解析。
//
// op 是写进错误信息的安全操作名；所有错误都以 sentinel 为底，便于 errors.Is
// 判定，且不含响应体、API Key 或 Secret。
func (c *Client) do(
	ctx context.Context,
	op string,
	out any,
	send func(ctx context.Context, r *resty.Request) (*resty.Response, error),
) error {
	resp, err := send(ctx, c.restyClient().R())
	if err != nil {
		if errors.Is(err, resty.ErrResponseBodyTooLarge) {
			return fmt.Errorf("%w: %s: response exceeds %d bytes", ErrResponseTooLarge, op, c.maxResponseBytes)
		}
		// 同时保留 ErrUnavailable 与原始错误（如 context.Canceled），两边的
		// errors.Is 都能命中。resty 的错误串只含方法与 URL，不含 key 或响应体。
		return fmt.Errorf("%w: %s: %w", ErrUnavailable, op, err)
	}
	if resp == nil {
		return fmt.Errorf("%w: %s: empty response", ErrUnavailable, op)
	}

	if status := resp.StatusCode(); status < 200 || status >= 300 {
		switch status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("%w: %s: status %d", ErrUnauthorized, op, status)
		case http.StatusNotFound:
			return fmt.Errorf("%w: %s: status %d", ErrNotFound, op, status)
		case http.StatusTooManyRequests:
			// 限流没有处理业务，重试是安全的，因此按“结果未知”而不是“明确拒绝”处理。
			return fmt.Errorf("%w: %s: status %d", ErrUnavailable, op, status)
		default:
			// 4xx 表示请求被明确拒绝（业务失败），5xx 及以上视为上游不可用；
			// 两类错误的处置完全不同：前者可以落 FAILED，后者只能转 REVIEW。
			if status >= 500 {
				return fmt.Errorf("%w: %s: status %d", ErrUnavailable, op, status)
			}
			return fmt.Errorf("%w: %s: status %d", ErrRejected, op, status)
		}
	}

	body := resp.Body()
	if int64(len(body)) > c.maxResponseBytes {
		return fmt.Errorf("%w: %s: response exceeds %d bytes", ErrResponseTooLarge, op, c.maxResponseBytes)
	}
	if err := json.Unmarshal(body, out); err != nil {
		// 不包装 json 错误，避免把解析片段带进错误信息。
		return fmt.Errorf("%w: %s: decode response", ErrInvalidResponse, op)
	}
	return nil
}

// 编译期断言：doerTransport 必须满足 http.RoundTripper。
var _ http.RoundTripper = doerTransport{}
