package user

import (
	"context"

	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/db"
	base "github.com/KiClover/orzfrp/kitex_gen/base"
	user "github.com/KiClover/orzfrp/kitex_gen/system/user"
	"go.uber.org/fx"
)

// SystemUserService implements the last service interface defined in the IDL.
type SystemUserService struct {
	fx.In
	DB  *db.UserServiceDB
	Cfg *conf.Config
}

var _ user.SystemUserService = (*SystemUserService)(nil)

func NewSystemUserService(svc SystemUserService) user.SystemUserService {
	return &svc
}

// CreateUser implements the SystemUserServiceImpl interface.
func (s *SystemUserService) CreateUser(ctx context.Context, req *user.UserInfo) (resp *base.BaseResp, err error) {
	s.DB.User.Create().Save(ctx)
	return
}
