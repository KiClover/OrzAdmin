package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/walletrecharge"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	settings "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
	creemprovider "github.com/KiClover/orzAdmin/services/commerce/wallet/provider/creem"
)

// 支付回调（设计 §8.2）。
//
// 顺序固定：Provider 白名单 -> 验签（原始 body + 恰好一个签名头 + 唯一 Secret）
// -> 解析事件定位订单 -> 校验订单 Profile -> 向 Provider 查询权威对象 -> 金额契约
// -> 单一状态应用函数。原始 body 与 Secret 都不落日志、不进响应。

const webhookSignatureHeader = "creem-signature"

// processPaymentWebhook 处理 Creem Webhook。
//
// 只有“无法重试”的问题（验签失败、订单不存在、Profile 不匹配）返回 accepted=false；
// 数据库或 Provider 暂时不可用返回 RPC 错误，让 Provider 按协议重试。
func (s *CommerceWalletService) processPaymentWebhook(
	ctx context.Context,
	req *wallet.ProcessPaymentWebhookReq,
) (*wallet.ProcessPaymentWebhookResp, error) {
	if req.GetProvider() != wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM {
		return webhookRejected("provider is not supported"), nil
	}
	body := req.GetBody()
	if len(body) == 0 {
		return webhookRejected("webhook payload is empty"), nil
	}

	snapshot, ok := s.Runtime.Snapshot()
	if !ok {
		// 快照缺失时无法确定 Secret，按暂时不可用处理，让 Provider 重试。
		return nil, walletUnavailable("wallet settings are unavailable")
	}
	candidates := webhookSecretCandidates(snapshot, time.UnixMilli(req.GetReceivedAt()))
	if len(candidates) == 0 {
		return nil, walletUnavailable("webhook secrets are not configured")
	}

	matched, err := creemprovider.VerifyWebhookSignature(
		body,
		headerValues(req.GetHeaders(), webhookSignatureHeader),
		candidates,
	)
	if err != nil {
		return webhookRejected("webhook signature is invalid"), nil
	}

	var event webhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return webhookRejected("webhook payload is malformed"), nil
	}

	recharge, err := s.locateWebhookRecharge(ctx, event.providerIDs())
	if errors.Is(err, errWebhookUnrelated) {
		return webhookRejected("webhook does not reference a known recharge"), nil
	}
	if err != nil {
		return nil, err
	}
	if recharge.PaymentProfileID != matched.ProfileID ||
		providerEnvironmentProto(recharge.ProviderEnvironment) != matched.Environment {
		return webhookRejected("webhook profile does not match the recharge"), nil
	}

	obs, _, err := s.observeRecharge(ctx, snapshot, recharge, event.providerIDs())
	if err != nil {
		// Provider 查询失败：不入账，让 Provider 重试。
		return nil, walletUnavailable("provider verification is unavailable")
	}
	if err := validateObservationAmount(recharge, obs.transaction); err != nil {
		return webhookRejected("provider amount does not match the frozen quote"), nil
	}

	updated, err := s.applyProviderObservation(ctx, recharge.ID, obs, actorProviderWebhook, event.Type)
	if err != nil {
		if errors.Is(err, errRechargeStateConflict) {
			return webhookRejected("recharge state has already advanced"), nil
		}
		return nil, err
	}
	return &wallet.ProcessPaymentWebhookResp{
		Accepted:   true,
		Message:    "webhook applied",
		RechargeId: stringPointer(formatDecimalID(updated.ID)),
	}, nil
}

// webhookRejected 构造“接受请求但不改变状态”的响应。
func webhookRejected(message string) *wallet.ProcessPaymentWebhookResp {
	return &wallet.ProcessPaymentWebhookResp{Accepted: false, Message: message}
}

// headerValues 保留同名头的重复值，供验签逻辑判断“恰好一个非空签名”。
func headerValues(headers []*wallet.WebhookHeader, name string) []string {
	var values []string
	for _, header := range headers {
		if header == nil || !strings.EqualFold(header.GetName(), name) {
			continue
		}
		values = append(values, header.GetValues()...)
	}
	return values
}

// webhookSecretCandidates 从同一份快照收集有限 Secret 候选（当前与 drain 中的旧 Secret）。
func webhookSecretCandidates(
	snapshot *settings.WalletRuntimeSettings,
	now time.Time,
) []creemprovider.SecretCandidate {
	var candidates []creemprovider.SecretCandidate
	add := func(profile *settings.WalletProviderProfileRuntime, environment wallet.ProviderEnvironment) {
		if profile == nil {
			return
		}
		for _, secret := range profile.GetWebhookSecrets() {
			if secret.GetSecret() == "" {
				continue
			}
			retiring := secret.GetRetireAfter() > 0 && secret.GetRetireAfter() <= now.UnixMilli()
			candidates = append(candidates, creemprovider.SecretCandidate{
				ProfileID:   profile.GetProfileId(),
				Environment: environment,
				Secret:      secret.GetSecret(),
				Retiring:    retiring,
			})
		}
	}
	add(snapshot.GetTestProfile(), wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST)
	add(snapshot.GetProductionProfile(), wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_PRODUCTION)
	return candidates
}

// errWebhookUnrelated 表示事件没有指向任何本地充值订单。
var errWebhookUnrelated = errors.New("webhook does not reference a recharge")

// webhookEvent 只解析定位订单所需的字段，其余内容不做假设。
type webhookEvent struct {
	ID     string `json:"id"`
	Type   string `json:"eventType"`
	Object struct {
		ID        string        `json:"id"`
		Object    string        `json:"object"`
		RequestID string        `json:"request_id"`
		Order     *webhookOrder `json:"order"`
		Checkout  *webhookRef   `json:"checkout"`
		Tx        *webhookRef   `json:"transaction"`
	} `json:"object"`
}

// webhookOrder 是事件对象里的订单摘要。
type webhookOrder struct {
	ID       string      `json:"id"`
	Checkout *webhookRef `json:"checkout"`
	Tx       *webhookRef `json:"transaction"`
}

// webhookRef 兼容字符串 ID 与 {"id": "..."} 两种引用形式。
type webhookRef struct {
	ID string `json:"id"`
}

// UnmarshalJSON 支持 "chk_123" 与 {"id":"chk_123"}。
func (r *webhookRef) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err == nil {
		r.ID = raw
		return nil
	}
	var object struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	r.ID = object.ID
	return nil
}

// providerIDs 汇总事件里携带的 Provider 对象 ID。
func (e webhookEvent) providerIDs() providerIDs {
	ids := providerIDs{
		checkoutID:    e.Object.ID,
		requestID:     e.Object.RequestID,
		transactionID: refID(e.Object.Tx),
	}
	if strings.HasPrefix(e.Type, "transaction.") {
		// transaction.* 事件的 object.id 是交易 ID，不是 Checkout ID。
		ids.transactionID = e.Object.ID
		ids.checkoutID = ""
	}
	if e.Object.Order != nil {
		if ids.orderID == "" {
			ids.orderID = e.Object.Order.ID
		}
		if ids.transactionID == "" {
			ids.transactionID = refID(e.Object.Order.Tx)
		}
		if ids.checkoutID == "" {
			ids.checkoutID = refID(e.Object.Order.Checkout)
		}
	}
	if ids.checkoutID == "" {
		ids.checkoutID = refID(e.Object.Checkout)
	}
	return ids
}

// providerIDs 是从事件中能提取到的 Provider 引用。
type providerIDs struct {
	checkoutID    string
	orderID       string
	transactionID string
	requestID     string
}

// refID 取引用 ID，nil 安全。
func refID(ref *webhookRef) string {
	if ref == nil {
		return ""
	}
	return ref.ID
}

// locateWebhookRecharge 用事件中的业务键或 Provider ID 定位订单。
func (s *CommerceWalletService) locateWebhookRecharge(
	ctx context.Context,
	ids providerIDs,
) (*ent.WalletRecharge, error) {
	lookups := []struct {
		column string
		value  string
	}{
		{"merchant_order_no", ids.requestID},
		{"provider_checkout_id", ids.checkoutID},
		{"provider_order_id", ids.orderID},
		{"provider_transaction_id", ids.transactionID},
	}
	for _, item := range lookups {
		if item.value == "" {
			continue
		}
		query := s.DB.Recharge.Query()
		switch item.column {
		case "merchant_order_no":
			query = query.Where(walletrecharge.MerchantOrderNo(item.value))
		case "provider_checkout_id":
			query = query.Where(walletrecharge.ProviderCheckoutID(item.value))
		case "provider_order_id":
			query = query.Where(walletrecharge.ProviderOrderID(item.value))
		case "provider_transaction_id":
			query = query.Where(walletrecharge.ProviderTransactionID(item.value))
		}
		recharge, err := query.Only(ctx)
		if err == nil {
			return recharge, nil
		}
		if !ent.IsNotFound(err) {
			return nil, wrapInternal("process webhook", err)
		}
	}
	return nil, errWebhookUnrelated
}

// gatewayForRecharge 用订单冻结的 Profile 构造查询网关。
func (s *CommerceWalletService) gatewayForRecharge(
	snapshot *settings.WalletRuntimeSettings,
	recharge *ent.WalletRecharge,
) (payment.PaymentGateway, *settings.WalletProviderProfileRuntime, error) {
	profile := profileByID(snapshot, recharge.ProviderEnvironment, recharge.PaymentProfileID)
	if profile == nil {
		return nil, nil, walletUnavailable("recharge profile is not available")
	}
	environment, err := providerEnvironmentFromProfile(profile.GetEnvironment())
	if err != nil {
		return nil, nil, err
	}
	gateway, err := s.Factory.NewGateway(gatewayCredentials(profile, environment, ""))
	if err != nil {
		return nil, nil, walletUnavailable("payment gateway is not configured")
	}
	return gateway, profile, nil
}

// observeProvider 查询权威 Checkout/Transaction 并组装观察结果。
func (s *CommerceWalletService) observeProvider(
	ctx context.Context,
	gateway payment.PaymentGateway,
	recharge *ent.WalletRecharge,
	ids providerIDs,
) (providerObservation, error) {
	obs := providerObservation{}
	checkoutID := ids.checkoutID
	if checkoutID == "" {
		checkoutID = stringOrEmpty(recharge.ProviderCheckoutID)
	}
	transactionID := ids.transactionID
	if transactionID == "" {
		transactionID = stringOrEmpty(recharge.ProviderTransactionID)
	}

	if checkoutID != "" {
		checkout, err := gateway.GetCheckout(ctx, checkoutID)
		if err != nil && !errors.Is(err, payment.ErrGatewayNotFound) {
			return obs, err
		}
		if err == nil {
			obs.checkoutStatus = checkout.Status
			obs.checkoutID = checkout.ID
			if checkout.Order != nil {
				obs.orderID = checkout.Order.ID
				if transactionID == "" {
					transactionID = checkout.Order.TransactionID
				}
			}
		}
	}
	if transactionID != "" {
		transaction, err := gateway.GetTransaction(ctx, transactionID)
		if err != nil {
			if errors.Is(err, payment.ErrGatewayNotFound) {
				return obs, nil
			}
			return obs, err
		}
		obs.transactionID = transaction.ID
		obs.transaction = &transaction
		if obs.orderID == "" {
			obs.orderID = transaction.OrderID
		}
	}
	return obs, nil
}

// observeRecharge 用已绑定的 Provider ID 主动查询，供回调、管理员刷新与退款复用。
func (s *CommerceWalletService) observeRecharge(
	ctx context.Context,
	snapshot *settings.WalletRuntimeSettings,
	recharge *ent.WalletRecharge,
	ids providerIDs,
) (providerObservation, *settings.WalletProviderProfileRuntime, error) {
	gateway, profile, err := s.gatewayForRecharge(snapshot, recharge)
	if err != nil {
		return providerObservation{}, nil, err
	}
	obs, err := s.observeProvider(ctx, gateway, recharge, ids)
	if err != nil {
		return providerObservation{}, nil, err
	}
	return obs, profile, nil
}
