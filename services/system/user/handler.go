package user

import (
	"context"

	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/db"
	base "github.com/KiClover/orzfrp/kitex_gen/base"
	user "github.com/KiClover/orzfrp/kitex_gen/system/user"
	"github.com/KiClover/orzfrp/pkg/utils"
	"go.uber.org/fx"
)

const ServiceName = "system-user"

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
	// Crypt password
	pwd, err := utils.HashPassword(*req.Password)
	if err != nil {
		return nil, err
	}
	u := s.DB.User.Create().
		SetName(*req.Name).
		SetPwd(pwd).
		SetGender(uint(*req.Gender)).
		SetPhone(*req.Mobile).
		SetEmail(*req.Email).
		SetAvatar(*req.Avatar)
	_, err = u.Save(ctx)
	if err != nil {
		return nil, err
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s *SystemUserService) UpdateUser(ctx context.Context, req *user.UserInfo) (resp *base.BaseResp, err error) {
	return &base.BaseResp{
		Msg: "success",
	}, nil
}
