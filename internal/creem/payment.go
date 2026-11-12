package creem

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
)

// 本文件定义钱包充值所需的 Checkout、Transaction 与退款写入操作。
//
// 所有响应都保留官方字段原文：未知状态不会被映射成成功，而是原样交给上层判定。
// 数字金额一律是整数最小货币单位（美分），不使用浮点。

// EntityRef 是官方接口里既可返回字符串 ID、也可返回完整对象的引用。
//
// 典型例子是 checkout.product 与 checkout.order.product；调用方只需要 ID，
// 因此保留字符串形态，遇到对象形态时取其 id 字段。
type EntityRef struct {
	ID string
}

// UnmarshalJSON 兼容 "prod_x" 与 {"id":"prod_x", ...} 两种官方形态。
func (r *EntityRef) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		r.ID = ""
		return nil
	}
	if trimmed[0] == '"' {
		var id string
		if err := json.Unmarshal(trimmed, &id); err != nil {
			return err
		}
		r.ID = id
		return nil
	}
	var object struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return err
	}
	r.ID = object.ID
	return nil
}

// MarshalJSON 总是输出字符串 ID，保证写入请求时形态稳定。
func (r EntityRef) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.ID)
}

// CheckoutRequest 是 POST /v1/checkouts 的请求体。
//
// 字段刻意只保留钱包充值需要的部分：不发送 discount_code、affiliate_code、
// customer、custom_fields，也不接受调用方提供的其他跳转字段。
type CheckoutRequest struct {
	ProductID   string            `json:"product_id"`
	RequestID   string            `json:"request_id,omitempty"`
	Units       int64             `json:"units"`
	CustomPrice int64             `json:"custom_price,omitempty"`
	SuccessURL  string            `json:"success_url,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Checkout 是 checkout 对象的响应。
type Checkout struct {
	ID          string            `json:"id"`
	Mode        string            `json:"mode"`
	Object      string            `json:"object"`
	Status      string            `json:"status"`
	Product     EntityRef         `json:"product"`
	RequestID   string            `json:"request_id"`
	Units       int64             `json:"units"`
	CustomPrice int64             `json:"custom_price"`
	CheckoutURL string            `json:"checkout_url"`
	SuccessURL  string            `json:"success_url"`
	Order       *CheckoutOrder    `json:"order"`
	Metadata    map[string]string `json:"metadata"`
}

// CheckoutOrder 是 checkout.order 对象。
//
// 可空金额使用指针，以区分“官方没有返回”和“返回了 0”。
type CheckoutOrder struct {
	ID             string    `json:"id"`
	Mode           string    `json:"mode"`
	Object         string    `json:"object"`
	Product        EntityRef `json:"product"`
	Amount         int64     `json:"amount"`
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
	Type           string    `json:"type"`
	Transaction    EntityRef `json:"transaction"`
	Discount       EntityRef `json:"discount"`
	SubTotal       int64     `json:"sub_total"`
	TaxAmount      *int64    `json:"tax_amount"`
	DiscountAmount *int64    `json:"discount_amount"`
	AmountDue      *int64    `json:"amount_due"`
	AmountPaid     *int64    `json:"amount_paid"`
}

// Transaction 是 GET /v1/transactions 的响应。
type Transaction struct {
	ID             string    `json:"id"`
	Mode           string    `json:"mode"`
	Object         string    `json:"object"`
	Amount         int64     `json:"amount"`
	Currency       string    `json:"currency"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	CreatedAt      int64     `json:"created_at"`
	AmountPaid     *int64    `json:"amount_paid"`
	DiscountAmount *int64    `json:"discount_amount"`
	TaxAmount      *int64    `json:"tax_amount"`
	RefundedAmount *int64    `json:"refunded_amount"`
	Order          EntityRef `json:"order"`
	Subscription   EntityRef `json:"subscription"`
	Customer       EntityRef `json:"customer"`
	Description    string    `json:"description"`
}

// Refund 是 POST /v1/refunds 的响应。
type Refund struct {
	ID          string    `json:"id"`
	Mode        string    `json:"mode"`
	Object      string    `json:"object"`
	Status      string    `json:"status"`
	Transaction EntityRef `json:"transaction"`
	Amount      *int64    `json:"amount"`
	CreatedAt   int64     `json:"created_at"`
}

// CreateCheckout 创建一次 Checkout。
func (c *Client) CreateCheckout(ctx context.Context, request CheckoutRequest) (*Checkout, error) {
	if request.ProductID == "" {
		return nil, errors.New("creem: checkout product is empty")
	}
	if request.Units <= 0 {
		return nil, errors.New("creem: checkout units must be positive")
	}
	if request.CustomPrice < 0 {
		return nil, errors.New("creem: checkout price must not be negative")
	}
	var checkout Checkout
	if err := c.post(ctx, "create checkout", "/checkouts", request, &checkout); err != nil {
		return nil, err
	}
	return &checkout, nil
}

// GetCheckout 按 ID 查询 Checkout。
func (c *Client) GetCheckout(ctx context.Context, checkoutID string) (*Checkout, error) {
	if checkoutID == "" {
		return nil, errors.New("creem: checkout id is empty")
	}
	var checkout Checkout
	path := "/checkouts?" + url.Values{"checkout_id": {checkoutID}}.Encode()
	if err := c.get(ctx, "get checkout", path, &checkout); err != nil {
		return nil, err
	}
	return &checkout, nil
}

// GetTransaction 按 ID 查询支付交易。
func (c *Client) GetTransaction(ctx context.Context, transactionID string) (*Transaction, error) {
	if transactionID == "" {
		return nil, errors.New("creem: transaction id is empty")
	}
	var transaction Transaction
	path := "/transactions?" + url.Values{"transaction_id": {transactionID}}.Encode()
	if err := c.get(ctx, "get transaction", path, &transaction); err != nil {
		return nil, err
	}
	return &transaction, nil
}

// CreateFullRefund 对一笔支付交易发起全额退款。
//
// Creem 会按交易当前剩余可退金额自动处理，响应可能是 pending（异步确认）。
func (c *Client) CreateFullRefund(ctx context.Context, transactionID string) (*Refund, error) {
	if transactionID == "" {
		return nil, errors.New("creem: refund transaction id is empty")
	}
	body := struct {
		TransactionID EntityRef `json:"transaction_id"`
	}{TransactionID: EntityRef{ID: transactionID}}
	var refund Refund
	if err := c.post(ctx, "create refund", "/refunds", body, &refund); err != nil {
		return nil, err
	}
	return &refund, nil
}

// 编译期断言：EntityRef 必须能双向编解码。
var (
	_ json.Unmarshaler = (*EntityRef)(nil)
	_ json.Marshaler   = EntityRef{}
)
