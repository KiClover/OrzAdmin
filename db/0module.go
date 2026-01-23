package db

import "go.uber.org/fx"

func Module() fx.Option {
	return fx.Module(
		"db.svc",
		fx.Provide(NewSystemUserDB),
		fx.Provide(NewSystemSettingDB),
		fx.Provide(NewSystemMessageDB),
		fx.Provide(NewSystemAccessDB),
		fx.Provide(NewSystemTokenDB),
	)
}
