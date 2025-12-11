package setting

import (
	"context"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/db"
	"github.com/KiClover/orzfrp/ent"
	"github.com/KiClover/orzfrp/ent/migrate"
	"github.com/KiClover/orzfrp/kitex_gen/base"
	"github.com/KiClover/orzfrp/kitex_gen/system/setting"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

const ServiceName = "system-setting"

type SystemSettingService struct {
	fx.In
	DB    *db.SettingServiceDB
	Cfg   *conf.Config
	DBAll *ent.Client
}

var _ setting.SystemSettingService = (*SystemSettingService)(nil)

// Logic

func (s SystemSettingService) InitDatabase(ctx context.Context, req *base.Empty) (res *base.BaseResp, err error) {
	tables := []*schema.Table{migrate.SettingsTable}
	if err := migrate.Create(ctx, s.DBAll.Schema, tables, schema.WithForeignKeys(false)); err != nil {
		logrus.Errorf("init database err: %v", err)
		return nil, err
	}

	return &base.BaseResp{
		Msg: "success",
	}, nil
}
func (s SystemSettingService) ChooseRegUserRole(ctx context.Context, req *setting.ChooseRegUserRoleReq) (res *base.BaseResp, err error) {
	//TODO implement me
	panic("implement me")
}
