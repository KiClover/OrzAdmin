package client

import (
	"github.com/KiClover/orzfrp/internal/client/system"
	"go.uber.org/fx"
)

func Module() fx.Option {
	return fx.Module(
		"internal.client",
		system.Module(),
	)
}
