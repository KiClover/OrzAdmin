package wallet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/walletaccount"
	"github.com/KiClover/orzAdmin/ent/walletrecharge"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
)

// 充值生命周期（设计 §8.1/§8.3）。
//
// 创建流程固定为：读取一次 Setting 快照 -> 冻结报价并落 CREATED -> 调用 Provider
// Create Checkout -> 用 revision CAS 写回 Checkout 结果。任何无法确认结果的创建
// 都停留 REVIEW，绝不盲目重试；回调可能早于创建响应，CAS 失败时以数据库现状为准。

// createRecharge 实现用户创建充值。
func (s *CommerceWalletService) createRecharge(
	ctx context.Context,
	req *wallet.CreateRechargeReq,
) (*wallet.CreateRechargeResp, error) {
	userID, err := parseDecimalID("user_id", req.GetUserId())
	if err != nil {
		return nil, err
	}
	requestID := req.GetRequestId()
	if requestID == "" || len(requestID) > requestIDMaxLen {
		return nil, invalidArgument("request_id is required and must be at most 64 characters")
	}
	if req.GetWalletAmount() <= 0 {
		return nil, invalidArgument("wallet_amount must be positive")
	}
	provider, err := providerCode(req.GetProvider())
	if err != nil {
		return nil, err
	}

	snapshot, ok := s.Runtime.Snapshot()
	if !ok {
		return nil, featureDisabled(reasonSettingsUnavailable)
	}
	profile, environment, err := s.rechargeProfile(snapshot)
	if err != nil {
		return nil, err
	}
	rate, err := snapshotRate(snapshot)
	if err != nil {
		return nil, err
	}
	providerAmount, err := QuoteProviderAmount(req.GetWalletAmount(), rate)
	if err != nil {
		return nil, invalidArgument("wallet_amount is out of range for the configured exchange rate")
	}
	if s.Factory == nil || s.Catalog == nil {
		return nil, walletUnavailable("payment gateway is not configured")
	}

	// 成功跳转地址来自 Product 的 default_success_url，必须先从 Provider 读取并校验。
	credentials := gatewayCredentials(profile, environment, "")
	successURL, err := s.Catalog.ProductSuccessURL(ctx, credentials)
	if err != nil {
		return nil, walletUnavailable("payment product is not available")
	}
	credentials.SuccessURL = successURL

	var (
		recharge *ent.WalletRecharge
		replayed bool
	)
	err = s.repository().runInTx(ctx, func(tx *ent.Tx) error {
		account, err := s.repository().lockWallet(ctx, tx, userID)
		if err != nil {
			return err
		}
		existing, err := tx.WalletRecharge.Query().
			Where(
				walletrecharge.WalletID(account.ID),
				walletrecharge.RequestID(requestID),
			).
			Only(ctx)
		if err == nil {
			if existing.WalletAmount != req.GetWalletAmount() || existing.Provider != provider {
				return idempotencyConflict()
			}
			recharge = existing
			replayed = true
			return nil
		}
		if !ent.IsNotFound(err) {
			return err
		}

		created, err := s.newRechargeRow(ctx, tx, account.ID, requestID, provider, profile, rate, req.GetWalletAmount(), providerAmount)
		if err != nil {
			return err
		}
		recharge = created
		return nil
	})
	if err != nil {
		return nil, wrapInternal("create recharge", err)
	}
	// 完全一致的 request_id 重放只返回原订单，不重新计算报价，也不重新创建 Checkout。
	if replayed {
		return &wallet.CreateRechargeResp{Recharge: rechargeViewOf(recharge)}, nil
	}

	gateway, err := s.Factory.NewGateway(credentials)
	if err != nil {
		return s.failRechargeCreation(ctx, recharge, err)
	}

	checkout, err := gateway.CreateCheckout(ctx, payment.CheckoutRequest{
		CustomPrice: providerAmount,
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
			// 渠道明确拒绝：本地可安全落 FAILED，不产生任何入账。
			return s.failRechargeCreation(ctx, recharge, err)
		default:
			// 超时、断线、5xx、响应不可解析：结果未知，进入 REVIEW，不得重试创建。
			logger.Warnf("wallet checkout creation unknown recharge_id=%d err=%v", recharge.ID, err)
			updated, markErr := s.markCreateUnknown(ctx, recharge)
			if markErr != nil {
				return nil, markErr
			}
			return &wallet.CreateRechargeResp{Recharge: rechargeViewOf(updated)}, nil
		}
	}

	expectation := payment.CheckoutExpectation{
		Environment:    environment,
		ProductID:      profile.GetProductId(),
		ProviderAmount: providerAmount,
		RequestID:      recharge.MerchantOrderNo,
	}
	if err := payment.ValidateCheckout(checkout, expectation); err != nil {
		// 渠道已建单但响应与本单不符：同样只能进 REVIEW，且必须留下可排查的记录。
		logger.Warnf("wallet checkout contract violation recharge_id=%d checkout_id=%s err=%v", recharge.ID, checkout.ID, err)
		updated, markErr := s.markCreateUnknown(ctx, recharge)
		if markErr != nil {
			return nil, markErr
		}
		return &wallet.CreateRechargeResp{Recharge: rechargeViewOf(updated)}, nil
	}

	updated, err := s.persistCheckout(ctx, recharge, checkout)
	if err != nil {
		return nil, err
	}
	return &wallet.CreateRechargeResp{Recharge: rechargeViewOf(updated)}, nil
}

// newRechargeRow 用同一份快照固化双金额、汇率与 Profile 代际。
func (s *CommerceWalletService) newRechargeRow(
	ctx context.Context,
	tx *ent.Tx,
	walletID uint64,
	requestID, provider string,
	profile interface {
		GetEnvironment() string
		GetProfileId() string
		GetProfileRevision() int64
		GetProductId() string
	},
	rate Rate,
	walletAmount, providerAmount int64,
) (*ent.WalletRecharge, error) {
	now := time.Now()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		created, err := tx.WalletRecharge.Create().
			SetWalletID(walletID).
			SetRequestID(requestID).
			SetMerchantOrderNo(newMerchantOrderNo()).
			SetProvider(provider).
			SetProviderEnvironment(profile.GetEnvironment()).
			SetPaymentProfileID(profile.GetProfileId()).
			SetPaymentProfileRevision(profile.GetProfileRevision()).
			SetProviderProductID(profile.GetProductId()).
			SetWalletAmount(walletAmount).
			SetWalletCurrency(currencyCNY).
			SetProviderAmount(providerAmount).
			SetProviderCurrency(currencyUSD).
			SetFxRateNumerator(rate.Numerator).
			SetFxRateDenominator(rate.Denominator).
			SetFxSettingRevision(s.settingRevision()).
			SetFxQuotedAt(now).
			SetFxRoundingMode(fxRoundingMode).
			SetStatus(rechargeStatusCreated).
			Save(ctx)
		if err == nil {
			return created, nil
		}
		if !ent.IsConstraintError(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

// settingRevision 返回用于报价审计的 Setting 代际。
func (s *CommerceWalletService) settingRevision() int64 {
	if snapshot, ok := s.Runtime.Snapshot(); ok {
		if revision := snapshot.GetRevision(); revision > 0 {
			return revision
		}
	}
	return 1
}

// persistCheckout 把 Checkout 结果用 revision CAS 写回，订单已被回调推进时不回退。
func (s *CommerceWalletService) persistCheckout(
	ctx context.Context,
	recharge *ent.WalletRecharge,
	checkout payment.CheckoutResult,
) (*ent.WalletRecharge, error) {
	var updated *ent.WalletRecharge
	err := s.repository().runInTx(ctx, func(tx *ent.Tx) error {
		current, err := lockRecharge(ctx, tx, recharge.ID)
		if err != nil {
			return err
		}
		// 回调可能早于创建响应：已经离开 CREATED 的订单只补 Provider ID，不改状态。
		if current.Status != rechargeStatusCreated {
			if err := persistProviderIDs(ctx, tx, current, providerObservation{
				checkoutID:    checkout.ID,
				orderID:       orderIDOf(checkout),
				transactionID: transactionIDOf(checkout),
			}); err != nil {
				return err
			}
			updated, err = lockRecharge(ctx, tx, current.ID)
			return err
		}
		updated, err = transitionRecharge(ctx, tx, current, rechargeStatusPendingPayment, func(update *ent.WalletRechargeUpdate) {
			update.SetProviderCheckoutID(checkout.ID).
				SetCheckoutURL(checkout.URL)
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
		return nil, wrapInternal("create recharge", err)
	}
	return updated, nil
}

// failRechargeCreation 把本地明确失败落为 FAILED；已推进的订单保持原状。
func (s *CommerceWalletService) failRechargeCreation(
	ctx context.Context,
	recharge *ent.WalletRecharge,
	_ error,
) (*wallet.CreateRechargeResp, error) {
	if _, err := s.transitionIfStatus(ctx, recharge.ID, rechargeStatusCreated, rechargeStatusFailed, nil); err != nil {
		return nil, err
	}
	return nil, walletUnavailable("payment checkout could not be created; recharge was not started")
}

// markCreateUnknown 把无法确认结果的创建转入 REVIEW。
func (s *CommerceWalletService) markCreateUnknown(
	ctx context.Context,
	recharge *ent.WalletRecharge,
) (*ent.WalletRecharge, error) {
	var updated *ent.WalletRecharge
	err := s.repository().runInTx(ctx, func(tx *ent.Tx) error {
		current, err := lockRecharge(ctx, tx, recharge.ID)
		if err != nil {
			return err
		}
		if current.Status != rechargeStatusCreated {
			updated = current
			return nil
		}
		now := time.Now()
		updated, err = transitionRecharge(ctx, tx, current, rechargeStatusReview, func(update *ent.WalletRechargeUpdate) {
			update.SetReviewReason("checkout creation result is unknown").
				SetReviewedBy(actorProviderWebhook).
				SetReviewedAt(now)
		})
		return err
	})
	if err != nil {
		return nil, wrapInternal("create recharge", err)
	}
	return updated, nil
}

// transitionIfStatus 在事务内校验当前状态后推进；状态已变化时返回现状。
func (s *CommerceWalletService) transitionIfStatus(
	ctx context.Context,
	rechargeID uint64,
	from, to string,
	apply func(*ent.WalletRechargeUpdate),
) (*ent.WalletRecharge, error) {
	var updated *ent.WalletRecharge
	err := s.repository().runInTx(ctx, func(tx *ent.Tx) error {
		current, err := lockRecharge(ctx, tx, rechargeID)
		if err != nil {
			return err
		}
		if current.Status != from {
			updated = current
			return nil
		}
		updated, err = transitionRecharge(ctx, tx, current, to, apply)
		return err
	})
	if err != nil {
		return nil, wrapInternal("update recharge", err)
	}
	return updated, nil
}

// getRecharge 实现用户侧单笔充值查询，只允许读取自己的订单。
func (s *CommerceWalletService) getRecharge(
	ctx context.Context,
	req *wallet.GetRechargeReq,
) (*wallet.GetRechargeResp, error) {
	recharge, err := s.userRecharge(ctx, req.GetUserId(), req.GetRechargeId())
	if err != nil {
		return nil, err
	}
	return &wallet.GetRechargeResp{Recharge: rechargeViewOf(recharge)}, nil
}

// userRecharge 解析 user_id + recharge_id 并校验归属；不存在与不归属统一按 NOT_FOUND。
func (s *CommerceWalletService) userRecharge(
	ctx context.Context,
	rawUserID, rawRechargeID string,
) (*ent.WalletRecharge, error) {
	userID, err := parseDecimalID("user_id", rawUserID)
	if err != nil {
		return nil, err
	}
	rechargeID, err := parseDecimalID("recharge_id", rawRechargeID)
	if err != nil {
		return nil, err
	}
	account, err := s.walletByUser(ctx, userID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("recharge was not found")
	}
	if err != nil {
		return nil, wrapInternal("get recharge", err)
	}
	recharge, err := s.DB.Recharge.Get(ctx, rechargeID)
	if ent.IsNotFound(err) || (err == nil && recharge.WalletID != account.ID) {
		return nil, walletNotFound("recharge was not found")
	}
	if err != nil {
		return nil, wrapInternal("get recharge", err)
	}
	return recharge, nil
}

// listRecharges 实现用户侧充值列表，按 (created_at, id) 稳定游标分页。
func (s *CommerceWalletService) listRecharges(
	ctx context.Context,
	req *wallet.ListRechargesReq,
) (*wallet.ListRechargesResp, error) {
	userID, err := parseDecimalID("user_id", req.GetUserId())
	if err != nil {
		return nil, err
	}
	pageSize, err := normalizePageSize(req.GetPageSize())
	if err != nil {
		return nil, err
	}
	position, err := decodeCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}
	status := ""
	if req.Status != nil {
		status, err = rechargeStatusName(req.GetStatus())
		if err != nil {
			return nil, err
		}
	}

	account, err := s.walletByUser(ctx, userID)
	if ent.IsNotFound(err) {
		return &wallet.ListRechargesResp{Items: []*wallet.RechargeView{}, HasMore: false}, nil
	}
	if err != nil {
		return nil, wrapInternal("list recharges", err)
	}

	query := s.DB.Recharge.Query().Where(walletrecharge.WalletID(account.ID))
	if status != "" {
		query = query.Where(walletrecharge.Status(status))
	}
	query = applyRechargeCursor(query, position)

	recharges, hasMore, err := fetchRechargePage(ctx, query, pageSize)
	if err != nil {
		return nil, wrapInternal("list recharges", err)
	}
	items := make([]*wallet.RechargeView, 0, len(recharges))
	for _, recharge := range recharges {
		items = append(items, rechargeViewOf(recharge))
	}
	next := ""
	if hasMore && len(recharges) > 0 {
		last := recharges[len(recharges)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return &wallet.ListRechargesResp{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

// cancelRecharge 实现用户取消：只允许 CREATED/PENDING_PAYMENT，且只改本地状态。
//
// 取消不假设 Provider 一定不会付款；如果之后真的收到付款回调，仍按正常入账处理。
func (s *CommerceWalletService) cancelRecharge(
	ctx context.Context,
	req *wallet.CancelRechargeReq,
) (*wallet.CancelRechargeResp, error) {
	recharge, err := s.userRecharge(ctx, req.GetUserId(), req.GetRechargeId())
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var updated *ent.WalletRecharge
	err = s.repository().runInTx(ctx, func(tx *ent.Tx) error {
		current, err := lockRecharge(ctx, tx, recharge.ID)
		if err != nil {
			return err
		}
		switch current.Status {
		case rechargeStatusCanceled:
			updated = current
			return nil
		case rechargeStatusCreated, rechargeStatusPendingPayment:
		default:
			return stateConflict("recharge cannot be canceled in its current state")
		}
		updated, err = transitionRecharge(ctx, tx, current, rechargeStatusCanceled, func(update *ent.WalletRechargeUpdate) {
			update.SetCanceledAt(now)
		})
		return err
	})
	if err != nil {
		return nil, wrapInternal("cancel recharge", err)
	}
	return &wallet.CancelRechargeResp{Recharge: rechargeViewOf(updated)}, nil
}

// adminGetRecharge 实现管理员单笔充值查询。
func (s *CommerceWalletService) adminGetRecharge(
	ctx context.Context,
	req *wallet.AdminGetRechargeReq,
) (*wallet.AdminGetRechargeResp, error) {
	rechargeID, err := parseDecimalID("recharge_id", req.GetRechargeId())
	if err != nil {
		return nil, err
	}
	recharge, err := s.DB.Recharge.Get(ctx, rechargeID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("recharge was not found")
	}
	if err != nil {
		return nil, wrapInternal("get recharge", err)
	}
	account, err := s.DB.Account.Get(ctx, recharge.WalletID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("recharge was not found")
	}
	if err != nil {
		return nil, wrapInternal("get recharge", err)
	}
	return &wallet.AdminGetRechargeResp{Recharge: adminRechargeViewOf(recharge, account.UserID)}, nil
}

// adminListRecharges 实现管理员充值列表，可按用户、状态与 Provider 过滤。
func (s *CommerceWalletService) adminListRecharges(
	ctx context.Context,
	req *wallet.AdminListRechargesReq,
) (*wallet.AdminListRechargesResp, error) {
	pageSize, err := normalizePageSize(req.GetPageSize())
	if err != nil {
		return nil, err
	}
	position, err := decodeCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	query := s.DB.Recharge.Query()
	if req.UserId != nil && strings.TrimSpace(*req.UserId) != "" {
		userID, parseErr := parseDecimalID("user_id", *req.UserId)
		if parseErr != nil {
			return nil, parseErr
		}
		account, accountErr := s.walletByUser(ctx, userID)
		if ent.IsNotFound(accountErr) {
			return &wallet.AdminListRechargesResp{Items: []*wallet.AdminRechargeView{}, HasMore: false}, nil
		}
		if accountErr != nil {
			return nil, wrapInternal("list recharges", accountErr)
		}
		query = query.Where(walletrecharge.WalletID(account.ID))
	}
	if req.Status != nil {
		status, parseErr := rechargeStatusName(req.GetStatus())
		if parseErr != nil {
			return nil, parseErr
		}
		query = query.Where(walletrecharge.Status(status))
	}
	if req.Provider != nil {
		provider, parseErr := providerCode(req.GetProvider())
		if parseErr != nil {
			return nil, parseErr
		}
		query = query.Where(walletrecharge.Provider(provider))
	}
	query = applyRechargeCursor(query, position)

	recharges, hasMore, err := fetchRechargePage(ctx, query, pageSize)
	if err != nil {
		return nil, wrapInternal("list recharges", err)
	}

	walletIDs := make([]uint64, 0, len(recharges))
	seen := make(map[uint64]struct{}, len(recharges))
	for _, recharge := range recharges {
		if _, ok := seen[recharge.WalletID]; ok {
			continue
		}
		seen[recharge.WalletID] = struct{}{}
		walletIDs = append(walletIDs, recharge.WalletID)
	}
	userIDByWallet := make(map[uint64]uint64, len(walletIDs))
	if len(walletIDs) > 0 {
		accounts, err := s.DB.Account.Query().Where(walletaccount.IDIn(walletIDs...)).All(ctx)
		if err != nil {
			return nil, wrapInternal("list recharges", err)
		}
		for _, account := range accounts {
			userIDByWallet[account.ID] = account.UserID
		}
	}

	items := make([]*wallet.AdminRechargeView, 0, len(recharges))
	for _, recharge := range recharges {
		items = append(items, adminRechargeViewOf(recharge, userIDByWallet[recharge.WalletID]))
	}
	next := ""
	if hasMore && len(recharges) > 0 {
		last := recharges[len(recharges)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return &wallet.AdminListRechargesResp{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

// applyRechargeCursor 追加严格的 (created_at, id) 游标过滤。
func applyRechargeCursor(
	query *ent.WalletRechargeQuery,
	position cursor,
) *ent.WalletRechargeQuery {
	if position.empty() {
		return query
	}
	return query.Where(walletrecharge.Or(
		walletrecharge.CreatedAtLT(position.createdAt),
		walletrecharge.And(
			walletrecharge.CreatedAtEQ(position.createdAt),
			walletrecharge.IDLT(position.id),
		),
	))
}

// fetchRechargePage 取 pageSize+1 条判断 has_more 并截断。
func fetchRechargePage(
	ctx context.Context,
	query *ent.WalletRechargeQuery,
	pageSize int,
) ([]*ent.WalletRecharge, bool, error) {
	recharges, err := query.
		Order(walletrecharge.ByCreatedAt(entsql.OrderDesc()), walletrecharge.ByID(entsql.OrderDesc())).
		Limit(pageSize + 1).
		All(ctx)
	if err != nil {
		return nil, false, err
	}
	hasMore := len(recharges) > pageSize
	if hasMore {
		recharges = recharges[:pageSize]
	}
	return recharges, hasMore, nil
}

// newMerchantOrderNo 生成对外可展示、可对账的商户订单号。
func newMerchantOrderNo() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "cw" + formatDecimalID(uint64(time.Now().UnixNano()))
	}
	return "cw" + hex.EncodeToString(buffer)
}

// orderIDOf 提取 Checkout 响应中的 Order ID。
func orderIDOf(checkout payment.CheckoutResult) string {
	if checkout.Order == nil {
		return ""
	}
	return checkout.Order.ID
}

// transactionIDOf 提取 Checkout 响应中的 Transaction ID。
func transactionIDOf(checkout payment.CheckoutResult) string {
	if checkout.Order == nil {
		return ""
	}
	return checkout.Order.TransactionID
}
