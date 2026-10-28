package system

import (
	"fmt"
	"time"

	"github.com/KiClover/orzAdmin/kitex_gen/system/access/systemaccessservice"
	"github.com/KiClover/orzAdmin/kitex_gen/system/message/systemmessageservice"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzAdmin/kitex_gen/system/token/systemtokenservice"
	"github.com/KiClover/orzAdmin/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzAdmin/services/system/access"
	"github.com/KiClover/orzAdmin/services/system/message"
	"github.com/KiClover/orzAdmin/services/system/setting"
	"github.com/KiClover/orzAdmin/services/system/token"
	"github.com/KiClover/orzAdmin/services/system/user"
	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/discovery"
)

const (
	DefaultRPCTimeout     = 5 * time.Second
	DefaultConnectTimeout = 5 * time.Second
)

func NewUserServiceClient(r *discovery.Resolver) (systemuserservice.Client, error) {
	c, err := systemuserservice.NewClient(
		user.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(DefaultRPCTimeout),
		client.WithConnectTimeout(DefaultConnectTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("create %s client: %w", user.ServiceName, err)
	}
	return c, nil
}

func NewSettingServiceClient(r *discovery.Resolver) (systemsettingservice.Client, error) {
	c, err := systemsettingservice.NewClient(
		setting.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(DefaultRPCTimeout),
		client.WithConnectTimeout(DefaultConnectTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("create %s client: %w", setting.ServiceName, err)
	}
	return c, nil
}

func NewMessageServiceClient(r *discovery.Resolver) (systemmessageservice.Client, error) {
	c, err := systemmessageservice.NewClient(
		message.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(DefaultRPCTimeout),
		client.WithConnectTimeout(DefaultConnectTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("create %s client: %w", message.ServiceName, err)
	}
	return c, nil
}

func NewAccessServiceClient(r *discovery.Resolver) (systemaccessservice.Client, error) {
	c, err := systemaccessservice.NewClient(
		access.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(DefaultRPCTimeout),
		client.WithConnectTimeout(DefaultConnectTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("create %s client: %w", access.ServiceName, err)
	}
	return c, nil
}

func NewTokenServiceClient(r *discovery.Resolver) (systemtokenservice.Client, error) {
	c, err := systemtokenservice.NewClient(
		token.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(DefaultRPCTimeout),
		client.WithConnectTimeout(DefaultConnectTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("create %s client: %w", token.ServiceName, err)
	}
	return c, nil
}
