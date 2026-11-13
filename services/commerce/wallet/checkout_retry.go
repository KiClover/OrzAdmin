package wallet

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
)

// 显式重建支付链接（用户重试 / 管理员操作）。
//
// 只对"本地没有任何 Provider 结果"的冻结订单生效：创建请求在写入 CREATED 之后
// 中断、或渠道结果未知进入 REVIEW 时，订单没有 checkout_url，也没有任何 Provider ID。
// 这类订单可以在用户或管理员明确触发时重建一次 Checkout；其余状态一律不碰。
//
// 重要事实：Creem 的 request_id 只是业务标识，不提供幂等语义（同一 request_id
// 连续提交会得到不同 Checkout），因此安全性只能来自本地状态机——只有从未记录过
// Provider 对象的订单才允许重建，历史上未被返回给用户的孤儿 Checkout 不可达。

const (
	// rebuildReasonUserReplay 是重放 CREATED 订单时的审计原因。
	rebuildReasonUserReplay = "created recharge replay"
	// rebuildReasonAdminDefault 是管理员未填写原因时的默认审计文案。
	rebuildReasonAdminDefault = "administrator rebuilt checkout"
)

// retryRechargeCheckout 实现用户侧重建支付链接，只允许操作自己的订单。
func (s *CommerceWalletService) retryRechargeCheckout(
	ctx context.Context,
	req *wallet.RetryRechargeCheckoutReq,
) (*wallet.RetryRechargeCheckoutResp, error) {
	recharge, err := s.userRecharge(ctx, req.GetUserId(), req.GetRechargeId())
	if err != nil {
		return nil, err
	}
	updated, err := s.retryCheckoutFor(ctx, recharge, actorUser, rebuildReasonUserReplay)
	if err != nil {
		return nil, err
	}
	return &wallet.RetryRechargeCheckoutResp{Recharge: rechargeViewOf(updated)}, nil
}

// adminRetryRechargeCheckout 实现管理员侧重建支付链接。
//
// 管理员可以处理任意订单，但重建规则与用户侧完全一致：不指定金额、汇率或成功态，
// 只能把"没有 Provider 对象"的订单重新推到待支付。
func (s *CommerceWalletService) adminRetryRechargeCheckout(
	ctx context.Context,
	req *wallet.AdminRetryRechargeCheckoutReq,
) (*wallet.AdminRetryRechargeCheckoutResp, error) {
	operator, err := parseOperator(req.GetOperatorId())
	if err != nil {
		return nil, err
	}
	rechargeID, err := parseDecimalID("recharge_id", req.GetRechargeId())
	if err != nil {
		return nil, err
	}
	recharge, err := s.DB.Recharge.Get(ctx, rechargeID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("recharge was not found")
	}
	if err != nil {
		return nil, wrapInternal("retry recharge checkout", err)
	}
	reason := strings.TrimSpace(req.GetReason())
	if reason == "" {
		reason = rebuildReasonAdminDefault
	}

	updated, err := s.retryCheckoutFor(ctx, recharge, operator, reason)
	if err != nil {
		return nil, err
	}
	account, err := s.DB.Account.Get(ctx, updated.WalletID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("recharge was not found")
	}
	if err != nil {
		return nil, wrapInternal("retry recharge checkout", err)
	}
	return &wallet.AdminRetryRechargeCheckoutResp{Recharge: adminRechargeViewOf(updated, account.UserID)}, nil
}

// retryCheckoutFor 校验"可重建"前提并执行一次重建。
//
// 幂等语义：订单已经进入 PENDING_PAYMENT 且有链接时直接返回现状，用户的按钮可以
// 重复点击；其余状态返回 STATE_CONFLICT，避免对已支付或已有 Provider 对象的订单
// 再次下单。
func (s *CommerceWalletService) retryCheckoutFor(
	ctx context.Context,
	recharge *ent.WalletRecharge,
	actor, reason string,
) (*ent.WalletRecharge, error) {
	if recharge.Status == rechargeStatusPendingPayment && stringOrEmpty(recharge.CheckoutURL) != "" {
		return recharge, nil
	}
	if !rechargeCheckoutRebuildable(recharge.Status) {
		return nil, stateConflict("recharge cannot be rebuilt in its current state")
	}
	if hasProviderObject(recharge) {
		return nil, stateConflict("recharge already has a provider checkout")
	}
	return s.rebuildCheckout(ctx, recharge, actor, reason)
}

// rechargeCheckoutRebuildable 判断状态是否允许重建：只有从未确认过 Provider 结果的两个状态。
func rechargeCheckoutRebuildable(status string) bool {
	return status == rechargeStatusCreated || status == rechargeStatusReview
}

// hasProviderObject 判断订单是否已经记录过任何 Provider 对象。
func hasProviderObject(recharge *ent.WalletRecharge) bool {
	return stringOrEmpty(recharge.ProviderCheckoutID) != "" ||
		stringOrEmpty(recharge.ProviderOrderID) != "" ||
		stringOrEmpty(recharge.ProviderTransactionID) != "" ||
		stringOrEmpty(recharge.CheckoutURL) != ""
}

// rebuildCheckout 用冻结报价重建一次 Checkout，并把结果写回同一笔订单。
func (s *CommerceWalletService) rebuildCheckout(
	ctx context.Context,
	recharge *ent.WalletRecharge,
	actor, reason string,
) (*ent.WalletRecharge, error) {
	snapshot, ok := s.Runtime.Snapshot()
	if !ok {
		return nil, featureDisabled(reasonSettingsUnavailable)
	}
	profile, environment, err := s.rechargeProfile(snapshot)
	if err != nil {
		return nil, err
	}
	// 冻结行记录了创建时的 Profile/Product；若当前快照已换代，重建会指向另一个商品或账户。
	if profile.GetProductId() != recharge.ProviderProductID ||
		providerEnvironmentProto(recharge.ProviderEnvironment) != environment {
		return nil, stateConflict("payment profile changed since the recharge was created")
	}
	if s.Factory == nil || s.Catalog == nil {
		return nil, walletUnavailable("payment gateway is not configured")
	}
	credentials := gatewayCredentials(profile, environment, "")
	successURL, err := s.Catalog.ProductSuccessURL(ctx, credentials)
	if err != nil {
		return nil, walletUnavailable("payment product is not available")
	}
	credentials.SuccessURL = successURL

	gateway, err := s.Factory.NewGateway(credentials)
	if err != nil {
		return nil, walletUnavailable("payment gateway is not configured")
	}
	// 金额与 request_id 全部取冻结行：重建不改价，也不会换一个业务标识。
	checkout, err := gateway.CreateCheckout(ctx, payment.CheckoutRequest{
		CustomPrice: recharge.ProviderAmount,
		RequestID:   recharge.MerchantOrderNo,
		Metadata: map[string]string{
			"recharge_id": formatDecimalID(recharge.ID),
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, payment.ErrGatewayRejected),
			errors.Is(err, payment.ErrGatewayUnauthorized),
			errors.Is(err, payment.ErrGatewayNotFound):
			logger.Warnf("wallet rebuilt checkout rejected recharge_id=%d actor=%s reason=%s err=%v",
				recharge.ID, actor, reason, err)
			return nil, s.failRebuild(ctx, recharge)
		default:
			logger.Warnf("wallet rebuilt checkout unknown recharge_id=%d actor=%s reason=%s err=%v",
				recharge.ID, actor, reason, err)
			return s.markRebuildUnknown(ctx, recharge)
		}
	}

	expectation := payment.CheckoutExpectation{
		Environment:    environment,
		ProductID:      profile.GetProductId(),
		ProviderAmount: recharge.ProviderAmount,
		RequestID:      recharge.MerchantOrderNo,
	}
	if err := payment.ValidateCheckout(checkout, expectation); err != nil {
		logger.Warnf("wallet rebuilt checkout contract violation recharge_id=%d checkout_id=%s err=%v",
			recharge.ID, checkout.ID, err)
		return s.markRebuildUnknown(ctx, recharge)
	}

	updated, err := s.persistRebuiltCheckout(ctx, recharge, checkout, actor)
	if err != nil {
		return nil, err
	}
	logger.Infof("wallet checkout rebuilt recharge_id=%d checkout_id=%s actor=%s reason=%s",
		updated.ID, checkout.ID, actor, reason)
	return updated, nil
}

// failRebuild 处理渠道的明确拒绝。
//
// CREATED 与新单创建保持一致：本地可安全落 FAILED；REVIEW 已经处于人工核对，
// 重建失败不改变其状态，避免把"待人工确认"误判成终态。
func (s *CommerceWalletService) failRebuild(
	ctx context.Context,
	recharge *ent.WalletRecharge,
) error {
	if recharge.Status == rechargeStatusCreated {
		if _, err := s.transitionIfStatus(ctx, recharge.ID, rechargeStatusCreated, rechargeStatusFailed, nil); err != nil {
			return err
		}
		return walletUnavailable("payment checkout could not be created; recharge was not started")
	}
	return walletUnavailable("payment checkout could not be created; recharge remains under review")
}

// markRebuildUnknown 处理结果未知的重建：CREATED 转入 REVIEW，REVIEW 保持原状。
func (s *CommerceWalletService) markRebuildUnknown(
	ctx context.Context,
	recharge *ent.WalletRecharge,
) (*ent.WalletRecharge, error) {
	if recharge.Status == rechargeStatusReview {
		return recharge, nil
	}
	return s.markCreateUnknown(ctx, recharge)
}

// persistRebuiltCheckout 把重建的 Checkout 写回订单。
//
// 与 persistCheckout 的差异：REVIEW 订单没有任何 Provider ID，重建成功后直接从
// REVIEW/CREATED 推进到 PENDING_PAYMENT 并写入 checkout_url；如果并发重试已经先
// 一步完成，则返回现状（调用方会拿到先完成那一方的链接）。
func (s *CommerceWalletService) persistRebuiltCheckout(
	ctx context.Context,
	recharge *ent.WalletRecharge,
	checkout payment.CheckoutResult,
	actor string,
) (*ent.WalletRecharge, error) {
	var updated *ent.WalletRecharge
	err := s.repository().runInTx(ctx, func(tx *ent.Tx) error {
		current, err := lockRecharge(ctx, tx, recharge.ID)
		if err != nil {
			return err
		}
		if !rechargeCheckoutRebuildable(current.Status) {
			// 并发重试/回调已经推进了订单：以数据库现状为准。
			updated = current
			return nil
		}
		if hasProviderObject(current) {
			return stateConflict("recharge already has a provider checkout")
		}
		now := time.Now()
		updated, err = transitionRecharge(ctx, tx, current, rechargeStatusPendingPayment, func(update *ent.WalletRechargeUpdate) {
			update.SetProviderCheckoutID(checkout.ID).
				SetCheckoutURL(checkout.URL).
				SetReviewedBy(actor).
				SetReviewedAt(now)
			if orderID := orderIDOf(checkout); orderID != "" {
				update.SetProviderOrderID(orderID)
			}
			if transactionID := transactionIDOf(checkout); transactionID != "" {
				update.SetProviderTransactionID(transactionID)
			}
		})
		return err
	})
	if err != nil {
		return nil, wrapInternal("retry recharge checkout", err)
	}
	return updated, nil
}