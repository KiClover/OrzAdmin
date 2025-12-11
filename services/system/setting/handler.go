package setting

import (
	"context"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/db"
	"github.com/KiClover/orzfrp/ent"
	"github.com/KiClover/orzfrp/ent/migrate"
	settingEnt "github.com/KiClover/orzfrp/ent/setting"
	"github.com/KiClover/orzfrp/kitex_gen/base"
	"github.com/KiClover/orzfrp/kitex_gen/system/setting"
	"github.com/KiClover/orzfrp/pkg/kerr"
	"github.com/casbin/casbin/v2"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

const ServiceName = "system-setting"

type SystemSettingService struct {
	fx.In
	DB     *db.SettingServiceDB
	Cfg    *conf.Config
	DBAll  *ent.Client
	Casbin *casbin.Enforcer
}

var _ setting.SystemSettingService = (*SystemSettingService)(nil)

func NewSystemSettingService(svc SystemSettingService) setting.SystemSettingService {
	return &svc
}

// Logic

func (s SystemSettingService) InitDatabase(ctx context.Context, req *base.Empty) (res *base.BaseResp, err error) {
	tables := []*schema.Table{migrate.SettingsTable}
	if err := migrate.Create(ctx, s.DBAll.Schema, tables, schema.WithForeignKeys(false)); err != nil {
		logrus.Errorf("init database err: %v", err)
		return nil, err
	}

	_, err = s.DB.Setting.Create().SetKey("reg_user_role").SetValue("user").Save(ctx)
	if err != nil {
		logrus.Errorf("init database err: %v", err)
		return nil, err
	}

	return &base.BaseResp{
		Msg: "success",
	}, nil
}
func (s SystemSettingService) ChooseRegUserRole(ctx context.Context, req *setting.ChooseRegUserRoleReq) (res *base.BaseResp, err error) {
	_, err = s.DB.Setting.Update().Where(settingEnt.Key("reg_user_role")).SetValue(req.Role).Save(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}
