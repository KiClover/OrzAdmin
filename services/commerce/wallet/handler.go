package wallet

import (
	"context"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/cloudwego/kitex/pkg/kerrors"
	"go.uber.org/fx"
)

// ServiceName 是 Commerce Wallet 服务在服务发现中的名字。
const ServiceName = "commerce-wallet"

// errMsgNotImplemented 是契约占位方法的统一安全文案。
// 真实实现由后续任务补齐，占位方法不得产生任何副作用。
const errMsgNotImplemented = "wallet service method is not implemented yet"

// CommerceWalletService 承载 Commerce Wallet 的业务实现。
// 本任务只冻结契约：所有方法返回 NOT_IMPLEMENTED 业务错误，不读写数据库、不访问 Provider。
type CommerceWalletService struct {
	fx.In

	Cfg *conf.Config
}

var _ wallet.CommerceWalletService = (*CommerceWalletService)(nil)

func NewCommerceWalletService(svc CommerceWalletService) wallet.CommerceWalletService {
	return &svc
}

// notImplemented 返回稳定的“未实现”业务错误，错误码与 WalletErrorCode 对齐。
func notImplemented() error {
	return kerrors.NewBizStatusError(int32(wallet.WalletErrorCode_WALLET_ERROR_CODE_NOT_IMPLEMENTED), errMsgNotImplemented)
}

// ============================ 用户类（7） ============================

func (s CommerceWalletService) GetWallet(ctx context.Context, req *wallet.GetWalletReq) (res *wallet.GetWalletResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) ListTransactions(ctx context.Context, req *wallet.ListTransactionsReq) (res *wallet.ListTransactionsResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) CreateRecharge(ctx context.Context, req *wallet.CreateRechargeReq) (res *wallet.CreateRechargeResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) GetRecharge(ctx context.Context, req *wallet.GetRechargeReq) (res *wallet.GetRechargeResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) ListRecharges(ctx context.Context, req *wallet.ListRechargesReq) (res *wallet.ListRechargesResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) CancelRecharge(ctx context.Context, req *wallet.CancelRechargeReq) (res *wallet.CancelRechargeResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) RedeemCode(ctx context.Context, req *wallet.RedeemCodeReq) (res *wallet.RedeemCodeResp, err error) {
	return nil, notImplemented()
}

// ============================ 可信内部类（3） ============================

func (s CommerceWalletService) DebitBalance(ctx context.Context, req *wallet.DebitBalanceReq) (res *wallet.DebitBalanceResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) RefundBalance(ctx context.Context, req *wallet.RefundBalanceReq) (res *wallet.RefundBalanceResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) ProcessPaymentWebhook(ctx context.Context, req *wallet.ProcessPaymentWebhookReq) (res *wallet.ProcessPaymentWebhookResp, err error) {
	return nil, notImplemented()
}

// ============================ 管理员类（10） ============================

func (s CommerceWalletService) AdminGetWallet(ctx context.Context, req *wallet.AdminGetWalletReq) (res *wallet.AdminGetWalletResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) AdminListWallets(ctx context.Context, req *wallet.AdminListWalletsReq) (res *wallet.AdminListWalletsResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) AdminListTransactions(ctx context.Context, req *wallet.AdminListTransactionsReq) (res *wallet.AdminListTransactionsResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) AdminGetRecharge(ctx context.Context, req *wallet.AdminGetRechargeReq) (res *wallet.AdminGetRechargeResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) AdminListRecharges(ctx context.Context, req *wallet.AdminListRechargesReq) (res *wallet.AdminListRechargesResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) AdminRefreshRecharge(ctx context.Context, req *wallet.AdminRefreshRechargeReq) (res *wallet.AdminRefreshRechargeResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) AdminRefundRecharge(ctx context.Context, req *wallet.AdminRefundRechargeReq) (res *wallet.AdminRefundRechargeResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) AdminCreateRedemptionCode(ctx context.Context, req *wallet.AdminCreateRedemptionCodeReq) (res *wallet.AdminCreateRedemptionCodeResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) AdminListRedemptionCodes(ctx context.Context, req *wallet.AdminListRedemptionCodesReq) (res *wallet.AdminListRedemptionCodesResp, err error) {
	return nil, notImplemented()
}

func (s CommerceWalletService) AdminSetRedemptionCodeStatus(ctx context.Context, req *wallet.AdminSetRedemptionCodeStatusReq) (res *wallet.AdminSetRedemptionCodeStatusResp, err error) {
	return nil, notImplemented()
}
