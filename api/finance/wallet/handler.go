package wallet

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	walletapi "github.com/KiClover/orzAdmin/api/model/finance/wallet"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/internal/log"
	"github.com/KiClover/orzAdmin/internal/middleware"
	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	walletpb "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	walletsvc "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet/commercewalletservice"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/kitex/pkg/kerrors"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

// ServiceName 是 Finance 用户钱包 API 在服务发现中的名称。
const ServiceName = "api.finance.wallet"

// 日志模块标识。
var logger = log.WithModule(log.ModuleWallet)

// 稳定的错误码与文案：上游信息不直接回传，避免泄漏内部标识或供应商细节。
const (
	financeCodeInvalidArgument  int32 = 1
	financeCodePermissionDenied int32 = 3
	financeCodeFeatureDisabled  int32 = 9
	financeCodeRateLimited      int32 = 10
	financeCodeUnavailable      int32 = 12
	financeCodeInternal         int32 = 13
)

// HzFinanceWalletServiceP 是 handler 的依赖。
type HzFinanceWalletServiceP struct {
	fx.In

	Cfg          *conf.Config
	WalletClient walletsvc.Client
	Redis        redis.UniversalClient
	Casbin       *casbin.Enforcer
}

// HzFinanceWalletService 承载用户与管理员钱包 HTTP 路由。
type HzFinanceWalletService struct {
	Cfg          *conf.Config
	WalletClient walletsvc.Client
	RateMw       *middleware.RateMw

	// Enforcer 供管理员路由的 Casbin 中间件使用。
	Enforcer *casbin.Enforcer
	// Codes 是兑换码明文的独立权限检查，缺失时按无权限处理。
	Codes plaintextCodeGuard
	// Audit 记录明文码读取审计，缺失时使用日志实现。
	Audit auditSink
}

// NewHzFinanceWalletService 构造钱包 HTTP handler。
func NewHzFinanceWalletService(p HzFinanceWalletServiceP) (*HzFinanceWalletService, error) {
	rateMw, err := middleware.NewRateMw(p.Cfg, p.Redis)
	if err != nil {
		return nil, err
	}
	return &HzFinanceWalletService{
		Cfg:          p.Cfg,
		WalletClient: p.WalletClient,
		RateMw:       rateMw,
		Enforcer:     p.Casbin,
		Codes:        newCasbinCodeGuard(p.Casbin),
		Audit:        newLogAuditSink(),
	}, nil
}

// GetWallet 返回当前登录用户的余额视图。
//
// GET /wallet
func (s *HzFinanceWalletService) GetWallet(ctx context.Context, c *app.RequestContext) {
	userID, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}

	r, err := s.WalletClient.GetWallet(ctx, &walletpb.GetWalletReq{UserId: userID})
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "get wallet success", &walletapi.GetWalletResp{Wallet: walletViewToAPI(r.GetWallet())})
}

// ListTransactions 返回当前用户的资金流水。
//
// POST /wallet/transactions/list
func (s *HzFinanceWalletService) ListTransactions(ctx context.Context, c *app.RequestContext) {
	userID, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}

	var req walletapi.ListTransactionsReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}

	rpcReq := &walletpb.ListTransactionsReq{
		UserId:    userID,
		PageSize:  req.GetPageSize(),
		Cursor:    req.GetCursor(),
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
	}
	if req.Type != nil {
		parsed, ok := parseTransactionType(req.GetType())
		if !ok {
			writeInvalidArgument(c)
			return
		}
		rpcReq.Type = parsed
	}

	r, err := s.WalletClient.ListTransactions(ctx, rpcReq)
	if err != nil {
		writeWalletError(c, err)
		return
	}

	resp := &walletapi.ListTransactionsResp{
		Transactions: transactionViewsToAPI(r.GetItems()),
		NextCursor:   stringOrNil(r.GetNextCursor()),
		HasMore:      boolOrNil(r.GetHasMore()),
	}
	writeSuccess(c, "list transactions success", resp)
}

// CreateRecharge 创建充值订单。
//
// POST /wallet/recharges
func (s *HzFinanceWalletService) CreateRecharge(ctx context.Context, c *app.RequestContext) {
	userID, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}

	var req walletapi.CreateRechargeReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}

	provider, ok := parsePaymentProvider(req.GetProvider())
	if !ok {
		writeInvalidArgument(c)
		return
	}

	r, err := s.WalletClient.CreateRecharge(ctx, &walletpb.CreateRechargeReq{
		UserId:       userID,
		RequestId:    req.GetRequestID(),
		WalletAmount: req.GetWalletAmount(),
		Provider:     provider,
	})
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "create recharge success", &walletapi.CreateRechargeResp{Recharge: rechargeViewToAPI(r.GetRecharge())})
}

// ListRecharges 返回当前用户的充值订单。
//
// POST /wallet/recharges/list
func (s *HzFinanceWalletService) ListRecharges(ctx context.Context, c *app.RequestContext) {
	userID, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}

	var req walletapi.ListRechargesReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}

	rpcReq := &walletpb.ListRechargesReq{
		UserId:   userID,
		PageSize: req.GetPageSize(),
		Cursor:   req.GetCursor(),
	}
	if req.Status != nil {
		parsed, ok := parseRechargeStatus(req.GetStatus())
		if !ok {
			writeInvalidArgument(c)
			return
		}
		rpcReq.Status = parsed
	}

	r, err := s.WalletClient.ListRecharges(ctx, rpcReq)
	if err != nil {
		writeWalletError(c, err)
		return
	}

	resp := &walletapi.ListRechargesResp{
		Recharges:  rechargeViewsToAPI(r.GetItems()),
		NextCursor: stringOrNil(r.GetNextCursor()),
		HasMore:    boolOrNil(r.GetHasMore()),
	}
	writeSuccess(c, "list recharges success", resp)
}

// GetRecharge 返回单笔充值订单。
//
// GET /wallet/recharges/:rechargeId
func (s *HzFinanceWalletService) GetRecharge(ctx context.Context, c *app.RequestContext) {
	userID, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}
	rechargeID, ok := pathDecimalID(c, "rechargeId")
	if !ok {
		writeInvalidArgument(c)
		return
	}

	r, err := s.WalletClient.GetRecharge(ctx, &walletpb.GetRechargeReq{UserId: userID, RechargeId: rechargeID})
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "get recharge success", &walletapi.GetRechargeResp{Recharge: rechargeViewToAPI(r.GetRecharge())})
}

// CancelRecharge 取消未支付的充值订单。
//
// POST /wallet/recharges/:rechargeId/cancel
func (s *HzFinanceWalletService) CancelRecharge(ctx context.Context, c *app.RequestContext) {
	userID, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}
	rechargeID, ok := pathDecimalID(c, "rechargeId")
	if !ok {
		writeInvalidArgument(c)
		return
	}

	r, err := s.WalletClient.CancelRecharge(ctx, &walletpb.CancelRechargeReq{UserId: userID, RechargeId: rechargeID})
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "cancel recharge success", &walletapi.CancelRechargeResp{Recharge: rechargeViewToAPI(r.GetRecharge())})
}

// RedeemCode 兑换充值码，入口先做共享限流。
//
// POST /wallet/redemption-codes/redeem
func (s *HzFinanceWalletService) RedeemCode(ctx context.Context, c *app.RequestContext) {
	userID, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}

	var req walletapi.RedeemCodeReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}
	if strings.TrimSpace(req.GetCode()) == "" {
		writeInvalidArgument(c)
		return
	}

	r, err := s.WalletClient.RedeemCode(ctx, &walletpb.RedeemCodeReq{UserId: userID, Code: req.GetCode()})
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "redeem code success", &walletapi.RedeemCodeResp{
		Wallet:         walletViewToAPI(r.GetWallet()),
		CreditedAmount: int64OrNil(r.GetCreditedAmount()),
	})
}

// PaymentCallback 把支付渠道回调原样转发给 Commerce。
//
// POST /wallet/payment-callbacks/:provider
//
// 该路由不使用用户 JWT/Casbin，但要求 provider 白名单、请求体上限，
// 并在 Commerce 内完成签名验证；原始字节不重新编码。
func (s *HzFinanceWalletService) PaymentCallback(ctx context.Context, c *app.RequestContext) {
	provider, ok := parsePaymentProvider(c.Param("provider"))
	if !ok {
		writeError(c, http.StatusNotFound, financeCodeInvalidArgument, "unknown payment provider")
		return
	}

	body := c.Request.Body()
	if int64(len(body)) > walletpolicy.WebhookMaxBodyBytes {
		writeError(c, http.StatusRequestEntityTooLarge, financeCodeInvalidArgument, "payment callback body is too large")
		return
	}
	if len(body) == 0 {
		writeError(c, http.StatusBadRequest, financeCodeInvalidArgument, "payment callback body is required")
		return
	}

	headers := webhookHeaders(c)

	callbackCtx, cancel := context.WithTimeout(ctx, walletpolicy.CallbackTimeout)
	defer cancel()

	r, err := s.WalletClient.ProcessPaymentWebhook(callbackCtx, &walletpb.ProcessPaymentWebhookReq{
		Provider:   provider,
		Headers:    headers,
		Body:       body,
		ReceivedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		writeWalletError(c, err)
		return
	}

	writeSuccess(c, "payment callback received", &walletapi.PaymentCallbackResp{
		Received: boolOrNil(r.GetAccepted()),
		Message:  stringOrNil(r.GetMessage()),
	})
}

// AdminListWallets 返回带归属信息的钱包分页列表。
//
// POST /admin/wallets/list
func (s *HzFinanceWalletService) AdminListWallets(ctx context.Context, c *app.RequestContext) {
	var req walletapi.AdminListWalletsReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}
	if !validPageSize(req.GetPageSize()) {
		writeInvalidArgument(c)
		return
	}

	rpcReq := &walletpb.AdminListWalletsReq{
		PageSize: req.GetPageSize(),
		Cursor:   req.GetCursor(),
	}
	userID, ok := optionalDecimalID(req.GetUserID())
	if !ok {
		writeInvalidArgument(c)
		return
	}
	rpcReq.UserId = userID
	if req.Status != nil {
		status, ok := parseWalletStatus(req.GetStatus())
		if !ok {
			writeInvalidArgument(c)
			return
		}
		rpcReq.Status = status
	}

	r, err := s.WalletClient.AdminListWallets(ctx, rpcReq)
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "list wallets success", &walletapi.AdminListWalletsResp{
		Wallets:    adminWalletViewsToAPI(r.GetItems()),
		NextCursor: stringOrNil(r.GetNextCursor()),
		HasMore:    boolOrNil(r.GetHasMore()),
	})
}

// AdminGetWallet 返回单个钱包；body 用 wallet_id 或 user_id 定位，至少提供一个。
//
// POST /admin/wallets/get
func (s *HzFinanceWalletService) AdminGetWallet(ctx context.Context, c *app.RequestContext) {
	var req walletapi.AdminGetWalletReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}
	if req.GetWalletID() == "" && req.GetUserID() == "" {
		writeInvalidArgument(c)
		return
	}

	rpcReq := &walletpb.AdminGetWalletReq{}
	walletID, ok := optionalDecimalID(req.GetWalletID())
	if !ok {
		writeInvalidArgument(c)
		return
	}
	rpcReq.WalletId = walletID
	userID, ok := optionalDecimalID(req.GetUserID())
	if !ok {
		writeInvalidArgument(c)
		return
	}
	rpcReq.UserId = userID

	r, err := s.WalletClient.AdminGetWallet(ctx, rpcReq)
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "get wallet success", &walletapi.AdminGetWalletResp{Wallet: adminWalletViewToAPI(r.GetWallet())})
}

// AdminListTransactions 返回指定钱包的流水，附带归属与业务键。
//
// POST /admin/transactions/list
func (s *HzFinanceWalletService) AdminListTransactions(ctx context.Context, c *app.RequestContext) {
	var req walletapi.AdminListTransactionsReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}
	if !validPageSize(req.GetPageSize()) {
		writeInvalidArgument(c)
		return
	}
	walletID, ok := optionalDecimalID(req.GetWalletID())
	if !ok {
		writeInvalidArgument(c)
		return
	}

	rpcReq := &walletpb.AdminListTransactionsReq{
		WalletId:  walletID,
		PageSize:  req.GetPageSize(),
		Cursor:    req.GetCursor(),
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
	}
	userID, ok := optionalDecimalID(req.GetUserID())
	if !ok {
		writeInvalidArgument(c)
		return
	}
	rpcReq.UserId = userID
	if req.Type != nil {
		parsed, ok := parseTransactionType(req.GetType())
		if !ok {
			writeInvalidArgument(c)
			return
		}
		rpcReq.Type = parsed
	}

	r, err := s.WalletClient.AdminListTransactions(ctx, rpcReq)
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "list transactions success", &walletapi.AdminListTransactionsResp{
		Transactions: adminTransactionViewsToAPI(r.GetItems()),
		NextCursor:   stringOrNil(r.GetNextCursor()),
		HasMore:      boolOrNil(r.GetHasMore()),
	})
}

// AdminListRecharges 返回全部用户的充值订单。
//
// POST /admin/recharges/list
func (s *HzFinanceWalletService) AdminListRecharges(ctx context.Context, c *app.RequestContext) {
	var req walletapi.AdminListRechargesReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}
	if !validPageSize(req.GetPageSize()) {
		writeInvalidArgument(c)
		return
	}

	rpcReq := &walletpb.AdminListRechargesReq{
		PageSize: req.GetPageSize(),
		Cursor:   req.GetCursor(),
	}
	userID, ok := optionalDecimalID(req.GetUserID())
	if !ok {
		writeInvalidArgument(c)
		return
	}
	rpcReq.UserId = userID
	if req.Status != nil {
		parsed, ok := parseRechargeStatus(req.GetStatus())
		if !ok {
			writeInvalidArgument(c)
			return
		}
		rpcReq.Status = parsed
	}
	if req.Provider != nil {
		parsed, ok := parsePaymentProvider(req.GetProvider())
		if !ok {
			writeInvalidArgument(c)
			return
		}
		rpcReq.Provider = &parsed
	}

	r, err := s.WalletClient.AdminListRecharges(ctx, rpcReq)
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "list recharges success", &walletapi.AdminListRechargesResp{
		Recharges:  adminRechargeViewsToAPI(r.GetItems()),
		NextCursor: stringOrNil(r.GetNextCursor()),
		HasMore:    boolOrNil(r.GetHasMore()),
	})
}

// AdminGetRecharge 返回单笔充值订单。
//
// GET /admin/recharges/:rechargeId
func (s *HzFinanceWalletService) AdminGetRecharge(ctx context.Context, c *app.RequestContext) {
	rechargeID, ok := pathDecimalID(c, "rechargeId")
	if !ok {
		writeInvalidArgument(c)
		return
	}

	r, err := s.WalletClient.AdminGetRecharge(ctx, &walletpb.AdminGetRechargeReq{RechargeId: rechargeID})
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "get recharge success", &walletapi.AdminGetRechargeResp{Recharge: adminRechargeViewToAPI(r.GetRecharge())})
}

// AdminRefreshRecharge 用供应商真实状态刷新单笔充值。
//
// POST /admin/recharges/:rechargeId/refresh
//
// 操作人只来自认证上下文；请求体中的 operator_id 会被忽略，因为生成的
// 请求结构没有该字段，身份无法被调用方覆盖。
func (s *HzFinanceWalletService) AdminRefreshRecharge(ctx context.Context, c *app.RequestContext) {
	operator, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}
	rechargeID, ok := pathDecimalID(c, "rechargeId")
	if !ok {
		writeInvalidArgument(c)
		return
	}

	var req walletapi.AdminRefreshRechargeReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}
	reason := strings.TrimSpace(req.GetReason())
	if reason == "" {
		writeInvalidArgument(c)
		return
	}

	rpcReq := &walletpb.AdminRefreshRechargeReq{
		OperatorId:                operator,
		RechargeId:                rechargeID,
		Reason:                    reason,
		ConfirmProviderNotCreated: req.GetConfirmProviderNotCreated(),
	}
	if value := strings.TrimSpace(req.GetProviderCheckoutID()); value != "" {
		rpcReq.ProviderCheckoutId = &value
	}
	if value := strings.TrimSpace(req.GetProviderOrderID()); value != "" {
		rpcReq.ProviderOrderId = &value
	}
	if value := strings.TrimSpace(req.GetProviderTransactionID()); value != "" {
		rpcReq.ProviderTransactionId = &value
	}

	r, err := s.WalletClient.AdminRefreshRecharge(ctx, rpcReq)
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "refresh recharge success", &walletapi.AdminRefreshRechargeResp{Recharge: adminRechargeViewToAPI(r.GetRecharge())})
}

// AdminRefundRecharge 发起全额退款。
//
// POST /admin/recharges/:rechargeId/refund
func (s *HzFinanceWalletService) AdminRefundRecharge(ctx context.Context, c *app.RequestContext) {
	operator, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}
	rechargeID, ok := pathDecimalID(c, "rechargeId")
	if !ok {
		writeInvalidArgument(c)
		return
	}

	var req walletapi.AdminRefundRechargeReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}
	reason := strings.TrimSpace(req.GetReason())
	if reason == "" {
		writeInvalidArgument(c)
		return
	}

	r, err := s.WalletClient.AdminRefundRecharge(ctx, &walletpb.AdminRefundRechargeReq{
		OperatorId: operator,
		RechargeId: rechargeID,
		Reason:     reason,
	})
	if err != nil {
		writeWalletError(c, err)
		return
	}
	writeSuccess(c, "refund recharge success", &walletapi.AdminRefundRechargeResp{Recharge: adminRechargeViewToAPI(r.GetRecharge())})
}

// AdminCreateRedemptionCode 生成兑换码并首次返回明文码。
//
// POST /admin/redemption-codes
//
// 明文读取是独立于路由权限的检查：没有 wallet-code-reader 策略时
// 直接拒绝，不调用 Commerce，也不产生兑换码。
func (s *HzFinanceWalletService) AdminCreateRedemptionCode(ctx context.Context, c *app.RequestContext) {
	operator, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}
	if !s.authorizePlaintextCode(c, operator, walletCodeActionCreate) {
		return
	}

	var req walletapi.AdminCreateRedemptionCodeReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}
	requestID := strings.TrimSpace(req.GetRequestID())
	if requestID == "" || req.Type == nil || req.GetAmount() <= 0 {
		writeInvalidArgument(c)
		return
	}
	codeType, ok := parseRedemptionCodeType(req.GetType())
	if !ok {
		writeInvalidArgument(c)
		return
	}
	// 非空 code 表示调用方指定明文；只校验格式，不改变内容。
	if req.GetCode() != "" && !validCustomRedemptionCode(req.GetCode()) {
		writeInvalidArgument(c)
		return
	}

	r, err := s.WalletClient.AdminCreateRedemptionCode(ctx, &walletpb.AdminCreateRedemptionCodeReq{
		OperatorId: operator,
		RequestId:  requestID,
		Type:       *codeType,
		Amount:     req.GetAmount(),
		ExpiresAt:  req.ExpiresAt,
		Code:       req.Code,
	})
	if err != nil {
		writeWalletError(c, err)
		return
	}

	writeNoStore(c)
	s.recordCodeAudit(codeAuditEvent{
		Operator:  operator,
		Action:    walletCodeActionCreate,
		RequestID: requestID,
		Count:     1,
	})
	writeSuccess(c, "create redemption code success", &walletapi.AdminCreateRedemptionCodeResp{
		Code: adminRedemptionCodeViewToAPI(r.GetCode()),
	})
}

// AdminListRedemptionCodes 分页返回兑换码，响应含明文码。
//
// POST /admin/redemption-codes/list
func (s *HzFinanceWalletService) AdminListRedemptionCodes(ctx context.Context, c *app.RequestContext) {
	operator, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}
	if !s.authorizePlaintextCode(c, operator, walletCodeActionList) {
		return
	}

	var req walletapi.AdminListRedemptionCodesReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}
	if !validPageSize(req.GetPageSize()) {
		writeInvalidArgument(c)
		return
	}

	rpcReq := &walletpb.AdminListRedemptionCodesReq{
		PageSize: req.GetPageSize(),
		Cursor:   req.GetCursor(),
	}
	if req.Type != nil {
		parsed, ok := parseRedemptionCodeType(req.GetType())
		if !ok {
			writeInvalidArgument(c)
			return
		}
		rpcReq.Type = parsed
	}
	if req.Status != nil {
		parsed, ok := parseRedemptionCodeStatus(req.GetStatus())
		if !ok {
			writeInvalidArgument(c)
			return
		}
		rpcReq.Status = parsed
	}

	r, err := s.WalletClient.AdminListRedemptionCodes(ctx, rpcReq)
	if err != nil {
		writeWalletError(c, err)
		return
	}

	writeNoStore(c)
	s.recordCodeAudit(codeAuditEvent{
		Operator: operator,
		Action:   walletCodeActionList,
		Filters: map[string]string{
			"type":   req.GetType(),
			"status": req.GetStatus(),
		},
		Count: len(r.GetItems()),
	})
	writeSuccess(c, "list redemption codes success", &walletapi.AdminListRedemptionCodesResp{
		Codes:      adminRedemptionCodeViewsToAPI(r.GetItems()),
		NextCursor: stringOrNil(r.GetNextCursor()),
		HasMore:    boolOrNil(r.GetHasMore()),
	})
}

// AdminSetRedemptionCodeStatus 启用或停用兑换码，响应含明文码。
//
// POST /admin/redemption-codes/:codeId/status
func (s *HzFinanceWalletService) AdminSetRedemptionCodeStatus(ctx context.Context, c *app.RequestContext) {
	operator, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}
	codeID, ok := pathDecimalID(c, "codeId")
	if !ok {
		writeInvalidArgument(c)
		return
	}
	if !s.authorizePlaintextCode(c, operator, walletCodeActionStatus) {
		return
	}

	var req walletapi.AdminSetRedemptionCodeStatusReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}
	if req.Status == nil {
		writeInvalidArgument(c)
		return
	}
	status, ok := parseRedemptionCodeStatus(req.GetStatus())
	if !ok {
		writeInvalidArgument(c)
		return
	}
	if *status != walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE &&
		*status != walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_DISABLED {
		writeInvalidArgument(c)
		return
	}

	rpcReq := &walletpb.AdminSetRedemptionCodeStatusReq{
		OperatorId: operator,
		CodeId:     codeID,
		Status:     *status,
	}
	if reason := strings.TrimSpace(req.GetReason()); reason != "" {
		rpcReq.Reason = &reason
	}

	r, err := s.WalletClient.AdminSetRedemptionCodeStatus(ctx, rpcReq)
	if err != nil {
		writeWalletError(c, err)
		return
	}

	writeNoStore(c)
	s.recordCodeAudit(codeAuditEvent{
		Operator: operator,
		Action:   walletCodeActionStatus,
		Filters: map[string]string{
			"code_id": codeID,
			"status":  req.GetStatus(),
		},
		Count: 1,
	})
	writeSuccess(c, "set redemption code status success", &walletapi.AdminSetRedemptionCodeStatusResp{
		Code: adminRedemptionCodeViewToAPI(r.GetCode()),
	})
}

// AdminAdjustBalance 管理员调账：正数加款、负数扣款。
//
// POST /admin/wallets/adjustments
//
// 操作人只来自 JWT；幂等键是 request_id，重放由 Commerce 侧判定。
// 调账不受普通扣款规则（余额充足、review_blocks_debit）约束，但仍要求
// 钱包功能已启用且运行设置快照有效；功能关闭在本接口固定返回 403。
func (s *HzFinanceWalletService) AdminAdjustBalance(ctx context.Context, c *app.RequestContext) {
	operator, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}

	var req walletapi.AdminAdjustBalanceReq
	if err := c.BindAndValidate(&req); err != nil {
		writeInvalidArgument(c)
		return
	}

	userID := strings.TrimSpace(req.GetUserID())
	requestID := strings.TrimSpace(req.GetRequestID())
	reason := strings.TrimSpace(req.GetReason())
	if !validDecimalID(userID) || len(requestID) == 0 || len(requestID) > 128 ||
		len(reason) == 0 || len(reason) > 512 ||
		req.DeltaAmount == nil || *req.DeltaAmount == 0 || *req.DeltaAmount == math.MinInt64 {
		writeInvalidArgument(c)
		return
	}

	r, err := s.WalletClient.AdminAdjustBalance(ctx, &walletpb.AdminAdjustBalanceReq{
		OperatorId:  operator,
		UserId:      userID,
		RequestId:   requestID,
		DeltaAmount: *req.DeltaAmount,
		Reason:      reason,
	})
	if err != nil {
		if isFeatureDisabled(err) {
			writeError(c, http.StatusForbidden, financeCodeFeatureDisabled, "wallet feature is disabled")
			return
		}
		writeWalletError(c, err)
		return
	}

	writeSuccess(c, "adjust wallet balance success", &walletapi.AdminAdjustBalanceResp{
		Transaction: adminTransactionViewToAPI(r.GetTransaction()),
	})
}

// isFeatureDisabled 判断错误是否为钱包功能关闭。
func isFeatureDisabled(err error) bool {
	var bizErr *kerrors.BizStatusError
	if !errors.As(err, &bizErr) {
		return false
	}
	return bizErr.BizStatusCode() == int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED)
}

// authorizePlaintextCode 执行独立的明文读取检查并记录拒绝审计。
func (s *HzFinanceWalletService) authorizePlaintextCode(c *app.RequestContext, operator, action string) bool {
	guard := s.Codes
	if guard == nil {
		s.recordCodeAudit(codeAuditEvent{
			Operator: operator,
			Action:   action,
			Denied:   true,
			Reason:   "plaintext guard is not configured",
		})
		writePermissionDenied(c)
		return false
	}

	allowed, err := guard.Allowed(operator)
	if err != nil {
		logger.Errorf("wallet code authorization failed: %v", err)
		s.recordCodeAudit(codeAuditEvent{
			Operator: operator,
			Action:   action,
			Denied:   true,
			Reason:   "authorization check failed",
		})
		writeError(c, http.StatusServiceUnavailable, financeCodeUnavailable, "wallet service is temporarily unavailable")
		return false
	}
	if !allowed {
		s.recordCodeAudit(codeAuditEvent{
			Operator: operator,
			Action:   action,
			Denied:   true,
			Reason:   "plaintext permission missing",
		})
		writePermissionDenied(c)
		return false
	}
	return true
}

// recordCodeAudit 记录一次兑换码审计；审计实现缺失时回退到日志。
func (s *HzFinanceWalletService) recordCodeAudit(event codeAuditEvent) {
	sink := s.Audit
	if sink == nil {
		sink = newLogAuditSink()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	sink.RecordCodeAccess(event)
}

// currentUserID 读取 JWT 中间件写入的用户 ID，并转成十进制字符串。
func currentUserID(c *app.RequestContext) (string, bool) {
	raw, ok := c.Get("userId")
	if !ok {
		return "", false
	}
	switch v := raw.(type) {
	case uint64:
		if v == 0 {
			return "", false
		}
		return strconv.FormatUint(v, 10), true
	case int64:
		if v <= 0 {
			return "", false
		}
		return strconv.FormatInt(v, 10), true
	case int:
		if v <= 0 {
			return "", false
		}
		return strconv.Itoa(v), true
	case string:
		if _, err := strconv.ParseUint(v, 10, 64); err != nil {
			return "", false
		}
		return v, true
	default:
		return "", false
	}
}

// pathDecimalID 读取并校验路径参数中的十进制 ID。
func pathDecimalID(c *app.RequestContext, name string) (string, bool) {
	raw := c.Param(name)
	if !validDecimalID(raw) {
		return "", false
	}
	return raw, true
}

// validDecimalID 校验十进制 ID：非空、仅数字、无前导零、不溢出 uint64。
func validDecimalID(raw string) bool {
	if raw == "" || len(raw) > 20 || raw[0] == '0' {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return false
		}
	}
	_, err := strconv.ParseUint(raw, 10, 64)
	return err == nil
}

// parsePaymentProvider 把 HTTP 中的 provider 名称映射为枚举。
//
// 对外使用短名称（CREEM），同时兼容完整枚举名，空或未知返回 false。
func parsePaymentProvider(raw string) (walletpb.PaymentProvider, bool) {
	name := strings.ToUpper(strings.TrimSpace(raw))
	if name == "" {
		return walletpb.PaymentProvider_PAYMENT_PROVIDER_UNSPECIFIED, false
	}
	if value, ok := walletpb.PaymentProvider_value[name]; ok && value != 0 {
		return walletpb.PaymentProvider(value), true
	}
	if value, ok := walletpb.PaymentProvider_value["PAYMENT_PROVIDER_"+name]; ok && value != 0 {
		return walletpb.PaymentProvider(value), true
	}
	return walletpb.PaymentProvider_PAYMENT_PROVIDER_UNSPECIFIED, false
}

// parseTransactionType 把查询参数中的流水类型映射为枚举。
func parseTransactionType(raw string) (*walletpb.TransactionType, bool) {
	name := strings.ToUpper(strings.TrimSpace(raw))
	if name == "" {
		return nil, false
	}
	value, ok := walletpb.TransactionType_value[name]
	if !ok {
		value, ok = walletpb.TransactionType_value["TRANSACTION_TYPE_"+name]
	}
	if !ok || value == 0 {
		return nil, false
	}
	parsed := walletpb.TransactionType(value)
	return &parsed, true
}

// parseRechargeStatus 把查询参数中的充值状态映射为枚举。
func parseRechargeStatus(raw string) (*walletpb.RechargeStatus, bool) {
	name := strings.ToUpper(strings.TrimSpace(raw))
	if name == "" {
		return nil, false
	}
	value, ok := walletpb.RechargeStatus_value[name]
	if !ok {
		value, ok = walletpb.RechargeStatus_value["RECHARGE_STATUS_"+name]
	}
	if !ok || value == 0 {
		return nil, false
	}
	parsed := walletpb.RechargeStatus(value)
	return &parsed, true
}

// webhookHeaders 保留同名多值头，供 Commerce 判断重复签名头。
func webhookHeaders(c *app.RequestContext) []*walletpb.WebhookHeader {
	order := make([]string, 0, 8)
	values := make(map[string][]string, 8)
	c.Request.Header.VisitAll(func(key, value []byte) {
		name := strings.ToLower(string(key))
		if _, seen := values[name]; !seen {
			order = append(order, name)
		}
		values[name] = append(values[name], string(value))
	})

	headers := make([]*walletpb.WebhookHeader, 0, len(order))
	for _, name := range order {
		headers = append(headers, &walletpb.WebhookHeader{Name: name, Values: values[name]})
	}
	return headers
}

// writeSuccess 输出统一的成功响应。
func writeSuccess(c *app.RequestContext, message string, body interface{}) {
	c.JSON(http.StatusOK, &struct {
		Code int32       `json:"code"`
		Msg  string      `json:"msg"`
		Data interface{} `json:"data"`
	}{Code: 0, Msg: message, Data: body})
}

// writeError 输出稳定的错误响应。
func writeError(c *app.RequestContext, status int, code int32, message string) {
	c.JSON(status, &struct {
		Code int32  `json:"code"`
		Msg  string `json:"msg"`
	}{Code: code, Msg: message})
}

// writeInvalidArgument 输出参数错误。
func writeInvalidArgument(c *app.RequestContext) {
	writeError(c, http.StatusBadRequest, financeCodeInvalidArgument, "request parameters are invalid")
}

// writeUnauthorized 输出未认证错误（JWT 中间件缺失时不泄漏细节）。
func writeUnauthorized(c *app.RequestContext) {
	writeError(c, http.StatusUnauthorized, financeCodeInvalidArgument, "user identity is required")
}

// walletHTTPStatus 把 Commerce 业务错误码映射为稳定的 HTTP 状态。
//
// 认证/授权/不存在/状态与幂等冲突属于 4xx；余额规则冲突用 422；
// 功能关闭与暂时不可用用 503；未实现与未知错误用 5xx。
var walletHTTPStatus = map[int32]int{
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT):     http.StatusBadRequest,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_UNAUTHENTICATED):      http.StatusUnauthorized,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_PERMISSION_DENIED):    http.StatusForbidden,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND):            http.StatusNotFound,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_STATE_CONFLICT):       http.StatusConflict,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT): http.StatusConflict,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_INSUFFICIENT_BALANCE): http.StatusUnprocessableEntity,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_BALANCE_BLOCKED):      http.StatusUnprocessableEntity,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED):     http.StatusServiceUnavailable,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_RATE_LIMITED):         http.StatusTooManyRequests,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_NOT_IMPLEMENTED):      http.StatusNotImplemented,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_UNAVAILABLE):          http.StatusServiceUnavailable,
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_INTERNAL):             http.StatusInternalServerError,
}

// walletHTTPMessage 是每个业务错误码对应的固定文案，避免回传内部标识。
var walletHTTPMessage = map[int32]string{
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT):     "request parameters are invalid",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_UNAUTHENTICATED):      "user identity is required",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_PERMISSION_DENIED):    "permission denied",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND):            "wallet resource not found",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_STATE_CONFLICT):       "request conflicts with current state",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT): "request conflicts with a previous request",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_INSUFFICIENT_BALANCE): "wallet balance is insufficient",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_BALANCE_BLOCKED):      "wallet is temporarily blocked",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED):     "wallet feature is disabled",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_RATE_LIMITED):         "too many requests",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_NOT_IMPLEMENTED):      "wallet feature is not available",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_UNAVAILABLE):          "wallet service is temporarily unavailable",
	int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_INTERNAL):             "wallet service failed",
}

// writeWalletError 输出稳定的钱包错误响应。
func writeWalletError(c *app.RequestContext, err error) {
	var bizErr *kerrors.BizStatusError
	if errors.As(err, &bizErr) {
		code := bizErr.BizStatusCode()
		status, ok := walletHTTPStatus[code]
		if !ok {
			status = http.StatusInternalServerError
		}
		message, ok := walletHTTPMessage[code]
		if !ok {
			message = "wallet request failed"
		}
		writeError(c, status, code, message)
		return
	}

	logger.Errorf("wallet rpc call failed: %v", err)
	writeError(c, http.StatusServiceUnavailable, financeCodeUnavailable, "wallet service is temporarily unavailable")
}

// walletViewToAPI 转换用户余额视图。
func walletViewToAPI(in *walletpb.WalletView) *walletapi.WalletView {
	if in == nil {
		return nil
	}
	return &walletapi.WalletView{
		Currency:      stringOrNil(in.GetCurrency()),
		BalanceAmount: int64OrNil(in.GetBalanceAmount()),
		Status:        stringOrNil(walletStatusName(in.GetStatus())),
		Revision:      int64OrNil(int64(in.GetRevision())),
		UpdatedAt:     timeOrNil(in.GetUpdatedAt()),
	}
}

// transactionViewsToAPI 批量转换用户流水视图。
func transactionViewsToAPI(in []*walletpb.TransactionView) []*walletapi.TransactionView {
	out := make([]*walletapi.TransactionView, 0, len(in))
	for _, item := range in {
		out = append(out, transactionViewToAPI(item))
	}
	return out
}

// transactionViewToAPI 转换单条用户流水视图。
func transactionViewToAPI(in *walletpb.TransactionView) *walletapi.TransactionView {
	if in == nil {
		return nil
	}
	return &walletapi.TransactionView{
		ID:           stringOrNil(in.GetId()),
		Type:         stringOrNil(transactionTypeName(in.GetType())),
		Currency:     stringOrNil(in.GetCurrency()),
		DeltaAmount:  int64OrNil(in.GetDeltaAmount()),
		BalanceAfter: int64OrNil(in.GetBalanceAfter()),
		Description:  stringOrNil(in.GetDescription()),
		CreatedAt:    timeOrNil(in.GetCreatedAt()),
	}
}

// rechargeViewsToAPI 批量转换用户充值视图。
func rechargeViewsToAPI(in []*walletpb.RechargeView) []*walletapi.RechargeView {
	out := make([]*walletapi.RechargeView, 0, len(in))
	for _, item := range in {
		out = append(out, rechargeViewToAPI(item))
	}
	return out
}

// rechargeViewToAPI 转换单笔用户充值视图。
func rechargeViewToAPI(in *walletpb.RechargeView) *walletapi.RechargeView {
	if in == nil {
		return nil
	}
	return &walletapi.RechargeView{
		ID:               stringOrNil(in.GetId()),
		Status:           stringOrNil(rechargeStatusName(in.GetStatus())),
		WalletAmount:     int64OrNil(in.GetWalletAmount()),
		WalletCurrency:   stringOrNil(in.GetWalletCurrency()),
		ProviderAmount:   int64OrNil(in.GetProviderAmount()),
		ProviderCurrency: stringOrNil(in.GetProviderCurrency()),
		CnyPerUsdRate:    stringOrNil(in.GetCnyPerUsdRate()),
		CheckoutURL:      stringOrNil(in.GetCheckoutUrl()),
		CreatedAt:        timeOrNil(in.GetCreatedAt()),
		PaidAt:           timeOrNil(in.GetPaidAt()),
		UpdatedAt:        timeOrNil(in.GetUpdatedAt()),
	}
}

// walletStatusName 返回派生余额状态的名称。
func walletStatusName(status walletpb.WalletStatus) string {
	switch status {
	case walletpb.WalletStatus_WALLET_STATUS_ACTIVE:
		return "ACTIVE"
	case walletpb.WalletStatus_WALLET_STATUS_DEBT:
		return "DEBT"
	default:
		return ""
	}
}

// transactionTypeName 去掉枚举前缀后返回流水类型名。
func transactionTypeName(t walletpb.TransactionType) string {
	if t == walletpb.TransactionType_TRANSACTION_TYPE_UNSPECIFIED {
		return ""
	}
	return strings.TrimPrefix(walletpb.TransactionType_name[int32(t)], "TRANSACTION_TYPE_")
}

// rechargeStatusName 去掉枚举前缀后返回充值状态名。
func rechargeStatusName(status walletpb.RechargeStatus) string {
	if status == walletpb.RechargeStatus_RECHARGE_STATUS_UNSPECIFIED {
		return ""
	}
	return strings.TrimPrefix(walletpb.RechargeStatus_name[int32(status)], "RECHARGE_STATUS_")
}

// timeOrNil 把 Unix 毫秒转成 RFC3339 毫秒字符串；0 表示未设置。
func timeOrNil(ms int64) *string {
	if ms <= 0 {
		return nil
	}
	formatted := time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z07:00")
	return &formatted
}

// stringOrNil 把空字符串转成 nil，避免响应出现空字段。
func stringOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// int64OrNil 把 0 保留为合法的金额/版本值。
func int64OrNil(value int64) *int64 {
	return &value
}

// boolOrNil 返回布尔指针。
func boolOrNil(value bool) *bool {
	return &value
}

// optionalDecimalID 校验可选的十进制过滤条件；空值表示不过滤。
func optionalDecimalID(raw string) (*string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, true
	}
	if !validDecimalID(value) {
		return nil, false
	}
	return &value, true
}

// validPageSize 限制列表分页上限，避免一次拉取全表。
func validPageSize(size int32) bool {
	return size >= 0 && int(size) <= walletpolicy.MaxListPageSize
}

// parseWalletStatus 把查询参数中的余额状态映射为枚举。
func parseWalletStatus(raw string) (*walletpb.WalletStatus, bool) {
	name := strings.ToUpper(strings.TrimSpace(raw))
	if name == "" {
		return nil, false
	}
	value, ok := walletpb.WalletStatus_value[name]
	if !ok {
		value, ok = walletpb.WalletStatus_value["WALLET_STATUS_"+name]
	}
	if !ok || value == 0 {
		return nil, false
	}
	parsed := walletpb.WalletStatus(value)
	return &parsed, true
}

// parseRedemptionCodeType 把兑换码类型映射为枚举。
func parseRedemptionCodeType(raw string) (*walletpb.RedemptionCodeType, bool) {
	name := strings.ToUpper(strings.TrimSpace(raw))
	if name == "" {
		return nil, false
	}
	value, ok := walletpb.RedemptionCodeType_value[name]
	if !ok {
		value, ok = walletpb.RedemptionCodeType_value["REDEMPTION_CODE_TYPE_"+name]
	}
	if !ok || value == 0 {
		return nil, false
	}
	parsed := walletpb.RedemptionCodeType(value)
	return &parsed, true
}

// redemptionCodeMaxInputLen 是自定义兑换码的最大字节长度，与数据库列长度一致。
const redemptionCodeMaxInputLen = 128

// validCustomRedemptionCode 校验非空自定义兑换码。
//
// 只接受 1~128 字节的可见 ASCII（0x21~0x7E）；不做去空白、大小写折叠
// 或连字符移除，兑换码必须按调用方给出的原值精确匹配。
func validCustomRedemptionCode(raw string) bool {
	if len(raw) == 0 || len(raw) > redemptionCodeMaxInputLen {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x21 || raw[i] > 0x7e {
			return false
		}
	}
	return true
}

// parseRedemptionCodeStatus 把兑换码状态映射为枚举。
func parseRedemptionCodeStatus(raw string) (*walletpb.RedemptionCodeStatus, bool) {
	name := strings.ToUpper(strings.TrimSpace(raw))
	if name == "" {
		return nil, false
	}
	value, ok := walletpb.RedemptionCodeStatus_value[name]
	if !ok {
		value, ok = walletpb.RedemptionCodeStatus_value["REDEMPTION_CODE_STATUS_"+name]
	}
	if !ok || value == 0 {
		return nil, false
	}
	parsed := walletpb.RedemptionCodeStatus(value)
	return &parsed, true
}

// writeNoStore 禁止缓存含明文兑换码的响应。
func writeNoStore(c *app.RequestContext) {
	c.Header("Cache-Control", "no-store")
}

// writePermissionDenied 输出无权限错误。
func writePermissionDenied(c *app.RequestContext) {
	writeError(c, http.StatusForbidden, financeCodePermissionDenied, "permission denied")
}

// adminWalletViewsToAPI 批量转换管理员钱包视图。
func adminWalletViewsToAPI(in []*walletpb.AdminWalletView) []*walletapi.AdminWalletView {
	out := make([]*walletapi.AdminWalletView, 0, len(in))
	for _, item := range in {
		out = append(out, adminWalletViewToAPI(item))
	}
	return out
}

// adminWalletViewToAPI 转换单个管理员钱包视图。
//
// 冻结的 Commerce 视图只携带用户可见部分，不含钱包主键，因此
// wallet_id 保持为空；管理员仍可用 user_id 定位钱包。
func adminWalletViewToAPI(in *walletpb.AdminWalletView) *walletapi.AdminWalletView {
	if in == nil {
		return nil
	}
	out := &walletapi.AdminWalletView{
		UserID:   stringOrNil(in.GetUserId()),
		WalletID: stringOrNil(in.GetWalletId()),
	}
	if wallet := in.GetWallet(); wallet != nil {
		out.Currency = stringOrNil(wallet.GetCurrency())
		out.BalanceAmount = int64OrNil(wallet.GetBalanceAmount())
		out.Status = stringOrNil(walletStatusName(wallet.GetStatus()))
		out.Revision = int64OrNil(wallet.GetRevision())
		out.UpdatedAt = timeOrNil(wallet.GetUpdatedAt())
	}
	return out
}

// adminTransactionViewsToAPI 批量转换管理员流水视图。
func adminTransactionViewsToAPI(in []*walletpb.AdminTransactionView) []*walletapi.AdminTransactionView {
	out := make([]*walletapi.AdminTransactionView, 0, len(in))
	for _, item := range in {
		out = append(out, adminTransactionViewToAPI(item))
	}
	return out
}

// adminTransactionViewToAPI 转换单条管理员流水视图，附带归属与业务键。
func adminTransactionViewToAPI(in *walletpb.AdminTransactionView) *walletapi.AdminTransactionView {
	if in == nil {
		return nil
	}
	out := &walletapi.AdminTransactionView{
		WalletID:             stringOrNil(in.GetWalletId()),
		UserID:               stringOrNil(in.GetUserId()),
		OwnerNamespace:       stringOrNil(in.GetOwnerNamespace()),
		BusinessType:         stringOrNil(in.GetBusinessType()),
		BusinessID:           stringOrNil(in.GetBusinessId()),
		RelatedTransactionID: stringOrNil(in.GetRelatedTransactionId()),
		Actor:                stringOrNil(in.GetActor()),
	}
	if transaction := in.GetTransaction(); transaction != nil {
		out.ID = stringOrNil(transaction.GetId())
		out.Type = stringOrNil(transactionTypeName(transaction.GetType()))
		out.Currency = stringOrNil(transaction.GetCurrency())
		out.DeltaAmount = int64OrNil(transaction.GetDeltaAmount())
		out.BalanceAfter = int64OrNil(transaction.GetBalanceAfter())
		out.Description = stringOrNil(transaction.GetDescription())
		out.CreatedAt = timeOrNil(transaction.GetCreatedAt())
	}
	return out
}

// adminRechargeViewsToAPI 批量转换管理员充值视图。
func adminRechargeViewsToAPI(in []*walletpb.AdminRechargeView) []*walletapi.AdminRechargeView {
	out := make([]*walletapi.AdminRechargeView, 0, len(in))
	for _, item := range in {
		out = append(out, adminRechargeViewToAPI(item))
	}
	return out
}

// adminRechargeViewToAPI 转换单笔管理员充值视图。
func adminRechargeViewToAPI(in *walletpb.AdminRechargeView) *walletapi.AdminRechargeView {
	if in == nil {
		return nil
	}
	out := &walletapi.AdminRechargeView{
		UserID:                 stringOrNil(in.GetUserId()),
		WalletID:               stringOrNil(in.GetWalletId()),
		RequestID:              stringOrNil(in.GetRequestId()),
		MerchantOrderNo:        stringOrNil(in.GetMerchantOrderNo()),
		Provider:               stringOrNil(paymentProviderName(in.GetProvider())),
		ProviderEnvironment:    stringOrNil(providerEnvironmentName(in.GetProviderEnvironment())),
		PaymentProfileID:       stringOrNil(in.GetPaymentProfileId()),
		PaymentProfileRevision: int64OrNil(in.GetPaymentProfileRevision()),
		ProviderProductID:      stringOrNil(in.GetProviderProductId()),
		ProviderCheckoutID:     stringOrNil(in.GetProviderCheckoutId()),
		ProviderOrderID:        stringOrNil(in.GetProviderOrderId()),
		ProviderTransactionID:  stringOrNil(in.GetProviderTransactionId()),
		FxRateNumerator:        int64OrNil(in.GetFxRateNumerator()),
		FxRateDenominator:      int64OrNil(in.GetFxRateDenominator()),
		FxSettingRevision:      int64OrNil(in.GetFxSettingRevision()),
		FxQuotedAt:             timeOrNil(in.GetFxQuotedAt()),
		FxRoundingMode:         stringOrNil(in.GetFxRoundingMode()),
		ReviewReason:           stringOrNil(in.GetReviewReason()),
		ReviewBlocksDebit:      boolOrNil(in.GetReviewBlocksDebit()),
		ReviewedBy:             stringOrNil(in.GetReviewedBy()),
		ReviewedAt:             timeOrNil(in.GetReviewedAt()),
		RefundReason:           stringOrNil(in.GetRefundReason()),
		RefundRequestedBy:      stringOrNil(in.GetRefundRequestedBy()),
		RefundRequestedAt:      timeOrNil(in.GetRefundRequestedAt()),
		Revision:               int64OrNil(in.GetRevision()),
		CanceledAt:             timeOrNil(in.GetCanceledAt()),
		RefundedAt:             timeOrNil(in.GetRefundedAt()),
	}
	if recharge := in.GetRecharge(); recharge != nil {
		out.ID = stringOrNil(recharge.GetId())
		out.Status = stringOrNil(rechargeStatusName(recharge.GetStatus()))
		out.WalletAmount = int64OrNil(recharge.GetWalletAmount())
		out.WalletCurrency = stringOrNil(recharge.GetWalletCurrency())
		out.ProviderAmount = int64OrNil(recharge.GetProviderAmount())
		out.ProviderCurrency = stringOrNil(recharge.GetProviderCurrency())
		out.CheckoutURL = stringOrNil(recharge.GetCheckoutUrl())
		out.PaidAt = timeOrNil(recharge.GetPaidAt())
		out.CreatedAt = timeOrNil(recharge.GetCreatedAt())
		out.UpdatedAt = timeOrNil(recharge.GetUpdatedAt())
	}
	return out
}

// adminRedemptionCodeViewsToAPI 批量转换管理员兑换码视图。
func adminRedemptionCodeViewsToAPI(in []*walletpb.AdminRedemptionCodeView) []*walletapi.AdminRedemptionCodeView {
	out := make([]*walletapi.AdminRedemptionCodeView, 0, len(in))
	for _, item := range in {
		out = append(out, adminRedemptionCodeViewToAPI(item))
	}
	return out
}

// adminRedemptionCodeViewToAPI 转换单个管理员兑换码视图（含明文码）。
func adminRedemptionCodeViewToAPI(in *walletpb.AdminRedemptionCodeView) *walletapi.AdminRedemptionCodeView {
	if in == nil {
		return nil
	}
	return &walletapi.AdminRedemptionCodeView{
		ID:              stringOrNil(in.GetId()),
		Code:            stringOrNil(in.GetCode()),
		Type:            stringOrNil(redemptionCodeTypeName(in.GetType())),
		Amount:          int64OrNil(in.GetAmount()),
		Currency:        stringOrNil(in.GetCurrency()),
		Status:          stringOrNil(redemptionCodeStatusName(in.GetStatus())),
		ExpiresAt:       timeOrNil(in.GetExpiresAt()),
		RedeemedCount:   int64OrNil(in.GetRedeemedCount()),
		ClaimedByUserID: stringOrNil(in.GetClaimedByUserId()),
		ClaimedAt:       timeOrNil(in.GetClaimedAt()),
		CreatedBy:       stringOrNil(in.GetCreatedBy()),
		CreatedAt:       timeOrNil(in.GetCreatedAt()),
		UpdatedAt:       timeOrNil(in.GetUpdatedAt()),
	}
}

// paymentProviderName 去掉枚举前缀后返回支付渠道名。
func paymentProviderName(provider walletpb.PaymentProvider) string {
	if provider == walletpb.PaymentProvider_PAYMENT_PROVIDER_UNSPECIFIED {
		return ""
	}
	return strings.TrimPrefix(walletpb.PaymentProvider_name[int32(provider)], "PAYMENT_PROVIDER_")
}

// providerEnvironmentName 去掉枚举前缀后返回渠道环境名。
func providerEnvironmentName(environment walletpb.ProviderEnvironment) string {
	if environment == walletpb.ProviderEnvironment_PROVIDER_ENVIRONMENT_UNSPECIFIED {
		return ""
	}
	return strings.TrimPrefix(walletpb.ProviderEnvironment_name[int32(environment)], "PROVIDER_ENVIRONMENT_")
}

// redemptionCodeTypeName 去掉枚举前缀后返回兑换码类型名。
func redemptionCodeTypeName(codeType walletpb.RedemptionCodeType) string {
	if codeType == walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNSPECIFIED {
		return ""
	}
	return strings.TrimPrefix(walletpb.RedemptionCodeType_name[int32(codeType)], "REDEMPTION_CODE_TYPE_")
}

// redemptionCodeStatusName 去掉枚举前缀后返回兑换码状态名。
func redemptionCodeStatusName(status walletpb.RedemptionCodeStatus) string {
	if status == walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_UNSPECIFIED {
		return ""
	}
	return strings.TrimPrefix(walletpb.RedemptionCodeStatus_name[int32(status)], "REDEMPTION_CODE_STATUS_")
}
