package user

import (
	"context"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/db"
	"github.com/KiClover/orzfrp/ent"
	"github.com/KiClover/orzfrp/ent/migrate"
	base "github.com/KiClover/orzfrp/kitex_gen/base"
	user "github.com/KiClover/orzfrp/kitex_gen/system/user"
	"github.com/KiClover/orzfrp/pkg/utils"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

const ServiceName = "system-user"

// SystemUserService implements the last service interface defined in the IDL.
type SystemUserService struct {
	fx.In
	DB    *db.UserServiceDB
	Cfg   *conf.Config
	DBAll *ent.Client
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

func (s *SystemUserService) InitDatabase(ctx context.Context, req *base.Empty) (res *base.BaseResp, err error) {
	tables := []*schema.Table{migrate.UsersTable}
	if err := migrate.Create(ctx, s.DBAll.Schema, tables, schema.WithForeignKeys(false)); err != nil {
		logrus.Errorf("init database err: %v", err)
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

func (s *SystemUserService) GetUserByEmail(ctx context.Context, req *user.GetUserByEmailReq) (resp *user.UserInfo, err error) {
	// todo:编写具体逻辑
	return nil, nil
}

func (s *SystemUserService) GetUserByMobile(ctx context.Context, req *user.GetUserByMobileReq) (resp *user.UserInfo, err error) {
	// todo:编写具体逻辑
	return nil, nil
}

func (s *SystemUserService) GetUserById(ctx context.Context, req *user.GetUserByIdReq) (res *user.UserInfo, err error) {
	// todo:编写具体逻辑
	return nil, nil
}
