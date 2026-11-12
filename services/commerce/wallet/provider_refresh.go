package wallet

import (
	"context"
	"strings"

	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// 管理员刷新（设计 §8.4）。
//
// 刷新只能“收敛到 Provider 事实”，不能指定成功、金额或汇率。带 Provider ID 的
// 订单按与回调相同的验证与应用函数处理；完全没有 Provider 对象的未知创建，
// 只有在管理员显式确认后才允许标为 FAILED，且之后真实成功回调仍可转 SUCCEEDED。

// adminRefreshRecharge 实现单笔充值刷新。
func (s *CommerceWalletService) adminRefreshRecharge(
	ctx context.Context,
	req *wallet.AdminRefreshRechargeReq,
) (*wallet.AdminRefreshRechargeResp, error) {
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
		return nil, wrapInternal("refresh recharge", err)
	}

	snapshot, ok := s.Runtime.Snapshot()
	if !ok {
		return nil, walletUnavailable("wallet settings are unavailable")
	}

	ids := providerIDs{
		checkoutID:    req.GetProviderCheckoutId(),
		orderID:       req.GetProviderOrderId(),
		transactionID: req.GetProviderTransactionId(),
	}
	// 管理员可以在“未知创建”后补充 Provider ID；先固化，再按冻结报价校验。
	if ids.checkoutID != "" || ids.orderID != "" || ids.transactionID != "" {
		if err := s.repository().runInTx(ctx, func(tx *ent.Tx) error {
			current, lockErr := lockRecharge(ctx, tx, rechargeID)
			if lockErr != nil {
				return lockErr
			}
			return persistProviderIDs(ctx, tx, current, providerObservation{
				checkoutID:    ids.checkoutID,
				orderID:       ids.orderID,
				transactionID: ids.transactionID,
			})
		}); err != nil {
			return nil, wrapInternal("refresh recharge", err)
		}
		recharge, err = s.DB.Recharge.Get(ctx, rechargeID)
		if err != nil {
			return nil, wrapInternal("refresh recharge", err)
		}
	}

	obs, _, err := s.observeRecharge(ctx, snapshot, recharge, ids)
	if err != nil {
		return nil, walletUnavailable("provider verification is unavailable")
	}
	if err := validateObservationAmount(recharge, obs.transaction); err != nil {
		return nil, err
	}

	hasProviderObject := obs.transaction != nil || obs.checkoutStatus != "" ||
		stringOrEmpty(recharge.ProviderCheckoutID) != "" ||
		stringOrEmpty(recharge.ProviderOrderID) != "" ||
		stringOrEmpty(recharge.ProviderTransactionID) != ""
	reason := strings.TrimSpace(req.GetReason())
	if reason == "" {
		reason = "administrator refresh"
	}

	var updated *ent.WalletRecharge
	switch {
	case obs.transaction != nil || obs.checkoutStatus != "":
		updated, err = s.applyProviderObservation(ctx, rechargeID, obs, actorSystemRefresh, reason)
		if err != nil {
			return nil, wrapInternal("refresh recharge", err)
		}
	case !hasProviderObject && req.GetConfirmProviderNotCreated() &&
		recharge.Status == rechargeStatusReview:
		// 未知创建类 REVIEW：管理员确认 Provider 侧没有对象后才能标 FAILED。
		updated, err = s.transitionIfStatus(ctx, rechargeID, rechargeStatusReview, rechargeStatusFailed, func(update *ent.WalletRechargeUpdate) {
			update.SetReviewReason(truncate("provider object was never created: "+reason, rechargeReviewReasonMaxLen)).
				SetReviewedBy(operator)
		})
		if err != nil {
			return nil, err
		}
	default:
		updated = recharge
	}

	account, err := s.DB.Account.Get(ctx, updated.WalletID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("recharge was not found")
	}
	if err != nil {
		return nil, wrapInternal("refresh recharge", err)
	}
	return &wallet.AdminRefreshRechargeResp{Recharge: adminRechargeViewOf(updated, account.UserID)}, nil
}

// parseOperator 校验管理员操作人标识；空值拒绝，长度与审计列一致。
func parseOperator(raw string) (string, error) {
	operator := strings.TrimSpace(raw)
	if operator == "" || len(operator) > 128 {
		return "", invalidArgument("operator_id is required and must be at most 128 characters")
	}
	return operator, nil
}
