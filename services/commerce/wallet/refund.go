package wallet

import (
	"context"
	"strings"
	"time"

	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	settings "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
)

// 管理员全额退款（设计 §8.5）。
//
// 退款前先把本地状态推进到 REFUNDING，再调用 Provider；任何无法确认的结果都保持
// REFUNDING，绝不盲目重发。只有“Provider 明确全额退款”才写 RECHARGE_REFUND 并
// 转 REFUNDED；部分退款/争议/拒付一律进入 REVIEW 并阻断扣款。

// adminRefundRecharge 实现管理员全额退款。
func (s *CommerceWalletService) adminRefundRecharge(
	ctx context.Context,
	req *wallet.AdminRefundRechargeReq,
) (*wallet.AdminRefundRechargeResp, error) {
	operator, err := parseOperator(req.GetOperatorId())
	if err != nil {
		return nil, err
	}
	rechargeID, err := parseDecimalID("recharge_id", req.GetRechargeId())
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(req.GetReason())
	if reason == "" || len(reason) > rechargeReviewReasonMaxLen {
		return nil, invalidArgument("reason is required and must be at most 512 characters")
	}

	recharge, err := s.DB.Recharge.Get(ctx, rechargeID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("recharge was not found")
	}
	if err != nil {
		return nil, wrapInternal("refund recharge", err)
	}
	snapshot, ok := s.Runtime.Snapshot()
	if !ok {
		return nil, walletUnavailable("wallet settings are unavailable")
	}

	transactionID := stringOrEmpty(recharge.ProviderTransactionID)
	if transactionID == "" {
		obs, _, obsErr := s.observeRecharge(ctx, snapshot, recharge, providerIDs{})
		if obsErr != nil {
			return nil, walletUnavailable("provider verification is unavailable")
		}
		if obs.transaction != nil {
			transactionID = obs.transaction.ID
		}
	}
	if transactionID == "" {
		return nil, stateConflict("provider transaction is not available for this recharge")
	}

	// 先落 REFUNDING 再调用 Provider；重复调用同一笔退款保持幂等。
	recharge, err = s.beginRefund(ctx, rechargeID, operator, reason)
	if err != nil {
		return nil, err
	}
	if recharge.Status == rechargeStatusRefunded {
		return s.adminRefundView(ctx, recharge)
	}

	gateway, _, err := s.gatewayForRecharge(snapshot, recharge)
	if err != nil {
		return nil, err
	}
	result, err := gateway.CreateFullRefund(ctx, transactionID)
	if err != nil {
		// 结果未知：保持 REFUNDING，等待回调或管理员刷新。
		return nil, walletUnavailable("refund result is unknown; recharge remains refunding")
	}

	switch result.Status {
	case payment.RefundStatusSucceeded:
		recharge, err = s.settleRefund(ctx, snapshot, recharge, transactionID)
	case payment.RefundStatusFailed, payment.RefundStatusCanceled:
		recharge, err = s.recoverFailedRefund(ctx, snapshot, recharge, transactionID)
	default:
		// PENDING/UNKNOWN：保持 REFUNDING，等待 Provider 最终事实。
		recharge, err = s.reloadRecharge(ctx, rechargeID)
	}
	if err != nil {
		return nil, err
	}
	return s.adminRefundView(ctx, recharge)
}

// beginRefund 把 SUCCEEDED 推进到 REFUNDING 并记录退款意图；已 REFUNDED 直接返回。
func (s *CommerceWalletService) beginRefund(
	ctx context.Context,
	rechargeID uint64,
	operator, reason string,
) (*ent.WalletRecharge, error) {
	var updated *ent.WalletRecharge
	err := s.repository().runInTx(ctx, func(tx *ent.Tx) error {
		current, err := lockRecharge(ctx, tx, rechargeID)
		if err != nil {
			return err
		}
		switch current.Status {
		case rechargeStatusRefunded, rechargeStatusRefunding:
			updated = current
			return nil
		case rechargeStatusSucceeded:
		default:
			return stateConflict("only a succeeded recharge can be refunded")
		}
		now := time.Now()
		updated, err = transitionRecharge(ctx, tx, current, rechargeStatusRefunding, func(update *ent.WalletRechargeUpdate) {
			update.SetRefundReason(truncate(reason, rechargeReviewReasonMaxLen)).
				SetRefundRequestedBy(operator).
				SetRefundRequestedAt(now)
		})
		return err
	})
	if err != nil {
		return nil, wrapInternal("refund recharge", err)
	}
	return updated, nil
}

// settleRefund 在 Provider 明确全额退款后写 RECHARGE_REFUND 并转 REFUNDED。
func (s *CommerceWalletService) settleRefund(
	ctx context.Context,
	snapshot *settings.WalletRuntimeSettings,
	recharge *ent.WalletRecharge,
	transactionID string,
) (*ent.WalletRecharge, error) {
	gateway, _, err := s.gatewayForRecharge(snapshot, recharge)
	if err != nil {
		return nil, err
	}
	transaction, err := gateway.GetTransaction(ctx, transactionID)
	if err != nil {
		return nil, walletUnavailable("refund result cannot be verified")
	}
	if !fullRefundSettled(&transaction, recharge.ProviderAmount) {
		// 退款凭证尚未反映全额事实：保持 REFUNDING，等回调或下次刷新。
		return s.reloadRecharge(ctx, recharge.ID)
	}
	obs := providerObservation{
		checkoutID:    stringOrEmpty(recharge.ProviderCheckoutID),
		transactionID: transaction.ID,
		transaction:   &transaction,
	}
	return s.applyProviderObservation(ctx, recharge.ID, obs, actorProviderRefund, "administrator full refund")
}

// recoverFailedRefund 处理 Provider 明确返回 failed/canceled 的退款。
//
// 必须先反查交易：已全退则补记并收敛；仍为 paid 且未退款才允许恢复 SUCCEEDED；
// 部分退款或状态不明一律进入 REVIEW 并阻断扣款。
func (s *CommerceWalletService) recoverFailedRefund(
	ctx context.Context,
	snapshot *settings.WalletRuntimeSettings,
	recharge *ent.WalletRecharge,
	transactionID string,
) (*ent.WalletRecharge, error) {
	gateway, _, err := s.gatewayForRecharge(snapshot, recharge)
	if err != nil {
		return nil, err
	}
	transaction, err := gateway.GetTransaction(ctx, transactionID)
	if err != nil {
		return nil, walletUnavailable("refund result cannot be verified")
	}
	if fullRefundSettled(&transaction, recharge.ProviderAmount) {
		obs := providerObservation{transactionID: transaction.ID, transaction: &transaction}
		return s.applyProviderObservation(ctx, recharge.ID, obs, actorProviderRefund, "provider refund settled")
	}
	if transaction.Status == payment.TransactionStatusPaid &&
		(transaction.RefundedAmount == nil || *transaction.RefundedAmount == 0) {
		return s.transitionIfStatus(ctx, recharge.ID, rechargeStatusRefunding, rechargeStatusSucceeded, nil)
	}
	return s.markRechargeReviewLocked(ctx, recharge.ID, actorProviderRefund,
		"provider reported a refund that did not settle", true)
}

// markRechargeReviewLocked 在独立事务中把订单转入 REVIEW。
func (s *CommerceWalletService) markRechargeReviewLocked(
	ctx context.Context,
	rechargeID uint64,
	actor, reason string,
	blockDebit bool,
) (*ent.WalletRecharge, error) {
	var updated *ent.WalletRecharge
	err := s.repository().runInTx(ctx, func(tx *ent.Tx) error {
		current, err := lockRecharge(ctx, tx, rechargeID)
		if err != nil {
			return err
		}
		updated, err = s.markRechargeReview(ctx, tx, current, actor, reason, blockDebit)
		return err
	})
	if err != nil {
		return nil, wrapInternal("refund recharge", err)
	}
	return updated, nil
}

// reloadRecharge 重新读取订单当前状态。
func (s *CommerceWalletService) reloadRecharge(
	ctx context.Context,
	rechargeID uint64,
) (*ent.WalletRecharge, error) {
	recharge, err := s.DB.Recharge.Get(ctx, rechargeID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("recharge was not found")
	}
	if err != nil {
		return nil, wrapInternal("refund recharge", err)
	}
	return recharge, nil
}

// adminRefundView 组装管理员视图。
func (s *CommerceWalletService) adminRefundView(
	ctx context.Context,
	recharge *ent.WalletRecharge,
) (*wallet.AdminRefundRechargeResp, error) {
	account, err := s.DB.Account.Get(ctx, recharge.WalletID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("recharge was not found")
	}
	if err != nil {
		return nil, wrapInternal("refund recharge", err)
	}
	return &wallet.AdminRefundRechargeResp{Recharge: adminRechargeViewOf(recharge, account.UserID)}, nil
}
