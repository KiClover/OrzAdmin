package payment

import (
	"context"
	"errors"
	"fmt"

	walletpb "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// 支付网关抽象：钱包只依赖这几个操作与类型化结果，不直接接触渠道 JSON。
//
// 设计约束：
//   - 只包含 CreateCheckout、GetCheckout/GetTransaction、CreateFullRefund；
//   - 结果保留官方状态原文（RawStatus），未知状态映射为 Unknown 而不是成功；
//   - 金额一律是整数最小货币单位，币种由字段名区分，不混用 CNY 与 USD。

// PaymentGateway 是钱包对支付渠道的最小依赖面。
type PaymentGateway interface {
	// CreateCheckout 创建一次 Checkout。请求里的金额是 USD 美分，不接受用户输入。
	CreateCheckout(ctx context.Context, request CheckoutRequest) (CheckoutResult, error)
	// GetCheckout 查询 Checkout 的权威状态。
	GetCheckout(ctx context.Context, checkoutID string) (CheckoutResult, error)
	// GetTransaction 查询支付交易的权威状态。
	GetTransaction(ctx context.Context, transactionID string) (TransactionResult, error)
	// CreateFullRefund 发起全额退款；渠道可能异步确认。
	CreateFullRefund(ctx context.Context, transactionID string) (RefundResult, error)
}

// GatewayCredentials 是一个已通过远程校验的渠道 Profile 的最小凭据集合。
//
// APIKey、ProductID 与 SuccessURL 都来自 Setting 的不可变快照，不由请求体提供。
type GatewayCredentials struct {
	Environment walletpb.ProviderEnvironment
	APIKey      string
	ProductID   string
	SuccessURL  string
}

// PaymentGatewayFactory 按 Profile 凭据构造网关；同一凭据可复用同一实例。
type PaymentGatewayFactory interface {
	NewGateway(credentials GatewayCredentials) (PaymentGateway, error)
}

// ProductCatalog 只读取 Provider 已配置 Product 的成功跳转地址。
//
// 充值创建必须使用该 Product 已通过远程校验的 default_success_url，不能接受
// 请求体里的跳转地址；这里只暴露只读的最小能力，避免把完整渠道客户端泄漏给业务层。
type ProductCatalog interface {
	ProductSuccessURL(ctx context.Context, credentials GatewayCredentials) (string, error)
}

// 网关错误分类。调用方据此决定“明确失败可落 FAILED”还是“未知结果转 REVIEW”：
// 只有 Rejected 与 InvalidResponse 代表结果可确定，其余都应视为未知结果。
var (
	// ErrGatewayRejected 表示渠道明确拒绝了请求（4xx 业务错误）。
	ErrGatewayRejected = errors.New("payment gateway rejected the request")
	// ErrGatewayUnauthorized 表示渠道凭据无效或无权限。
	ErrGatewayUnauthorized = errors.New("payment gateway credentials are invalid")
	// ErrGatewayNotFound 表示渠道侧不存在该对象。
	ErrGatewayNotFound = errors.New("payment gateway resource not found")
	// ErrGatewayInvalidResponse 表示渠道响应不可解析或不符合契约。
	ErrGatewayInvalidResponse = errors.New("payment gateway returned an invalid response")
	// ErrGatewayUnavailable 表示无法确认结果（网络、超时、5xx 等），不得盲目重试创建。
	ErrGatewayUnavailable = errors.New("payment gateway is unavailable")
)

// CheckoutRequest 是创建 Checkout 的业务输入。
type CheckoutRequest struct {
	// CustomPrice 是冻结的 USD 美分金额，必须由服务端按固定汇率换算。
	CustomPrice int64
	// RequestID 是稳定请求 ID，会作为 Creem request_id 与 metadata 的一部分。
	RequestID string
	// Metadata 只允许不敏感字段（本地订单 ID、汇率 revision 等）。
	Metadata map[string]string
}

// CheckoutStatus 是 Checkout 的映射后状态；Unknown 表示官方出现了未知枚举。
type CheckoutStatus string

const (
	CheckoutStatusUnknown    CheckoutStatus = "UNKNOWN"
	CheckoutStatusPending    CheckoutStatus = "PENDING"
	CheckoutStatusProcessing CheckoutStatus = "PROCESSING"
	CheckoutStatusCompleted  CheckoutStatus = "COMPLETED"
	CheckoutStatusExpired    CheckoutStatus = "EXPIRED"
)

// TransactionStatus 是支付交易的映射后状态；Unknown 表示官方出现了未知枚举。
type TransactionStatus string

const (
	TransactionStatusUnknown    TransactionStatus = "UNKNOWN"
	TransactionStatusPending    TransactionStatus = "PENDING"
	TransactionStatusPaid       TransactionStatus = "PAID"
	TransactionStatusRefunded   TransactionStatus = "REFUNDED"
	TransactionStatusPartial    TransactionStatus = "PARTIAL_REFUND"
	TransactionStatusChargeback TransactionStatus = "CHARGEBACK"
	TransactionStatusUncollect  TransactionStatus = "UNCOLLECTIBLE"
	TransactionStatusDeclined   TransactionStatus = "DECLINED"
	TransactionStatusCanceled   TransactionStatus = "CANCELED"
	TransactionStatusVoided     TransactionStatus = "VOID"
)

// RefundStatus 是退款请求的映射后状态；渠道可能返回 pending。
type RefundStatus string

const (
	RefundStatusUnknown   RefundStatus = "UNKNOWN"
	RefundStatusPending   RefundStatus = "PENDING"
	RefundStatusSucceeded RefundStatus = "SUCCEEDED"
	RefundStatusFailed    RefundStatus = "FAILED"
	RefundStatusCanceled  RefundStatus = "CANCELED"
)

// ProviderOrder 是 Checkout 内嵌的订单摘要。
type ProviderOrder struct {
	ID             string
	ProductID      string
	Currency       string
	Status         string
	Amount         int64
	AmountPaid     *int64
	DiscountAmount *int64
	TaxAmount      *int64
	TransactionID  string
}

// CheckoutResult 是网关返回的 Checkout 视图。
type CheckoutResult struct {
	ID          string
	URL         string
	Status      CheckoutStatus
	RawStatus   string
	Mode        string
	ProductID   string
	Units       int64
	CustomPrice int64
	RequestID   string
	Order       *ProviderOrder
}

// TransactionResult 是网关返回的支付交易视图。
type TransactionResult struct {
	ID             string
	Mode           string
	Currency       string
	Type           string
	Status         TransactionStatus
	RawStatus      string
	Amount         int64
	AmountPaid     *int64
	DiscountAmount *int64
	TaxAmount      *int64
	RefundedAmount *int64
	OrderID        string
}

// RefundResult 是网关返回的退款视图。
type RefundResult struct {
	ID            string
	Mode          string
	Status        RefundStatus
	RawStatus     string
	TransactionID string
	Amount        *int64
}

// errProviderContract 表示渠道响应不符合钱包的金额/环境/产品契约。
var errProviderContract = errors.New("provider response violates the wallet contract")

// ParseCheckoutStatus 映射官方 Checkout 状态；未知值返回 Unknown，绝不视为成功。
func ParseCheckoutStatus(raw string) CheckoutStatus {
	switch raw {
	case "pending":
		return CheckoutStatusPending
	case "processing":
		return CheckoutStatusProcessing
	case "completed":
		return CheckoutStatusCompleted
	case "expired":
		return CheckoutStatusExpired
	default:
		return CheckoutStatusUnknown
	}
}

// ParseTransactionStatus 映射官方交易状态；未知值返回 Unknown。
func ParseTransactionStatus(raw string) TransactionStatus {
	switch raw {
	case "pending":
		return TransactionStatusPending
	case "paid":
		return TransactionStatusPaid
	case "refunded":
		return TransactionStatusRefunded
	case "partialRefund":
		return TransactionStatusPartial
	case "chargedBack":
		return TransactionStatusChargeback
	case "uncollectible":
		return TransactionStatusUncollect
	case "declined":
		return TransactionStatusDeclined
	case "canceled":
		return TransactionStatusCanceled
	case "void":
		return TransactionStatusVoided
	default:
		return TransactionStatusUnknown
	}
}

// ParseRefundStatus 映射官方退款状态；未知值返回 Unknown。
func ParseRefundStatus(raw string) RefundStatus {
	switch raw {
	case "pending":
		return RefundStatusPending
	case "succeeded":
		return RefundStatusSucceeded
	case "failed":
		return RefundStatusFailed
	case "canceled":
		return RefundStatusCanceled
	default:
		return RefundStatusUnknown
	}
}

// expectedProviderMode 把环境映射成 Creem 响应中的 mode 原文。
func expectedProviderMode(environment walletpb.ProviderEnvironment) (string, error) {
	switch environment {
	case walletpb.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST:
		return "test", nil
	case walletpb.ProviderEnvironment_PROVIDER_ENVIRONMENT_PRODUCTION:
		return "prod", nil
	default:
		return "", fmt.Errorf("%w: unsupported provider environment", errProviderContract)
	}
}

// CheckoutExpectation 是验证 Checkout 响应所需的冻结订单信息。
type CheckoutExpectation struct {
	Environment    walletpb.ProviderEnvironment
	ProductID      string
	ProviderAmount int64
	RequestID      string
}

// ValidateCheckout 校验 Checkout 响应的环境、Product、币种、金额与请求号。
//
// 只接受满足冻结报价的响应；任何不一致都视为渠道契约违例，调用方不得据此入账。
func ValidateCheckout(result CheckoutResult, expectation CheckoutExpectation) error {
	mode, err := expectedProviderMode(expectation.Environment)
	if err != nil {
		return err
	}
	if result.Mode != mode {
		return fmt.Errorf("%w: checkout mode %q does not match %q", errProviderContract, result.Mode, mode)
	}
	if result.ProductID != expectation.ProductID {
		return fmt.Errorf("%w: checkout product does not match the frozen product", errProviderContract)
	}
	// 渠道不一定在响应里回显 request_id（Creem 的 checkout 响应就没有该字段），
	// 因此空值视为“未回显”而非违例；一旦回显就必须与冻结请求号一致。
	if result.RequestID != "" && result.RequestID != expectation.RequestID {
		return fmt.Errorf("%w: checkout request id does not match", errProviderContract)
	}
	if result.CustomPrice != expectation.ProviderAmount {
		return fmt.Errorf("%w: checkout custom price does not match the frozen amount", errProviderContract)
	}
	if result.Status == CheckoutStatusUnknown {
		return fmt.Errorf("%w: unknown checkout status %q", errProviderContract, result.RawStatus)
	}
	if result.Order != nil {
		order := result.Order
		if order.Currency != "USD" {
			return fmt.Errorf("%w: checkout order currency is not USD", errProviderContract)
		}
		if order.Amount != expectation.ProviderAmount {
			return fmt.Errorf("%w: checkout order amount does not match the frozen amount", errProviderContract)
		}
		if order.ProductID != "" && order.ProductID != expectation.ProductID {
			return fmt.Errorf("%w: checkout order product does not match the frozen product", errProviderContract)
		}
	}
	return nil
}

// TransactionExpectation 是验证支付交易所需的冻结订单信息。
type TransactionExpectation struct {
	Environment    walletpb.ProviderEnvironment
	ProviderAmount int64
}

// ValidatePaidTransaction 校验已支付交易的环境、类型、币种、金额、实付与零折扣。
//
// 税可以非零（inclusive 已包含在 custom_price 内），但折扣必须缺省或为 0。
func ValidatePaidTransaction(result TransactionResult, expectation TransactionExpectation) error {
	mode, err := expectedProviderMode(expectation.Environment)
	if err != nil {
		return err
	}
	if result.Mode != mode {
		return fmt.Errorf("%w: transaction mode %q does not match %q", errProviderContract, result.Mode, mode)
	}
	if result.Type != "payment" {
		return fmt.Errorf("%w: transaction type is not payment", errProviderContract)
	}
	if result.Status != TransactionStatusPaid {
		return fmt.Errorf("%w: transaction is not paid", errProviderContract)
	}
	if result.Currency != "USD" {
		return fmt.Errorf("%w: transaction currency is not USD", errProviderContract)
	}
	if result.Amount != expectation.ProviderAmount {
		return fmt.Errorf("%w: transaction amount does not match the frozen amount", errProviderContract)
	}
	if result.AmountPaid == nil || *result.AmountPaid != expectation.ProviderAmount {
		return fmt.Errorf("%w: transaction paid amount does not match the frozen amount", errProviderContract)
	}
	if result.DiscountAmount != nil && *result.DiscountAmount != 0 {
		return fmt.Errorf("%w: discounted payments are not supported", errProviderContract)
	}
	return nil
}
