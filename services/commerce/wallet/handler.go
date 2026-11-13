package wallet

import (
	"context"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
	"github.com/cloudwego/kitex/pkg/kerrors"
	"go.uber.org/fx"
)

// ServiceName 是 Commerce Wallet 服务在服务发现中的名字。
const ServiceName = "commerce-wallet"

// errMsgNotImplemented 是契约占位方法的统一安全文案。
// 真实实现由后续任务补齐，占位方法不得产生任何副作用。
const errMsgNotImplemented = "wallet service method is not implemented yet"

// CommerceWalletService 承载 Commerce Wallet 的业务实现。
//
// 任务 4 实现查询、余额消费与同 owner 补偿；其余方法继续保持 NOT_IMPLEMENTED。
// 余额变更必须经过 applyBalanceChange，Handler 不得直接写账户或流水。
type CommerceWalletService struct {
	fx.In

	Cfg     *conf.Config
	DBAll   *ent.Client
	DB      *db.WalletServiceDB
	Runtime *Service
	Factory payment.PaymentGatewayFactory
	Catalog payment.ProductCatalog
}

var _ wallet.CommerceWalletService = (*CommerceWalletService)(nil)

func NewCommerceWalletService(svc CommerceWalletService) wallet.CommerceWalletService {
	return &svc
}

// notImplemented 返回稳定的“未实现”业务错误，错误码与 WalletErrorCode 对齐。
func notImplemented() error {
	return kerrors.NewBizStatusError(int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_NOT_IMPLEMENTED), errMsgNotImplemented)
}

// guard 是所有钱包 RPC 的统一入口检查（设计 §10.1）：
//   - readiness 失败（无可信快照且已有业务数据，或数据存在性无法确认）一律拒绝；
//   - 全新空库允许按内置关闭默认值提供只读查询；
//   - 其余动作按 runtime_settings.go 的动作策略表判定，拒绝时返回稳定安全文案。
func (s CommerceWalletService) guard(ctx context.Context, action Action) error {
	if s.Runtime == nil {
		return walletUnavailable("wallet service is not configured")
	}
	if err := s.Runtime.Ready(ctx); err != nil {
		return walletUnavailable("wallet service is unavailable")
	}
	if _, ok := s.Runtime.Snapshot(); !ok {
		if actionCategories[action] == categoryRead {
			return nil
		}
		return featureDisabled(reasonSettingsUnavailable)
	}
	if decision := s.Runtime.Decide(action); !decision.Allowed {
		return featureDisabled(decision.Reason)
	}
	return nil
}

// ============================ 用户类（7） ============================

func (s CommerceWalletService) GetWallet(ctx context.Context, req *wallet.GetWalletReq) (res *wallet.GetWalletResp, err error) {
	if err := s.guard(ctx, ActionQuery); err != nil {
		return nil, err
	}
	return s.getWallet(ctx, req)
}

func (s CommerceWalletService) ListTransactions(ctx context.Context, req *wallet.ListTransactionsReq) (res *wallet.ListTransactionsResp, err error) {
	if err := s.guard(ctx, ActionListTransactions); err != nil {
		return nil, err
	}
	return s.listTransactions(ctx, req)
}

func (s CommerceWalletService) CreateRecharge(ctx context.Context, req *wallet.CreateRechargeReq) (res *wallet.CreateRechargeResp, err error) {
	if err := s.guard(ctx, ActionCreateRecharge); err != nil {
		return nil, err
	}
	return s.createRecharge(ctx, req)
}

func (s CommerceWalletService) GetRecharge(ctx context.Context, req *wallet.GetRechargeReq) (res *wallet.GetRechargeResp, err error) {
	if err := s.guard(ctx, ActionGetRecharge); err != nil {
		return nil, err
	}
	return s.getRecharge(ctx, req)
}

func (s CommerceWalletService) ListRecharges(ctx context.Context, req *wallet.ListRechargesReq) (res *wallet.ListRechargesResp, err error) {
	if err := s.guard(ctx, ActionListRecharges); err != nil {
		return nil, err
	}
	return s.listRecharges(ctx, req)
}

func (s CommerceWalletService) CancelRecharge(ctx context.Context, req *wallet.CancelRechargeReq) (res *wallet.CancelRechargeResp, err error) {
	if err := s.guard(ctx, ActionCancelRecharge); err != nil {
		return nil, err
	}
	return s.cancelRecharge(ctx, req)
}

// RetryRechargeCheckout 为用户提供显式"重建支付链接"：只对没有 Provider 结果的
// 冻结订单生效，已有链接时幂等返回现状，绝不自动重试。
func (s CommerceWalletService) RetryRechargeCheckout(ctx context.Context, req *wallet.RetryRechargeCheckoutReq) (res *wallet.RetryRechargeCheckoutResp, err error) {
	if err := s.guard(ctx, ActionRetryRechargeCheckout); err != nil {
		return nil, err
	}
	return s.retryRechargeCheckout(ctx, req)
}

func (s CommerceWalletService) RedeemCode(ctx context.Context, req *wallet.RedeemCodeReq) (res *wallet.RedeemCodeResp, err error) {
	if err := s.guard(ctx, ActionRedeemCode); err != nil {
		return nil, err
	}
	return s.redeemCode(ctx, req)
}

// ============================ 可信内部类（3） ============================

// DebitBalance 实现用户余额消费：客户端只提交金额、币种与业务键，
// owner_namespace 与 actor 由服务端派生，重复业务键走幂等重放或冲突判定。
func (s CommerceWalletService) DebitBalance(ctx context.Context, req *wallet.DebitBalanceReq) (res *wallet.DebitBalanceResp, err error) {
	if err := s.guard(ctx, ActionDebitBalance); err != nil {
		return nil, err
	}

	userID, err := parseDecimalID("user_id", req.GetUserId())
	if err != nil {
		return nil, err
	}
	currency, err := normalizeWalletCurrency(req.GetCurrency())
	if err != nil {
		return nil, err
	}
	if req.GetAmount() <= 0 {
		return nil, invalidArgument("amount must be positive")
	}

	transaction, err := s.applyBalanceChange(ctx, balanceCommand{
		UserID:          userID,
		Currency:        currency,
		TransactionType: txTypeBalanceDebit,
		Amount:          req.GetAmount(),
		OwnerNamespace:  s.Cfg.Wallet.OwnerNamespace,
		BusinessType:    req.GetBusinessType(),
		BusinessID:      req.GetBusinessId(),
		Description:     req.GetDescription(),
		Actor:           actorUser,
	})
	if err != nil {
		return nil, err
	}
	return &wallet.DebitBalanceResp{Transaction: transactionViewOf(transaction)}, nil
}

// RefundBalance 实现同 owner 的余额补偿：只接受原 BALANCE_DEBIT 交易 ID，
// 金额、币种与 namespace 全部从原交易派生，不接受客户端自带金额。
func (s CommerceWalletService) RefundBalance(ctx context.Context, req *wallet.RefundBalanceReq) (res *wallet.RefundBalanceResp, err error) {
	if err := s.guard(ctx, ActionRefundBalance); err != nil {
		return nil, err
	}

	relatedID, err := parseDecimalID("related_transaction_id", req.GetRelatedTransactionId())
	if err != nil {
		return nil, err
	}

	related, err := s.DB.Transaction.Get(ctx, relatedID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("related transaction was not found")
	}
	if err != nil {
		return nil, wrapInternal("refund balance", err)
	}
	if related.Type != txTypeBalanceDebit {
		return nil, invalidArgument("related transaction is not a balance debit")
	}
	// 首期不支持跨 namespace 补偿；对其它 namespace 的交易一律按不存在处理，避免探测。
	namespace := s.Cfg.Wallet.OwnerNamespace
	if related.OwnerNamespace != namespace {
		return nil, walletNotFound("related transaction was not found")
	}

	account, err := s.DB.Account.Get(ctx, related.WalletID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("related transaction was not found")
	}
	if err != nil {
		return nil, wrapInternal("refund balance", err)
	}

	transaction, err := s.applyBalanceChange(ctx, balanceCommand{
		UserID:               account.UserID,
		Currency:             related.Currency,
		TransactionType:      txTypeBalanceRefund,
		Amount:               absAmount(related.DeltaAmount),
		OwnerNamespace:       namespace,
		BusinessType:         businessTypeBalanceRefund,
		BusinessID:           formatDecimalID(related.ID),
		RelatedTransactionID: &related.ID,
		Description:          "balance refund",
		Actor:                actorUser,
	})
	if err != nil {
		return nil, err
	}
	return &wallet.RefundBalanceResp{Transaction: transactionViewOf(transaction)}, nil
}

func (s CommerceWalletService) ProcessPaymentWebhook(ctx context.Context, req *wallet.ProcessPaymentWebhookReq) (res *wallet.ProcessPaymentWebhookResp, err error) {
	if err := s.guard(ctx, ActionProcessPaymentWebhook); err != nil {
		return nil, err
	}
	return s.processPaymentWebhook(ctx, req)
}

// ============================ 管理员类（11） ============================

func (s CommerceWalletService) AdminGetWallet(ctx context.Context, req *wallet.AdminGetWalletReq) (res *wallet.AdminGetWalletResp, err error) {
	if err := s.guard(ctx, ActionAdminGetWallet); err != nil {
		return nil, err
	}
	return s.adminGetWallet(ctx, req)
}

func (s CommerceWalletService) AdminListWallets(ctx context.Context, req *wallet.AdminListWalletsReq) (res *wallet.AdminListWalletsResp, err error) {
	if err := s.guard(ctx, ActionAdminListWallets); err != nil {
		return nil, err
	}
	return s.adminListWallets(ctx, req)
}

func (s CommerceWalletService) AdminListTransactions(ctx context.Context, req *wallet.AdminListTransactionsReq) (res *wallet.AdminListTransactionsResp, err error) {
	if err := s.guard(ctx, ActionAdminListTransactions); err != nil {
		return nil, err
	}
	return s.adminListTransactions(ctx, req)
}

func (s CommerceWalletService) AdminGetRecharge(ctx context.Context, req *wallet.AdminGetRechargeReq) (res *wallet.AdminGetRechargeResp, err error) {
	if err := s.guard(ctx, ActionAdminGetRecharge); err != nil {
		return nil, err
	}
	return s.adminGetRecharge(ctx, req)
}

func (s CommerceWalletService) AdminListRecharges(ctx context.Context, req *wallet.AdminListRechargesReq) (res *wallet.AdminListRechargesResp, err error) {
	if err := s.guard(ctx, ActionAdminListRecharges); err != nil {
		return nil, err
	}
	return s.adminListRecharges(ctx, req)
}

func (s CommerceWalletService) AdminRefreshRecharge(ctx context.Context, req *wallet.AdminRefreshRechargeReq) (res *wallet.AdminRefreshRechargeResp, err error) {
	if err := s.guard(ctx, ActionAdminRefreshRecharge); err != nil {
		return nil, err
	}
	return s.adminRefreshRecharge(ctx, req)
}

// AdminRetryRechargeCheckout 与用户侧重试共用一套状态规则，操作人写入审计字段。
func (s CommerceWalletService) AdminRetryRechargeCheckout(ctx context.Context, req *wallet.AdminRetryRechargeCheckoutReq) (res *wallet.AdminRetryRechargeCheckoutResp, err error) {
	if err := s.guard(ctx, ActionAdminRetryRechargeCheckout); err != nil {
		return nil, err
	}
	return s.adminRetryRechargeCheckout(ctx, req)
}

func (s CommerceWalletService) AdminRefundRecharge(ctx context.Context, req *wallet.AdminRefundRechargeReq) (res *wallet.AdminRefundRechargeResp, err error) {
	if err := s.guard(ctx, ActionAdminRefundRecharge); err != nil {
		return nil, err
	}
	return s.adminRefundRecharge(ctx, req)
}

func (s CommerceWalletService) AdminCreateRedemptionCode(ctx context.Context, req *wallet.AdminCreateRedemptionCodeReq) (res *wallet.AdminCreateRedemptionCodeResp, err error) {
	if err := s.guard(ctx, ActionAdminCreateRedemptionCode); err != nil {
		return nil, err
	}
	return s.adminCreateRedemptionCode(ctx, req)
}

func (s CommerceWalletService) AdminListRedemptionCodes(ctx context.Context, req *wallet.AdminListRedemptionCodesReq) (res *wallet.AdminListRedemptionCodesResp, err error) {
	if err := s.guard(ctx, ActionAdminListRedemptionCodes); err != nil {
		return nil, err
	}
	return s.adminListRedemptionCodes(ctx, req)
}

func (s CommerceWalletService) AdminSetRedemptionCodeStatus(ctx context.Context, req *wallet.AdminSetRedemptionCodeStatusReq) (res *wallet.AdminSetRedemptionCodeStatusResp, err error) {
	action := ActionAdminSetCodeStatusActive
	if req.GetStatus() == wallet.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_DISABLED {
		action = ActionAdminSetCodeStatusDisable
	}
	if err := s.guard(ctx, action); err != nil {
		return nil, err
	}
	return s.adminSetRedemptionCodeStatus(ctx, req)
}
