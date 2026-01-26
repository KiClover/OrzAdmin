package discovery

import (
	"context"
	"fmt"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/cloudwego/hertz/pkg/app/server/registry"
	hetcd "github.com/hertz-contrib/registry/etcd"
	hnacos "github.com/hertz-contrib/registry/nacos/v2"
	"github.com/sirupsen/logrus"
)

func NewHertzEtcdRegistry(ctx context.Context, cfg *conf.Config) (registry.Registry, error) {
	ctx, span := tracer.Start(ctx, "etcd_reg.NewHertzEtcdRegistry")
	defer span.End()
	switch cfg.DiscoveryConf.Method {
	case "etcd":
		endpoints := cfg.EtcdConf.Endpoints
		r, err := hetcd.NewEtcdRegistry(endpoints)
		if err != nil {
			logrus.WithContext(ctx).Errorf("failed to create etcd registry: %v", err)
			return nil, err
		}
		return r, nil
	case "nacos":
		cli, err := newNacosNamingClient(cfg)
		if err != nil {
			logrus.WithContext(ctx).Errorf("failed to create nacos naming client: %v", err)
			return nil, err
		}

		opts := make([]hnacos.RegistryOption, 0, 2)
		if cfg.NacosConf.Cluster != "" {
			opts = append(opts, hnacos.WithRegistryCluster(cfg.NacosConf.Cluster))
		}
		if cfg.NacosConf.Group != "" {
			opts = append(opts, hnacos.WithRegistryGroup(cfg.NacosConf.Group))
		}
		return hnacos.NewNacosRegistry(cli, opts...), nil
	default:
		return nil, fmt.Errorf("unsupported discovery method: %s", cfg.DiscoveryConf.Method)
	}
}
