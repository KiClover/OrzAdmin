package etcd

import (
	"context"

	"github.com/KiClover/orzfrp/conf"
	"github.com/cloudwego/hertz/pkg/app/server/registry"
	hetcd "github.com/hertz-contrib/registry/etcd"
	"github.com/sirupsen/logrus"
)

func NewHertzEtcdRegistry(ctx context.Context, cfg *conf.Config) (registry.Registry, error) {
	ctx, span := tracer.Start(ctx, "etcd_reg.NewHertzEtcdRegistry")
	defer span.End()
	endpoints := cfg.EtcdConf.Endpoints

	r, err := hetcd.NewEtcdRegistry(endpoints)
	if err != nil {
		logrus.WithContext(ctx).Errorf("failed to create etcd registry: %v", err)
		return nil, err
	}
	return r, nil
}
