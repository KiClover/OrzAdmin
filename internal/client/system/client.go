package system

import (
	"fmt"

	"github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	"github.com/KiClover/orzfrp/services/system/user"
	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/discovery"
)

func NewUserServiceClient(r *discovery.Resolver) systemuserservice.Client {
	c, err := systemuserservice.NewClient(user.ServiceName, client.WithResolver(*r))
	if err != nil {
		panic(fmt.Errorf("cannot connect to %s client:%v", user.ServiceName, err))
	}
	return c
}
