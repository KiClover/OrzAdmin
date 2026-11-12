// Package creem 提供 Creem 支付渠道的共享 HTTP 客户端。
//
// 本包承载钱包充值需要的 Creem 协议：保存配置前的只读查询（产品、Webhook 端点
// 与签名密钥），以及 Checkout 创建/查询、交易查询与全额退款。钱包业务状态机仍只
// 存在于 Commerce Wallet。
//
// API 根地址固定为 Creem 官方主机，不接受调用方传入：这样即使上层配置被改坏，
// 也不会把 x-api-key 发往非预期主机。
package creem

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Environment 是 Creem 的运行环境。
type Environment string

const (
	// EnvironmentTest 是 Creem 测试环境（test-api.creem.io）。
	EnvironmentTest Environment = "TEST"
	// EnvironmentProduction 是 Creem 生产环境（api.creem.io）。
	EnvironmentProduction Environment = "PRODUCTION"
)

const (
	// testBaseURL 是测试环境的固定 API 根地址。
	testBaseURL = "https://test-api.creem.io/v1"
	// productionBaseURL 是生产环境的固定 API 根地址。
	productionBaseURL = "https://api.creem.io/v1"
)

// BaseURL 返回该环境的固定 API 根地址；未知环境返回空字符串和错误。
func (e Environment) BaseURL() (string, error) {
	switch e {
	case EnvironmentTest:
		return testBaseURL, nil
	case EnvironmentProduction:
		return productionBaseURL, nil
	default:
		return "", fmt.Errorf("creem: unknown environment %q", string(e))
	}
}

const (
	// defaultTimeout 是 Options.Timeout 为零值时的单次请求超时。
	defaultTimeout = 10 * time.Second
	// defaultMaxResponseBytes 是 Options.MaxResponseBytes 为零值时的响应体上限。
	defaultMaxResponseBytes int64 = 1 << 20 // 1 MiB
)

// 这些 sentinel 是调用方唯一需要判定的错误类别，均用 errors.Is 判断。
// 错误信息里只允许出现安全操作名与状态码，不得出现响应体、API Key 或 Secret。
var (
	// ErrNotFound 表示 Creem 侧不存在该资源（HTTP 404）。
	ErrNotFound = errors.New("creem: resource not found")
	// ErrUnauthorized 表示 API Key 无效或无权访问（HTTP 401/403）。
	ErrUnauthorized = errors.New("creem: unauthorized")
	// ErrUnavailable 表示上游不可用或返回了未预期的状态码。
	ErrUnavailable = errors.New("creem: upstream unavailable")
	// ErrRejected 表示 Creem 明确拒绝了请求（4xx 业务错误，不含 401/403/404）。
	ErrRejected = errors.New("creem: request rejected")
	// ErrResponseTooLarge 表示响应体超过 Options.MaxResponseBytes。
	ErrResponseTooLarge = errors.New("creem: response too large")
	// ErrInvalidResponse 表示响应体不是本包能解析的 JSON。
	ErrInvalidResponse = errors.New("creem: invalid response")
)

// Doer 是 *http.Client 需要满足的最小接口，便于测试注入。
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Options 是 NewClient 的入参。
type Options struct {
	// Environment 决定 API 根地址，必须显式给出。
	Environment Environment
	// APIKey 是 Creem 的 x-api-key，不允许为空。
	APIKey string
	// Timeout 是单次请求超时，零值取默认 10s。
	Timeout time.Duration
	// MaxResponseBytes 是响应体上限，零值取默认 1 MiB。
	MaxResponseBytes int64
	// Doer 为 nil 时使用内部 *http.Client（超时取 Timeout）。
	Doer Doer
}

// Product 是 GET /v1/products/{id} 的响应。
//
// 字段与 Creem 官方 API 对齐，只保留钱包渠道校验需要的部分；官方响应里的
// mode、object、features、usage_prices 等字段会被 encoding/json 忽略。
type Product struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Status             string `json:"status"`       // active / inactive
	BillingType        string `json:"billing_type"` // onetime / recurring
	Currency           string `json:"currency"`     // usd
	TaxMode            string `json:"tax_mode"`     // inclusive / exclusive
	Price              int64  `json:"price"`        // 最小货币单位（美分）
	DefaultSuccessURL  string `json:"default_success_url"`
	BusinessNetPricing bool   `json:"business_net_pricing"`
}

// Webhook 是 GET /v1/webhooks/{id} 的响应。
//
// 官方响应同时带有 secret 字段，这里刻意不建模：签名密钥只能通过
// GetWebhookSecret 单独获取，避免调用方顺手把密钥当普通字段记录或打日志。
type Webhook struct {
	ID     string   `json:"id"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

// WebhookSecret 是 GET /v1/webhooks/{id}/secret 的响应。
type WebhookSecret struct {
	Secret string `json:"secret"`
}
