package wallet

import "go.uber.org/fx"

// Module 提供 Finance 用户钱包 HTTP handler。
func Module() fx.Option {
	return fx.Module(
		"finance.wallet.handler",
		fx.Provide(NewHzFinanceWalletService),
	)
}
