package discovery

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/cloudwego/kitex/pkg/discovery"
	"github.com/cloudwego/kitex/pkg/registry"
	ketcd "github.com/kitex-contrib/registry-etcd"
	nacosRegistry "github.com/kitex-contrib/registry-nacos/v2/registry"
	nacosResolver "github.com/kitex-contrib/registry-nacos/v2/resolver"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("github.com/KiClover/orzAdmin")

func NewKitexEtcdRegistry(ctx context.Context, cfg *conf.Config) (reg registry.Registry, err error) {
	ctx, span := tracer.Start(ctx, "etcd_reg.NewKitexEtcdRegistry")
	defer span.End()
	var r registry.Registry
	switch cfg.DiscoveryConf.Method {
	case "etcd":
		endpoints := cfg.EtcdConf.Endpoints
		r, err = ketcd.NewEtcdRegistry(endpoints)
		if err != nil {
			logrus.WithContext(ctx).Errorf("failed to create etcd registry: %v", err)
			return nil, err
		}
	case "nacos":
		cli, err := newNacosNamingClient(cfg)
		if err != nil {
			logrus.WithContext(ctx).Errorf("failed to create nacos naming client: %v", err)
			return nil, err
		}
		r = nacosRegistry.NewNacosRegistry(cli)
	default:
		return nil, fmt.Errorf("unsupported discovery method: %s", cfg.DiscoveryConf.Method)
	}

	return r, nil
}

func NewKitexEtcdResolver(ctx context.Context, cfg *conf.Config) (*discovery.Resolver, error) {
	ctx, span := tracer.Start(ctx, "etcd_reg.NewKitexEtcdResolver")
	defer span.End()
	switch cfg.DiscoveryConf.Method {
	case "etcd":
		endpoints := cfg.EtcdConf.Endpoints
		r, err := ketcd.NewEtcdResolver(endpoints)
		if err != nil {
			logrus.WithContext(ctx).Errorf("failed to create etcd resolver: %v", err)
			return nil, err
		}
		return &r, nil
	case "nacos":
		cli, err := newNacosNamingClient(cfg)
		if err != nil {
			logrus.WithContext(ctx).Errorf("failed to create nacos naming client: %v", err)
			return nil, err
		}
		r := nacosResolver.NewNacosResolver(cli)
		return &r, nil
	default:
		return nil, fmt.Errorf("unsupported discovery method: %s", cfg.DiscoveryConf.Method)
	}
}

func newNacosNamingClient(cfg *conf.Config) (naming_client.INamingClient, error) {
	sc := make([]constant.ServerConfig, 0, len(cfg.NacosConf.Addresses))
	for _, addr := range cfg.NacosConf.Addresses {
		host, port, err := splitHostPortWithDefault(addr, 8848)
		if err != nil {
			return nil, err
		}
		sc = append(sc, *constant.NewServerConfig(host, port))
	}

	cc := constant.ClientConfig{
		NamespaceId:          cfg.NacosConf.NamespaceId,
		TimeoutMs:            cfg.NacosConf.TimeoutMs,
		NotLoadCacheAtStart:  true,
		UpdateCacheWhenEmpty: true,
		Username:             cfg.NacosConf.Username,
		Password:             cfg.NacosConf.Password,
		LogLevel:             "info",
	}

	cli, err := clients.NewNamingClient(
		vo.NacosClientParam{ClientConfig: &cc, ServerConfigs: sc},
	)
	if err != nil {
		return nil, err
	}
	return cli, nil
}

func splitHostPortWithDefault(addr string, defaultPort uint64) (string, uint64, error) {
	if host, portStr, err := net.SplitHostPort(addr); err == nil {
		p, err := strconv.ParseUint(portStr, 10, 64)
		if err != nil {
			return "", 0, err
		}
		return host, p, nil
	}

	return addr, defaultPort, nil
}
