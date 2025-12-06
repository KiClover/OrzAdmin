package etcd

import "go.uber.org/fx"

func Module() fx.Option {
	return fx.Module(
		"internal.etcd",
		fx.Provide(NewKitexEtcdRegistry),
		fx.Provide(NewKitexEtcdResolver),
	)
}
