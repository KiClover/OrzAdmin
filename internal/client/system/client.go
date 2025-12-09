package system

import (
	"time"

	"github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzfrp/services/system/user"
	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/discovery"
	"github.com/sirupsen/logrus"
)

func NewUserServiceClient(r *discovery.Resolver) systemuserservice.Client {
	c, err := systemuserservice.NewClient(
		user.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(999*time.Second),
		client.WithConnectTimeout(999*time.Second),
	)
	if err != nil {
		logrus.Errorf("cannot connect to %s client:%v", user.ServiceName, err)
	}
	return c
}
