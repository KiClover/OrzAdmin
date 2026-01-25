package system

import (
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
	"github.com/sirupsen/logrus"
)

const (
	DefaultRPCTimeout     = 5 * time.Second
	DefaultConnectTimeout = 5 * time.Second
)

func NewUserServiceClient(r *discovery.Resolver) systemuserservice.Client {
	c, err := systemuserservice.NewClient(
		user.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(DefaultRPCTimeout),
		client.WithConnectTimeout(DefaultConnectTimeout),
	)
	if err != nil {
		logrus.Errorf("cannot connect to %s client:%v", user.ServiceName, err)
	}
	return c
}

func NewSettingServiceClient(r *discovery.Resolver) systemsettingservice.Client {
	c, err := systemsettingservice.NewClient(
		setting.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(DefaultRPCTimeout),
		client.WithConnectTimeout(DefaultConnectTimeout),
	)
	if err != nil {
		logrus.Errorf("cannot connect to %s client:%v", setting.ServiceName, err)
	}
	return c
}

func NewMessageServiceClient(r *discovery.Resolver) systemmessageservice.Client {
	c, err := systemmessageservice.NewClient(
		message.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(DefaultRPCTimeout),
		client.WithConnectTimeout(DefaultConnectTimeout),
	)
	if err != nil {
		logrus.Errorf("cannot connect to %s client:%v", message.ServiceName, err)
	}
	return c
}

func NewAccessServiceClient(r *discovery.Resolver) systemaccessservice.Client {
	c, err := systemaccessservice.NewClient(
		access.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(DefaultRPCTimeout),
		client.WithConnectTimeout(DefaultConnectTimeout),
	)
	if err != nil {
		logrus.Errorf("cannot connect to %s client:%v", access.ServiceName, err)
	}
	return c
}

func NewClientServiceClient(r *discovery.Resolver) systemtokenservice.Client {
	c, err := systemtokenservice.NewClient(
		access.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(DefaultRPCTimeout),
		client.WithConnectTimeout(DefaultConnectTimeout),
	)
	if err != nil {
		logrus.Errorf("cannot connect to %s client:%v", token.ServiceName, err)
	}
	return c
}
