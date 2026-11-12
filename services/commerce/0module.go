package commerce

import (
	"github.com/KiClover/orzAdmin/services/commerce/wallet"
	"go.uber.org/fx"
)

// Module 提供 Commerce 域的服务实现。
func Module() fx.Option {
	return fx.Options(
		wallet.Module(),
	)
}
