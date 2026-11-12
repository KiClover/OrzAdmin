package wallet

import (
	"github.com/KiClover/orzAdmin/internal/health"
	"github.com/KiClover/orzAdmin/internal/middleware"
	"github.com/cloudwego/hertz/pkg/app/server"
)

// NewRegisterRouter 注册 Finance 用户钱包路由。
//
// 用户路由只使用 JWT 中间件；用户身份完全来自令牌，请求体与 query
// 不允许携带 user_id / wallet_id / operator_id。支付回调不挂 JWT/Casbin，
// 依赖 provider 白名单、请求体上限和 Commerce 内的签名验证。
func NewRegisterRouter(h *server.Hertz, svc *HzFinanceWalletService) {
	root := h
	root.GET("/health", health.Health)

	jwtMw := middleware.NewJwtMw(svc.Cfg)

	root.GET("/wallet", jwtMw.UseJwtMw, svc.GetWallet)
	root.GET("/wallet/transactions", jwtMw.UseJwtMw, svc.ListTransactions)
	root.POST("/wallet/recharges", jwtMw.UseJwtMw, svc.CreateRecharge)
	root.GET("/wallet/recharges", jwtMw.UseJwtMw, svc.ListRecharges)
	root.GET("/wallet/recharges/:rechargeId", jwtMw.UseJwtMw, svc.GetRecharge)
	root.POST("/wallet/recharges/:rechargeId/cancel", jwtMw.UseJwtMw, svc.CancelRecharge)
	root.POST("/wallet/redemption-codes/redeem", jwtMw.UseJwtMw, svc.RateMw.UseRateMw("wallet_redemption"), svc.RedeemCode)

	// Provider 回调：无 JWT/Casbin/CSRF，白名单与验签在 handler/Commerce 内完成。
	root.POST("/wallet/payment-callbacks/:provider", svc.PaymentCallback)
}
