package wallet

import (
	"github.com/KiClover/orzAdmin/internal/health"
	"github.com/KiClover/orzAdmin/internal/middleware"
	"github.com/cloudwego/hertz/pkg/app/server"
)

// NewRegisterRouter 注册 Finance 钱包路由。
//
// 用户路由只使用 JWT 中间件；用户身份完全来自令牌，请求体与 query
// 不允许携带 user_id / wallet_id / operator_id。支付回调不挂 JWT/Casbin，
// 依赖 provider 白名单、请求体上限和 Commerce 内的签名验证。
//
// 管理员路由在 JWT 之后追加 Casbin 检查：身份同样只来自令牌，操作人
// 不接受请求体或 query 覆盖。
func NewRegisterRouter(h *server.Hertz, svc *HzFinanceWalletService) {
	root := h
	root.GET("/health", health.Health)

	jwtMw := middleware.NewJwtMw(svc.Cfg)
	casbinMw := middleware.NewCasbinMw(svc.Cfg, svc.Enforcer)

	root.GET("/wallet", jwtMw.UseJwtMw, svc.GetWallet)
	root.POST("/wallet/transactions/list", jwtMw.UseJwtMw, svc.ListTransactions)
	root.POST("/wallet/recharges", jwtMw.UseJwtMw, svc.CreateRecharge)
	root.POST("/wallet/recharges/list", jwtMw.UseJwtMw, svc.ListRecharges)
	root.GET("/wallet/recharges/:rechargeId", jwtMw.UseJwtMw, svc.GetRecharge)
	root.POST("/wallet/recharges/:rechargeId/cancel", jwtMw.UseJwtMw, svc.CancelRecharge)
	root.POST("/wallet/recharges/:rechargeId/checkout", jwtMw.UseJwtMw, svc.RetryRechargeCheckout)
	root.POST("/wallet/redemption-codes/redeem", jwtMw.UseJwtMw, svc.RateMw.UseRateMw("wallet_redemption"), svc.RedeemCode)

	// Provider 回调：无 JWT/Casbin/CSRF，白名单与验签在 handler/Commerce 内完成。
	root.POST("/wallet/payment-callbacks/:provider", svc.PaymentCallback)

	// 管理员路由：钱包查询、充值刷新/退款、兑换码管理。
	root.POST("/admin/wallets/list", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminListWallets)
	root.POST("/admin/wallets/get", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminGetWallet)
	root.POST("/admin/transactions/list", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminListTransactions)
	root.POST("/admin/recharges/list", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminListRecharges)
	root.GET("/admin/recharges/:rechargeId", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminGetRecharge)
	root.POST("/admin/recharges/:rechargeId/refresh", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminRefreshRecharge)
	root.POST("/admin/recharges/:rechargeId/checkout", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminRetryRechargeCheckout)
	root.POST("/admin/recharges/:rechargeId/refund", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminRefundRecharge)
	root.POST("/admin/redemption-codes", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminCreateRedemptionCode)
	root.POST("/admin/redemption-codes/list", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminListRedemptionCodes)
	root.POST("/admin/redemption-codes/:codeId/status", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminSetRedemptionCodeStatus)
	root.POST("/admin/wallets/adjustments", jwtMw.UseJwtMw, casbinMw.UseCasbinMw, svc.AdminAdjustBalance)
}
