package wallet

import (
	"context"
	"errors"
	"time"

	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/walletrecharge"
	"github.com/KiClover/orzAdmin/ent/wallettransaction"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	settings "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
)

// 充值状态机与 Provider 事实收敛。
//
// 设计 §8：充值订单的本地状态只能由「已查询并核验过的 Provider 事实」推进；
// 创建请求、回调和管理员刷新共用同一组状态应用函数，避免三套写入路径。

const (
	// 本地充值状态（数据库中使用大写枚举，与设计 §6.3 一致）。
	rechargeStatusCreated        = "CREATED"
	rechargeStatusPendingPayment = "PENDING_PAYMENT"
	rechargeStatusSucceeded      = "SUCCEEDED"
	rechargeStatusCanceled       = "CANCELED"
	rechargeStatusFailed         = "FAILED"
	rechargeStatusReview         = "REVIEW"
	rechargeStatusRefunding      = "REFUNDING"
	rechargeStatusRefunded       = "REFUNDED"
)

const (
	// 首期只接入 Creem。
	providerNameCreem = "CREEM"

	// Provider 环境取值固定为设计约定的大写枚举。
	providerEnvironmentTest       = "TEST"
	providerEnvironmentProduction = "PRODUCTION"

	// fxRoundingMode 是唯一允许的换算舍入模式。
	fxRoundingMode = "CEIL_PROVIDER_MINOR_UNIT"

	// 充值入账与退款冲回使用不可变的业务键类型。
	businessTypeRecharge       = "recharge"
	businessTypeRechargeRefund = "recharge_refund"

	// 充值业务的审计 actor：Provider 回调、Provider 退款、拒付证据与人工刷新。
	actorProviderWebhook    = "provider:webhook"
	actorProviderRefund     = "provider:refund"
	actorProviderChargeback = "provider:chargeback"
	actorSystemRefresh      = "system:refresh"

	// requestIDMaxLen 与 schema 中 request_id 的列宽一致。
	requestIDMaxLen = 64
	// rechargeReviewReasonMaxLen 与 schema 中 review_reason 的列宽一致。
	rechargeReviewReasonMaxLen = 512
)

// errRechargeStateConflict 表示订单在并发处理中已经被其他路径推进，当前事实不再适用。
var errRechargeStateConflict = errors.New("wallet recharge state changed concurrently")

// rechargeStatusProto 把数据库状态映射成对外枚举；未知状态按未指定处理。
func rechargeStatusProto(name string) wallet.RechargeStatus {
	switch name {
	case rechargeStatusCreated:
		return wallet.RechargeStatus_RECHARGE_STATUS_CREATED
	case rechargeStatusPendingPayment:
		return wallet.RechargeStatus_RECHARGE_STATUS_PENDING_PAYMENT
	case rechargeStatusSucceeded:
		return wallet.RechargeStatus_RECHARGE_STATUS_SUCCEEDED
	case rechargeStatusCanceled:
		return wallet.RechargeStatus_RECHARGE_STATUS_CANCELED
	case rechargeStatusFailed:
		return wallet.RechargeStatus_RECHARGE_STATUS_FAILED
	case rechargeStatusReview:
		return wallet.RechargeStatus_RECHARGE_STATUS_REVIEW
	case rechargeStatusRefunding:
		return wallet.RechargeStatus_RECHARGE_STATUS_REFUNDING
	case rechargeStatusRefunded:
		return wallet.RechargeStatus_RECHARGE_STATUS_REFUNDED
	default:
		return wallet.RechargeStatus_RECHARGE_STATUS_UNSPECIFIED
	}
}

// rechargeStatusName 把对外枚举映射成数据库状态；未指定与未知值一律拒绝。
func rechargeStatusName(status wallet.RechargeStatus) (string, error) {
	switch status {
	case wallet.RechargeStatus_RECHARGE_STATUS_CREATED:
		return rechargeStatusCreated, nil
	case wallet.RechargeStatus_RECHARGE_STATUS_PENDING_PAYMENT:
		return rechargeStatusPendingPayment, nil
	case wallet.RechargeStatus_RECHARGE_STATUS_SUCCEEDED:
		return rechargeStatusSucceeded, nil
	case wallet.RechargeStatus_RECHARGE_STATUS_CANCELED:
		return rechargeStatusCanceled, nil
	case wallet.RechargeStatus_RECHARGE_STATUS_FAILED:
		return rechargeStatusFailed, nil
	case wallet.RechargeStatus_RECHARGE_STATUS_REVIEW:
		return rechargeStatusReview, nil
	case wallet.RechargeStatus_RECHARGE_STATUS_REFUNDING:
		return rechargeStatusRefunding, nil
	case wallet.RechargeStatus_RECHARGE_STATUS_REFUNDED:
		return rechargeStatusRefunded, nil
	default:
		return "", invalidArgument("recharge status is not supported")
	}
}

// providerCode 把对外 Provider 枚举映射成数据库取值；首期只接受 Creem。
func providerCode(provider wallet.PaymentProvider) (string, error) {
	switch provider {
	case wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM:
		return providerNameCreem, nil
	default:
		return "", invalidArgument("payment provider is not supported")
	}
}

// providerFromCode 把数据库 Provider 取值映射成对外枚举。
func providerFromCode(code string) wallet.PaymentProvider {
	if code == providerNameCreem {
		return wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM
	}
	return wallet.PaymentProvider_PAYMENT_PROVIDER_UNSPECIFIED
}

// providerEnvironmentProto 把数据库环境映射成对外枚举。
func providerEnvironmentProto(name string) wallet.ProviderEnvironment {
	switch name {
	case providerEnvironmentTest:
		return wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST
	case providerEnvironmentProduction:
		return wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_PRODUCTION
	default:
		return wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_UNSPECIFIED
	}
}

// providerEnvironmentFromProfile 把 Setting Profile 的环境字符串转换为网关凭据枚举。
func providerEnvironmentFromProfile(environment string) (wallet.ProviderEnvironment, error) {
	switch environment {
	case providerEnvironmentTest:
		return wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST, nil
	case providerEnvironmentProduction:
		return wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_PRODUCTION, nil
	default:
		return wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_UNSPECIFIED,
			walletUnavailable("payment profile environment is not configured")
	}
}

// rechargeViewOf 映射用户可见的充值视图：不含归属、Provider ID 与人工处置字段。
func rechargeViewOf(recharge *ent.WalletRecharge) *wallet.RechargeView {
	return &wallet.RechargeView{
		Id:               formatDecimalID(recharge.ID),
		Status:           rechargeStatusProto(recharge.Status),
		WalletAmount:     recharge.WalletAmount,
		WalletCurrency:   recharge.WalletCurrency,
		ProviderAmount:   recharge.ProviderAmount,
		ProviderCurrency: recharge.ProviderCurrency,
		CnyPerUsdRate:    rechargeRateString(recharge),
		CheckoutUrl:      stringOrEmpty(recharge.CheckoutURL),
		CreatedAt:        recharge.CreatedAt.UTC().UnixMilli(),
		PaidAt:           millisOrZero(recharge.PaidAt),
		UpdatedAt:        recharge.UpdatedAt.UTC().UnixMilli(),
	}
}

// adminRechargeViewOf 在用户视图之上追加归属、冻结报价快照与人工处置字段。
func adminRechargeViewOf(recharge *ent.WalletRecharge, userID uint64) *wallet.AdminRechargeView {
	return &wallet.AdminRechargeView{
		Recharge:               rechargeViewOf(recharge),
		UserId:                 formatDecimalID(userID),
		WalletId:               formatDecimalID(recharge.WalletID),
		RequestId:              recharge.RequestID,
		MerchantOrderNo:        recharge.MerchantOrderNo,
		Provider:               providerFromCode(recharge.Provider),
		ProviderEnvironment:    providerEnvironmentProto(recharge.ProviderEnvironment),
		PaymentProfileId:       recharge.PaymentProfileID,
		PaymentProfileRevision: int64(recharge.PaymentProfileRevision),
		ProviderProductId:      recharge.ProviderProductID,
		ProviderCheckoutId:     stringOrEmpty(recharge.ProviderCheckoutID),
		ProviderOrderId:        stringOrEmpty(recharge.ProviderOrderID),
		ProviderTransactionId:  stringOrEmpty(recharge.ProviderTransactionID),
		FxRateNumerator:        recharge.FxRateNumerator,
		FxRateDenominator:      recharge.FxRateDenominator,
		FxSettingRevision:      int64(recharge.FxSettingRevision),
		FxQuotedAt:             millisOrZero(&recharge.FxQuotedAt),
		FxRoundingMode:         recharge.FxRoundingMode,
		ReviewReason:           stringOrEmpty(recharge.ReviewReason),
		ReviewBlocksDebit:      recharge.ReviewBlocksDebit,
		ReviewedBy:             stringOrEmpty(recharge.ReviewedBy),
		ReviewedAt:             millisOrZero(recharge.ReviewedAt),
		RefundReason:           stringOrEmpty(recharge.RefundReason),
		RefundRequestedBy:      stringOrEmpty(recharge.RefundRequestedBy),
		RefundRequestedAt:      millisOrZero(recharge.RefundRequestedAt),
		Revision:               int64(recharge.Revision),
		CanceledAt:             millisOrZero(recharge.CanceledAt),
		RefundedAt:             millisOrZero(recharge.RefundedAt),
	}
}

// rechargeRateString 从冻结的分子/分母重建展示汇率；异常数据返回空串，避免伪造报价。
func rechargeRateString(recharge *ent.WalletRecharge) string {
	canonical, err := FormatRate(recharge.FxRateNumerator, recharge.FxRateDenominator)
	if err != nil {
		return ""
	}
	return canonical
}

// millisOrZero 把可空时间转换为 Unix 毫秒，未设置返回 0。
func millisOrZero(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.UTC().UnixMilli()
}

// stringOrEmpty 把可空字符串转换为空串，避免视图出现 nil。
func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// stringPointer 返回字符串指针，用于可选协议字段。
func stringPointer(value string) *string {
	return &value
}

// rechargeProfile 从同一份 Setting 快照中选择充值使用的 Profile。
//
// 模式与 Profile 必须来自同一快照：TEST 只能用 TEST Profile，PRODUCTION 只能用
// PRODUCTION Profile，避免用测试凭据产生真实收款。
func (s *CommerceWalletService) rechargeProfile(
	snapshot *settings.WalletRuntimeSettings,
) (*settings.WalletProviderProfileRuntime, wallet.ProviderEnvironment, error) {
	var (
		profile *settings.WalletProviderProfileRuntime
		env     wallet.ProviderEnvironment
	)
	switch snapshot.GetRechargeMode() {
	case rechargeModeTest:
		profile = snapshot.GetTestProfile()
		env = wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST
	case rechargeModeProduction:
		profile = snapshot.GetProductionProfile()
		env = wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_PRODUCTION
	default:
		return nil, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_UNSPECIFIED,
			featureDisabled(reasonRechargeDisabled)
	}
	if profile == nil || profile.GetProfileId() == "" || profile.GetApiKey() == "" || profile.GetProductId() == "" {
		return nil, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_UNSPECIFIED,
			walletUnavailable("recharge profile is not configured")
	}
	if _, err := providerEnvironmentFromProfile(profile.GetEnvironment()); err != nil {
		return nil, wallet.ProviderEnvironment_PROVIDER_ENVIRONMENT_UNSPECIFIED, err
	}
	return profile, env, nil
}

// snapshotRate 从快照构造冻结汇率；整数分子/分母优先，缺失时回退到十进制字符串。
func snapshotRate(snapshot *settings.WalletRuntimeSettings) (Rate, error) {
	if snapshot.GetCnyPerUsdNumerator() > 0 && snapshot.GetCnyPerUsdDenominator() > 0 {
		rate, err := RateFromFraction(snapshot.GetCnyPerUsdNumerator(), snapshot.GetCnyPerUsdDenominator())
		if err != nil {
			return Rate{}, walletUnavailable("wallet exchange rate is not configured")
		}
		return rate, nil
	}
	rate, err := ParseRate(snapshot.GetCnyPerUsdRate())
	if err != nil {
		return Rate{}, walletUnavailable("wallet exchange rate is not configured")
	}
	return rate, nil
}

// gatewayCredentials 用订单冻结的 Profile 构造网关凭据。
//
// SuccessURL 来自该 Product 的 default_success_url（创建前再次从 Provider 读取），
// 不接受请求体或用户提供跳转地址。
func gatewayCredentials(
	profile *settings.WalletProviderProfileRuntime,
	env wallet.ProviderEnvironment,
	successURL string,
) payment.GatewayCredentials {
	return payment.GatewayCredentials{
		Environment: env,
		APIKey:      profile.GetApiKey(),
		ProductID:   profile.GetProductId(),
		SuccessURL:  successURL,
	}
}

// profileByID 在快照中按 Profile ID 与环境定位 Profile，供回调与退款复用冻结凭据。
func profileByID(
	snapshot *settings.WalletRuntimeSettings,
	environment, profileID string,
) *settings.WalletProviderProfileRuntime {
	if snapshot == nil {
		return nil
	}
	for _, profile := range []*settings.WalletProviderProfileRuntime{
		snapshot.GetTestProfile(),
		snapshot.GetProductionProfile(),
	} {
		if profile == nil {
			continue
		}
		if profile.GetProfileId() == profileID && profile.GetEnvironment() == environment {
			return profile
		}
	}
	return nil
}

// lockRecharge 在事务内锁订单行；充值状态推进必须持有该锁。
func lockRecharge(ctx context.Context, tx *ent.Tx, rechargeID uint64) (*ent.WalletRecharge, error) {
	recharge, err := tx.WalletRecharge.Query().
		Where(walletrecharge.ID(rechargeID)).
		ForUpdate().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("recharge was not found")
	}
	if err != nil {
		return nil, err
	}
	return recharge, nil
}

// transitionRecharge 用 revision 做乐观并发控制推进订单；调用方必须持有订单行锁。
func transitionRecharge(
	ctx context.Context,
	tx *ent.Tx,
	recharge *ent.WalletRecharge,
	status string,
	apply func(*ent.WalletRechargeUpdate),
) (*ent.WalletRecharge, error) {
	update := tx.WalletRecharge.Update().
		Where(
			walletrecharge.ID(recharge.ID),
			walletrecharge.Revision(recharge.Revision),
		).
		SetStatus(status)
	if apply != nil {
		apply(update)
	}
	affected, err := update.AddRevision(1).Save(ctx)
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, errRechargeStateConflict
	}
	return lockRecharge(ctx, tx, recharge.ID)
}

// persistProviderIDs 固化本轮观察到的 Provider 对象 ID；已存在且不一致时拒绝。
func persistProviderIDs(
	ctx context.Context,
	tx *ent.Tx,
	recharge *ent.WalletRecharge,
	obs providerObservation,
) error {
	update := tx.WalletRecharge.Update().Where(
		walletrecharge.ID(recharge.ID),
		walletrecharge.Revision(recharge.Revision),
	)
	changed := false
	setIfChanged := func(current *string, observed string, set func(*ent.WalletRechargeUpdate, string)) error {
		if observed == "" {
			return nil
		}
		existing := stringOrEmpty(current)
		if existing == observed {
			return nil
		}
		if existing != "" {
			return stateConflict("provider object does not match the frozen recharge")
		}
		set(update, observed)
		changed = true
		return nil
	}
	if err := setIfChanged(recharge.ProviderCheckoutID, obs.checkoutID, func(u *ent.WalletRechargeUpdate, v string) {
		u.SetProviderCheckoutID(v)
	}); err != nil {
		return err
	}
	if err := setIfChanged(recharge.ProviderOrderID, obs.orderID, func(u *ent.WalletRechargeUpdate, v string) {
		u.SetProviderOrderID(v)
	}); err != nil {
		return err
	}
	if err := setIfChanged(recharge.ProviderTransactionID, obs.transactionID, func(u *ent.WalletRechargeUpdate, v string) {
		u.SetProviderTransactionID(v)
	}); err != nil {
		return err
	}
	if !changed {
		return nil
	}
	affected, err := update.AddRevision(1).Save(ctx)
	if err != nil {
		return err
	}
	if affected != 1 {
		return errRechargeStateConflict
	}
	return nil
}

// providerObservation 是一次经核验的 Provider 事实快照。
type providerObservation struct {
	checkoutStatus payment.CheckoutStatus
	checkoutID     string
	orderID        string
	transactionID  string
	transaction    *payment.TransactionResult
}

// rechargeCredited 判断订单是否已经写过 RECHARGE_CREDIT。
func (s *CommerceWalletService) rechargeCredited(
	ctx context.Context,
	tx *ent.Tx,
	recharge *ent.WalletRecharge,
) (bool, error) {
	return tx.WalletTransaction.Query().
		Where(
			wallettransaction.OwnerNamespace(s.Cfg.Wallet.OwnerNamespace),
			wallettransaction.Type(txTypeRechargeCredit),
			wallettransaction.BusinessType(businessTypeRecharge),
			wallettransaction.BusinessID(formatDecimalID(recharge.ID)),
		).
		Exist(ctx)
}

// ensureRechargeCredit 幂等写入 RECHARGE_CREDIT 并增加余额。
//
// 金额永远取订单冻结的 CNY 分，不读取当前汇率；已有同键流水时只校验一致性。
func (s *CommerceWalletService) ensureRechargeCredit(
	ctx context.Context,
	tx *ent.Tx,
	recharge *ent.WalletRecharge,
	actor string,
) (*ent.WalletTransaction, error) {
	namespace := s.Cfg.Wallet.OwnerNamespace
	command := balanceCommand{
		UserID:          0,
		Currency:        recharge.WalletCurrency,
		TransactionType: txTypeRechargeCredit,
		Amount:          recharge.WalletAmount,
		OwnerNamespace:  namespace,
		BusinessType:    businessTypeRecharge,
		BusinessID:      formatDecimalID(recharge.ID),
		Description:     "recharge credit",
		Actor:           actor,
	}
	existing, err := s.repository().findTransaction(ctx, tx, command)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if !replayMatches(recordOf(existing), recharge.WalletID, command) {
			return nil, idempotencyConflict()
		}
		return existing, nil
	}

	account, err := s.repository().lockWalletByID(ctx, tx, recharge.WalletID)
	if err != nil {
		return nil, err
	}
	newBalance, ok := addAmount(account.BalanceAmount, recharge.WalletAmount)
	if !ok {
		return nil, walletInternal("wallet balance overflow")
	}
	if err := s.repository().applyBalance(ctx, tx, account, newBalance); err != nil {
		return nil, err
	}
	command.UserID = account.UserID
	return s.repository().insertTransaction(ctx, tx, command, recharge.WalletID, recharge.WalletAmount, newBalance)
}

// ensureRechargeRefund 幂等写入 RECHARGE_REFUND 并扣回余额。
//
// 金额恒等于原 RECHARGE_CREDIT 的绝对值，并关联原交易，数据库唯一索引保证只成功一次。
func (s *CommerceWalletService) ensureRechargeRefund(
	ctx context.Context,
	tx *ent.Tx,
	recharge *ent.WalletRecharge,
	actor string,
) (*ent.WalletTransaction, error) {
	namespace := s.Cfg.Wallet.OwnerNamespace
	credit, err := tx.WalletTransaction.Query().
		Where(
			wallettransaction.OwnerNamespace(namespace),
			wallettransaction.Type(txTypeRechargeCredit),
			wallettransaction.BusinessType(businessTypeRecharge),
			wallettransaction.BusinessID(formatDecimalID(recharge.ID)),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, stateConflict("recharge credit has not been recorded")
	}
	if err != nil {
		return nil, err
	}

	command := balanceCommand{
		Currency:             credit.Currency,
		TransactionType:      txTypeRechargeRefund,
		Amount:               absAmount(credit.DeltaAmount),
		OwnerNamespace:       namespace,
		BusinessType:         businessTypeRechargeRefund,
		BusinessID:           formatDecimalID(recharge.ID),
		RelatedTransactionID: &credit.ID,
		Description:          "recharge refund",
		Actor:                actor,
	}
	existing, err := s.repository().findTransaction(ctx, tx, command)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if !replayMatches(recordOf(existing), recharge.WalletID, command) {
			return nil, idempotencyConflict()
		}
		return existing, nil
	}

	account, err := s.repository().lockWalletByID(ctx, tx, recharge.WalletID)
	if err != nil {
		return nil, err
	}
	delta, err := signedDelta(command.TransactionType, command.Amount)
	if err != nil {
		return nil, err
	}
	newBalance, ok := addAmount(account.BalanceAmount, delta)
	if !ok {
		return nil, walletInternal("wallet balance overflow")
	}
	if err := s.repository().applyBalance(ctx, tx, account, newBalance); err != nil {
		return nil, err
	}
	return s.repository().insertTransaction(ctx, tx, command, recharge.WalletID, delta, newBalance)
}

// markRechargeReview 把订单收敛到 REVIEW；blockDebit 同时设置该钱包的扣款阻断。
func (s *CommerceWalletService) markRechargeReview(
	ctx context.Context,
	tx *ent.Tx,
	recharge *ent.WalletRecharge,
	actor, reason string,
	blockDebit bool,
) (*ent.WalletRecharge, error) {
	now := time.Now()
	return transitionRecharge(ctx, tx, recharge, rechargeStatusReview, func(update *ent.WalletRechargeUpdate) {
		update.SetReviewReason(truncate(reason, rechargeReviewReasonMaxLen)).
			SetReviewedBy(actor).
			SetReviewedAt(now)
		if blockDebit {
			update.SetReviewBlocksDebit(true)
		}
	})
}

// truncate 按字节截断审计文案，保证不超出列宽。
func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

// truncatedObservationError 把 Provider 状态映射成稳定的本地收敛动作。
//
// 返回值定义：
//   - 订单已 SUCCEEDED 且交易已全额退款：先补偿入账再冲回，最终 REFUNDED；
//   - 交易仍为 paid：幂等入账并 SUCCEEDED；
//   - 部分退款/拒付/争议：REVIEW 并阻断扣款；
//   - 未入账的失败类终态：FAILED；已入账订单遇到失败类终态：REVIEW；
//   - 其余未知组合：REVIEW，等待人工刷新。
func (s *CommerceWalletService) applyProviderObservation(
	ctx context.Context,
	rechargeID uint64,
	obs providerObservation,
	actor, reason string,
) (*ent.WalletRecharge, error) {
	var updated *ent.WalletRecharge
	err := s.repository().runInTx(ctx, func(tx *ent.Tx) error {
		current, err := lockRecharge(ctx, tx, rechargeID)
		if err != nil {
			return err
		}
		if err := persistProviderIDs(ctx, tx, current, obs); err != nil {
			return err
		}
		current, err = lockRecharge(ctx, tx, rechargeID)
		if err != nil {
			return err
		}

		credited, err := s.rechargeCredited(ctx, tx, current)
		if err != nil {
			return err
		}

		transaction := obs.transaction
		switch {
		case transaction != nil && transaction.Status == payment.TransactionStatusPaid:
			updated, err = s.applyPaidObservation(ctx, tx, current, obs, credited, actor, reason)

		case transaction != nil && transaction.Status == payment.TransactionStatusRefunded:
			updated, err = s.applyRefundedObservation(ctx, tx, current, obs, credited, actor, reason)

		case transaction != nil && (transaction.Status == payment.TransactionStatusPartial ||
			transaction.Status == payment.TransactionStatusChargeback):
			updated, err = s.markRechargeReview(ctx, tx, current,
				actor, "provider reported a partial refund or chargeback: "+reason, true)

		case transaction != nil && (transaction.Status == payment.TransactionStatusDeclined ||
			transaction.Status == payment.TransactionStatusCanceled ||
			transaction.Status == payment.TransactionStatusVoided ||
			transaction.Status == payment.TransactionStatusUncollect):
			updated, err = s.applyRejectedObservation(ctx, tx, current, credited, actor, reason)

		case transaction != nil && transaction.Status == payment.TransactionStatusPending:
			updated, err = s.applyUnpaidObservation(ctx, tx, current, obs, credited)

		default:
			updated, err = s.applyUnpaidObservation(ctx, tx, current, obs, credited)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// applyPaidObservation 处理权威 Transaction=paid 的事实。
func (s *CommerceWalletService) applyPaidObservation(
	ctx context.Context,
	tx *ent.Tx,
	current *ent.WalletRecharge,
	obs providerObservation,
	credited bool,
	actor, reason string,
) (*ent.WalletRecharge, error) {
	// REFUNDING/REFUNDED 已进入退款链路，paid 事实不能把订单拉回 SUCCEEDED。
	if current.Status == rechargeStatusRefunding || current.Status == rechargeStatusRefunded {
		return current, nil
	}
	if !credited {
		if _, err := s.ensureRechargeCredit(ctx, tx, current, actor); err != nil {
			return nil, err
		}
	}
	if current.Status == rechargeStatusSucceeded {
		return current, nil
	}
	now := time.Now()
	return transitionRecharge(ctx, tx, current, rechargeStatusSucceeded, func(update *ent.WalletRechargeUpdate) {
		update.SetPaidAt(now)
		if obs.transactionID != "" {
			update.SetProviderTransactionID(obs.transactionID)
		}
		if obs.orderID != "" {
			update.SetProviderOrderID(obs.orderID)
		}
	})
}

// applyRefundedObservation 处理权威 Transaction=refunded 的事实。
//
// 只有「已支付且退款额等于实付额」才视为全额退款；恢复场景允许在同一事务内
// 先补记 RECHARGE_CREDIT 再写 RECHARGE_REFUND，净额为零。
func (s *CommerceWalletService) applyRefundedObservation(
	ctx context.Context,
	tx *ent.Tx,
	current *ent.WalletRecharge,
	obs providerObservation,
	credited bool,
	actor, reason string,
) (*ent.WalletRecharge, error) {
	transaction := obs.transaction
	if transaction == nil || !fullRefundSettled(transaction, current.ProviderAmount) {
		return s.markRechargeReview(ctx, tx, current, actor,
			"provider reported a refund that does not match the frozen amount", true)
	}
	if _, err := s.ensureRechargeCredit(ctx, tx, current, actor); err != nil {
		return nil, err
	}
	if _, err := s.ensureRechargeRefund(ctx, tx, current, actor); err != nil {
		return nil, err
	}
	if current.Status == rechargeStatusRefunded {
		return current, nil
	}
	now := time.Now()
	return transitionRecharge(ctx, tx, current, rechargeStatusRefunded, func(update *ent.WalletRechargeUpdate) {
		update.SetRefundedAt(now)
		if obs.transactionID != "" {
			update.SetProviderTransactionID(obs.transactionID)
		}
		if obs.orderID != "" {
			update.SetProviderOrderID(obs.orderID)
		}
	})
}

// applyRejectedObservation 处理 declined/canceled/void/uncollectible 等失败类终态。
func (s *CommerceWalletService) applyRejectedObservation(
	ctx context.Context,
	tx *ent.Tx,
	current *ent.WalletRecharge,
	credited bool,
	actor, reason string,
) (*ent.WalletRecharge, error) {
	// 已入账订单不能因为 Provider 事后状态自动删改余额，只能进入人工审核。
	if credited || current.Status == rechargeStatusSucceeded || current.Status == rechargeStatusRefunding {
		return s.markRechargeReview(ctx, tx, current, actor,
			"provider reported a failure after the recharge was credited", false)
	}
	if current.Status == rechargeStatusFailed || current.Status == rechargeStatusCanceled {
		return current, nil
	}
	return transitionRecharge(ctx, tx, current, rechargeStatusFailed, func(update *ent.WalletRechargeUpdate) {
		update.SetReviewReason(truncate("provider reported: "+reason, rechargeReviewReasonMaxLen))
	})
}

// applyUnpaidObservation 处理尚无 paid Transaction 的 Checkout 状态。
func (s *CommerceWalletService) applyUnpaidObservation(
	ctx context.Context,
	tx *ent.Tx,
	current *ent.WalletRecharge,
	obs providerObservation,
	credited bool,
) (*ent.WalletRecharge, error) {
	switch obs.checkoutStatus {
	case payment.CheckoutStatusExpired:
		if credited {
			return current, nil
		}
		if current.Status == rechargeStatusFailed || current.Status == rechargeStatusCanceled {
			return current, nil
		}
		return transitionRecharge(ctx, tx, current, rechargeStatusFailed, nil)

	case payment.CheckoutStatusCompleted:
		// completed 只表示 Checkout 完成，不代表已收款；没有 paid Transaction 时人工核对。
		return s.markRechargeReview(ctx, tx, current,
			actorProviderWebhook, "checkout completed without a paid transaction", false)

	case payment.CheckoutStatusPending, payment.CheckoutStatusProcessing:
		if current.Status == rechargeStatusCreated {
			return transitionRecharge(ctx, tx, current, rechargeStatusPendingPayment, nil)
		}
		return current, nil

	default:
		// 未知 Checkout 状态：不猜测，进入人工审核。
		if current.Status == rechargeStatusReview {
			return current, nil
		}
		return s.markRechargeReview(ctx, tx, current,
			actorProviderWebhook, "provider returned an unknown checkout status", false)
	}
}

// fullRefundSettled 判断 Provider 是否已确认全额退款。
func fullRefundSettled(transaction *payment.TransactionResult, frozenAmount int64) bool {
	if transaction == nil {
		return false
	}
	if transaction.Amount != frozenAmount || transaction.Currency != currencyUSD {
		return false
	}
	if transaction.AmountPaid == nil || transaction.RefundedAmount == nil {
		return false
	}
	return *transaction.AmountPaid == frozenAmount && *transaction.RefundedAmount == frozenAmount
}

// validateObservationAmount 校验 Provider 金额契约，防止把别的订单金额记到本单。
func validateObservationAmount(recharge *ent.WalletRecharge, transaction *payment.TransactionResult) error {
	if transaction == nil {
		return nil
	}
	if transaction.Currency != currencyUSD {
		return invalidArgument("provider transaction currency does not match")
	}
	if transaction.Amount != recharge.ProviderAmount {
		return invalidArgument("provider transaction amount does not match")
	}
	if transaction.AmountPaid != nil && *transaction.AmountPaid != recharge.ProviderAmount {
		return invalidArgument("provider transaction paid amount does not match")
	}
	if transaction.DiscountAmount != nil && *transaction.DiscountAmount != 0 {
		return invalidArgument("provider transaction discount is not supported")
	}
	return nil
}
