package services

import (
	"github.com/KiClover/orzfrp/services/system"
	"go.uber.org/fx"
)

func Module() fx.Option {
	return fx.Options(
		system.Module(),
	)
}
