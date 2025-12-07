package etcd

import (
	"context"

	"github.com/KiClover/orzfrp/conf"
	"github.com/cloudwego/kitex/pkg/discovery"
	"github.com/cloudwego/kitex/pkg/registry"
	ketcd "github.com/kitex-contrib/registry-etcd"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("github.com/KiClover/orzfrp")

func NewKitexEtcdRegistry(ctx context.Context, cfg *conf.Config) (registry.Registry, error) {
	ctx, span := tracer.Start(ctx, "etcd_reg.NewKitexEtcdRegistry")
	defer span.End()
	endpoints := cfg.EtcdConf.Endpoints

	r, err := ketcd.NewEtcdRegistry(endpoints)
	if err != nil {
		logrus.WithContext(ctx).Errorf("failed to create etcd registry: %v", err)
		return nil, err
	}
	return r, nil
}

func NewKitexEtcdResolver(ctx context.Context, cfg *conf.Config) (*discovery.Resolver, error) {
	ctx, span := tracer.Start(ctx, "etcd_reg.NewKitexEtcdResolver")
	defer span.End()

	endpoints := cfg.EtcdConf.Endpoints

	r, err := ketcd.NewEtcdResolver(endpoints)
	if err != nil {
		logrus.WithContext(ctx).Errorf("failed to create etcd resolver: %v", err)
		return nil, err
	}
	return &r, nil
}
