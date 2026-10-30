package discovery

import "go.uber.org/fx"

func Module() fx.Option {
	return fx.Module(
		"internal.etcd",
		fx.Provide(NewKitexRegistry),
		fx.Provide(NewKitexResolver),
		fx.Provide(NewHertzRegistry),
	)
}
