package finance

import (
	"github.com/KiClover/orzAdmin/api/finance/wallet"
	"go.uber.org/fx"
)

// Module 聚合 Finance 侧（用户钱包）API 模块。
func Module() fx.Option {
	return fx.Module(
		"finance.handler",
		wallet.Module(),
	)
}
