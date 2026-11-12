package wallet

import (
	"context"
	"errors"
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
	financeCodeInvalidArgument int32 = 1
	financeCodeFeatureDisabled int32 = 9
	financeCodeRateLimited     int32 = 10
	financeCodeUnavailable     int32 = 12
	financeCodeInternal        int32 = 13
)

// HzFinanceWalletServiceP 是 handler 的依赖。
type HzFinanceWalletServiceP struct {
	fx.In

	Cfg          *conf.Config
	WalletClient walletsvc.Client
	Redis        redis.UniversalClient
}

// HzFinanceWalletService 承载用户钱包 HTTP 路由。
type HzFinanceWalletService struct {
	Cfg          *conf.Config
	WalletClient walletsvc.Client
	RateMw       *middleware.RateMw
}

// NewHzFinanceWalletService 构造用户钱包 HTTP handler。
func NewHzFinanceWalletService(p HzFinanceWalletServiceP) (*HzFinanceWalletService, error) {
	rateMw, err := middleware.NewRateMw(p.Cfg, p.Redis)
	if err != nil {
		return nil, err
	}
	return &HzFinanceWalletService{
		Cfg:          p.Cfg,
		WalletClient: p.WalletClient,
		RateMw:       rateMw,
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
// GET /wallet/transactions
func (s *HzFinanceWalletService) ListTransactions(ctx context.Context, c *app.RequestContext) {
	userID, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}

	var req walletapi.ListTransactionsReq
	if err := c.BindQuery(&req); err != nil {
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
// GET /wallet/recharges
func (s *HzFinanceWalletService) ListRecharges(ctx context.Context, c *app.RequestContext) {
	userID, ok := currentUserID(c)
	if !ok {
		writeUnauthorized(c)
		return
	}

	var req walletapi.ListRechargesReq
	if err := c.BindQuery(&req); err != nil {
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
