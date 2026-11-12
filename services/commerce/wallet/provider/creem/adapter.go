// Package creem 是 Creem 支付渠道对钱包 PaymentGateway 的适配实现。
//
// 职责边界：
//   - 只做请求构造、响应映射与结构校验，不决定充值状态机；
//   - 渠道错误按“明确失败 / 无法确认 / 契约违例”分类，供上层决定落 FAILED 还是 REVIEW；
//   - 金额与币种字段一一对应，不把 USD 金额当成 CNY 使用。
package creem

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	creemapi "github.com/KiClover/orzAdmin/internal/creem"
	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
)

// Options 是构造适配器的入参。
type Options struct {
	// Credentials 是已通过远程校验的 Profile 凭据。
	Credentials payment.GatewayCredentials
	// Timeout 为 0 时使用 walletpolicy.ProviderRequestTimeout。
	Timeout time.Duration
	// MaxResponseBytes 为 0 时使用 walletpolicy.ProviderMaxResponseBytes。
	MaxResponseBytes int64
	// Doer 仅用于测试注入；为 nil 时走真实 HTTP 客户端。
	Doer creemapi.Doer
}

// Adapter 把一个固定的 Creem Profile 适配成钱包支付网关。
type Adapter struct {
	client      *creemapi.Client
	productID   string
	successURL  string
	expectedMod string
}

// New 校验凭据并构造适配器。
func New(options Options) (*Adapter, error) {
	environment, mode, err := environmentOf(options.Credentials.Environment)
	if err != nil {
		return nil, err
	}
	if options.Credentials.ProductID == "" {
		return nil, errors.New("creem adapter: product id is empty")
	}
	// 查询类网关（回调核验、管理员刷新、退款）不需要成功跳转地址；
	// 只有 CreateCheckout 强制要求它，且地址必须来自已校验 Product。
	successURL := ""
	if options.Credentials.SuccessURL != "" {
		successURL, err = parseSuccessURL(options.Credentials.SuccessURL)
		if err != nil {
			return nil, err
		}
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = walletpolicy.ProviderRequestTimeout
	}
	maxResponseBytes := options.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = walletpolicy.ProviderMaxResponseBytes
	}
	client, err := creemapi.NewClient(creemapi.Options{
		Environment:      environment,
		APIKey:           options.Credentials.APIKey,
		Timeout:          timeout,
		MaxResponseBytes: maxResponseBytes,
		Doer:             options.Doer,
	})
	if err != nil {
		return nil, err
	}
	return &Adapter{
		client:      client,
		productID:   options.Credentials.ProductID,
		successURL:  successURL,
		expectedMod: mode,
	}, nil
}

// CreateCheckout 创建 Checkout 并校验响应的环境、Product 与请求号。
func (a *Adapter) CreateCheckout(ctx context.Context, request payment.CheckoutRequest) (payment.CheckoutResult, error) {
	if request.CustomPrice <= 0 {
		return payment.CheckoutResult{}, fmt.Errorf("%w: provider amount must be positive", payment.ErrGatewayInvalidResponse)
	}
	if request.RequestID == "" {
		return payment.CheckoutResult{}, fmt.Errorf("%w: request id is empty", payment.ErrGatewayInvalidResponse)
	}
	if a.successURL == "" {
		return payment.CheckoutResult{}, fmt.Errorf("%w: success url is not configured", payment.ErrGatewayInvalidResponse)
	}
	checkout, err := a.client.CreateCheckout(ctx, creemapi.CheckoutRequest{
		ProductID:   a.productID,
		RequestID:   request.RequestID,
		Units:       1,
		CustomPrice: request.CustomPrice,
		SuccessURL:  a.successURL,
		Metadata:    cloneMetadata(request.Metadata),
	})
	if err != nil {
		return payment.CheckoutResult{}, mapError("create checkout", err)
	}
	result := checkoutResult(checkout)
	if err := a.verifyCheckout(result, request); err != nil {
		return payment.CheckoutResult{}, err
	}
	return result, nil
}

// GetCheckout 查询 Checkout；未知状态原样保留为 Unknown。
func (a *Adapter) GetCheckout(ctx context.Context, checkoutID string) (payment.CheckoutResult, error) {
	if checkoutID == "" {
		return payment.CheckoutResult{}, fmt.Errorf("%w: checkout id is empty", payment.ErrGatewayInvalidResponse)
	}
	checkout, err := a.client.GetCheckout(ctx, checkoutID)
	if err != nil {
		return payment.CheckoutResult{}, mapError("get checkout", err)
	}
	result := checkoutResult(checkout)
	if err := a.verifyModeOnly(result.Mode); err != nil {
		return payment.CheckoutResult{}, err
	}
	return result, nil
}

// GetTransaction 查询支付交易；未知状态原样保留为 Unknown。
func (a *Adapter) GetTransaction(ctx context.Context, transactionID string) (payment.TransactionResult, error) {
	if transactionID == "" {
		return payment.TransactionResult{}, fmt.Errorf("%w: transaction id is empty", payment.ErrGatewayInvalidResponse)
	}
	transaction, err := a.client.GetTransaction(ctx, transactionID)
	if err != nil {
		return payment.TransactionResult{}, mapError("get transaction", err)
	}
	if err := a.verifyModeOnly(transaction.Mode); err != nil {
		return payment.TransactionResult{}, err
	}
	return transactionResult(transaction), nil
}

// CreateFullRefund 发起全额退款；pending 会原样保留，由上层等待收敛。
func (a *Adapter) CreateFullRefund(ctx context.Context, transactionID string) (payment.RefundResult, error) {
	if transactionID == "" {
		return payment.RefundResult{}, fmt.Errorf("%w: transaction id is empty", payment.ErrGatewayInvalidResponse)
	}
	refund, err := a.client.CreateFullRefund(ctx, transactionID)
	if err != nil {
		return payment.RefundResult{}, mapError("create refund", err)
	}
	if err := a.verifyModeOnly(refund.Mode); err != nil {
		return payment.RefundResult{}, err
	}
	return payment.RefundResult{
		ID:            refund.ID,
		Mode:          refund.Mode,
		Status:        payment.ParseRefundStatus(refund.Status),
		RawStatus:     refund.Status,
		TransactionID: refund.Transaction.ID,
		Amount:        refund.Amount,
	}, nil
}

// verifyCheckout 校验响应对应的就是本次冻结报价。
func (a *Adapter) verifyCheckout(result payment.CheckoutResult, request payment.CheckoutRequest) error {
	if err := a.verifyModeOnly(result.Mode); err != nil {
		return err
	}
	if result.ProductID != a.productID {
		return fmt.Errorf("%w: checkout product does not match the configured product", payment.ErrGatewayInvalidResponse)
	}
	if result.Units != 1 {
		return fmt.Errorf("%w: checkout units must be 1", payment.ErrGatewayInvalidResponse)
	}
	if result.RequestID != "" && result.RequestID != request.RequestID {
		return fmt.Errorf("%w: checkout request id does not match", payment.ErrGatewayInvalidResponse)
	}
	if result.CustomPrice != request.CustomPrice {
		return fmt.Errorf("%w: checkout custom price does not match the quoted amount", payment.ErrGatewayInvalidResponse)
	}
	return nil
}

// verifyModeOnly 校验渠道响应的环境与当前 Profile 一致。
func (a *Adapter) verifyModeOnly(mode string) error {
	if mode != a.expectedMod {
		return fmt.Errorf("%w: provider mode %q does not match %q", payment.ErrGatewayInvalidResponse, mode, a.expectedMod)
	}
	return nil
}

// environmentOf 把钱包枚举映射成客户端的固定环境与响应 mode。
func environmentOf(environment wallet.ProviderEnvironment) (creemapi.Environment, string, error) {
	switch environment {
	case wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST:
		return creemapi.EnvironmentTest, "test", nil
	case wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_PRODUCTION:
		return creemapi.EnvironmentProduction, "prod", nil
	default:
		return "", "", errors.New("creem adapter: unsupported provider environment")
	}
}

// parseSuccessURL 只接受 https 绝对地址；跳转地址必须来自已校验 Product。
func parseSuccessURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("creem adapter: success url is empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("creem adapter: success url is invalid")
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("creem adapter: success url must be an absolute https url")
	}
	return raw, nil
}

// cloneMetadata 复制一份 metadata，避免调用方在请求过程中修改。
func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

// mapError 把渠道客户端错误映射成网关错误分类。
func mapError(operation string, err error) error {
	var target error
	switch {
	case errors.Is(err, creemapi.ErrRejected):
		target = payment.ErrGatewayRejected
	case errors.Is(err, creemapi.ErrUnauthorized):
		target = payment.ErrGatewayUnauthorized
	case errors.Is(err, creemapi.ErrNotFound):
		target = payment.ErrGatewayNotFound
	case errors.Is(err, creemapi.ErrInvalidResponse), errors.Is(err, creemapi.ErrResponseTooLarge):
		target = payment.ErrGatewayInvalidResponse
	default:
		target = payment.ErrGatewayUnavailable
	}
	return fmt.Errorf("%w: %s: %w", target, operation, err)
}

// checkoutResult 把官方 Checkout 映射成类型化视图。
func checkoutResult(checkout *creemapi.Checkout) payment.CheckoutResult {
	result := payment.CheckoutResult{
		ID:          checkout.ID,
		URL:         checkout.CheckoutURL,
		Status:      payment.ParseCheckoutStatus(checkout.Status),
		RawStatus:   checkout.Status,
		Mode:        checkout.Mode,
		ProductID:   checkout.Product.ID,
		Units:       checkout.Units,
		CustomPrice: checkout.CustomPrice,
		RequestID:   checkout.RequestID,
	}
	if order := checkout.Order; order != nil {
		result.Order = &payment.ProviderOrder{
			ID:             order.ID,
			ProductID:      order.Product.ID,
			Currency:       order.Currency,
			Status:         order.Status,
			Amount:         order.Amount,
			AmountPaid:     order.AmountPaid,
			DiscountAmount: order.DiscountAmount,
			TaxAmount:      order.TaxAmount,
			TransactionID:  order.Transaction.ID,
		}
	}
	return result
}

// transactionResult 把官方交易映射成类型化视图。
func transactionResult(transaction *creemapi.Transaction) payment.TransactionResult {
	return payment.TransactionResult{
		ID:             transaction.ID,
		Mode:           transaction.Mode,
		Currency:       transaction.Currency,
		Type:           transaction.Type,
		Status:         payment.ParseTransactionStatus(transaction.Status),
		RawStatus:      transaction.Status,
		Amount:         transaction.Amount,
		AmountPaid:     transaction.AmountPaid,
		DiscountAmount: transaction.DiscountAmount,
		TaxAmount:      transaction.TaxAmount,
		RefundedAmount: transaction.RefundedAmount,
		OrderID:        transaction.Order.ID,
	}
}

// 编译期断言：Adapter 必须实现钱包网关接口。
var _ payment.PaymentGateway = (*Adapter)(nil)
