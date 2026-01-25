package api

import (
	system2 "github.com/KiClover/orzAdmin/api/system"
	"go.uber.org/fx"
)

func Module() fx.Option {
	return fx.Module(
		"api.svc",
		system2.Module(),
	)
}
