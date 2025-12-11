package system

import (
	"time"

	"github.com/KiClover/orzfrp/kitex_gen/system/message/systemmessageservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzfrp/services/system/message"
	"github.com/KiClover/orzfrp/services/system/setting"
	"github.com/KiClover/orzfrp/services/system/user"
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
