package user

import (
	"context"
	base "github.com/KiClover/orzfrp/kitex_gen/base"
	user "github.com/KiClover/orzfrp/kitex_gen/system/user"
)

// SystemUserServiceImpl implements the last service interface defined in the IDL.
type SystemUserServiceImpl struct {
}

func NewSystemUserServiceImpl() *SystemUserServiceImpl {
	return &SystemUserServiceImpl{}
}

// CreateUser implements the SystemUserServiceImpl interface.
func (s *SystemUserServiceImpl) CreateUser(ctx context.Context, req *user.UserInfo) (resp *base.BaseResp, err error) {
	// TODO: Your code here...
	return
}
