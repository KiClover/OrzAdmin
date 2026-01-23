package setting

import (
	"context"
	"strconv"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/migrate"
	settingEnt "github.com/KiClover/orzAdmin/ent/setting"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/KiClover/orzAdmin/pkg/kerr"
	"github.com/KiClover/orzAdmin/pkg/utils"
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
	settings := []*ent.SettingCreate{
		s.DB.Setting.Create().SetKey("reg_user_role").SetValue("user"),

		// Email 配置内容
		s.DB.Setting.Create().SetKey("email_host").SetValue(""),
		s.DB.Setting.Create().SetKey("email_port").SetValue(""),
		s.DB.Setting.Create().SetKey("email_user").SetValue(""),
		s.DB.Setting.Create().SetKey("email_password").SetValue(""),
		s.DB.Setting.Create().SetKey("email_display_name").SetValue(""),
		s.DB.Setting.Create().SetKey("email_crypt_type").SetValue(""),
		// 1 SSL 2 TLS 3 None

		// SMS 配置内容
		s.DB.Setting.Create().SetKey("sms_type").SetValue(""),
		// 1 SMSBao
		s.DB.Setting.Create().SetKey("sms_access_id").SetValue(""),
		s.DB.Setting.Create().SetKey("sms_access_key").SetValue(""),
		s.DB.Setting.Create().SetKey("sms_sign_name").SetValue(""),
	}
	_, err = s.DB.Setting.CreateBulk(settings...).Save(ctx)
	if err != nil && !ent.IsConstraintError(err) {
		logrus.Errorf("init database err: %v", err)
		return nil, err
	}

	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemSettingService) ChangeRegUserRole(ctx context.Context, req *setting.ChangeRegUserRoleReq) (res *base.BaseResp, err error) {
	_, err = s.DB.Setting.Update().Where(settingEnt.Key("reg_user_role")).SetValue(req.Role).Save(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemSettingService) GetRegUserRole(ctx context.Context, req *base.Empty) (res *setting.GetRegUserRoleResp, err error) {
	r, err := s.DB.Setting.Query().Where(settingEnt.Key("reg_user_role")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &setting.GetRegUserRoleResp{
		Role: r.Value,
	}, nil
}

func (s SystemSettingService) SetEmailProvider(ctx context.Context, req *setting.EmailProviderInfo) (res *base.BaseResp, err error) {
	tx, err := s.DBAll.Tx(ctx)
	if err != nil {
		return nil, err
	}

	updates := []struct {
		key   string
		value string
	}{
		{"email_host", *req.Host},
		{"email_port", strconv.Itoa(int(*req.Port))},
		{"email_user", *req.User},
		{"email_password", *req.Password},
		{"email_display_name", *req.DisplayName},
		{"email_crypt_type", strconv.Itoa(int(*req.CryptType))},
	}

	for _, update := range updates {
		_, err = tx.Setting.Update().
			Where(settingEnt.Key(update.key)).
			SetValue(update.value).
			Save(ctx)
		if err != nil {
			tx.Rollback()
			logrus.Errorf("set email provider err: %v", err)
			return nil, err
		}
	}

	err = tx.Commit()
	if err != nil {
		logrus.Errorf("tx commit transaction err: %v", err)
		return nil, err
	}

	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemSettingService) GetEmailProvider(ctx context.Context, req *base.Empty) (res *setting.EmailProviderInfo, err error) {
	emailHost, err := s.DB.Setting.Query().Where(settingEnt.Key("email_host")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}

	emailPort, err := s.DB.Setting.Query().Where(settingEnt.Key("email_port")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}

	emailUser, err := s.DB.Setting.Query().Where(settingEnt.Key("email_user")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}

	emailPassword, err := s.DB.Setting.Query().Where(settingEnt.Key("email_password")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}

	emailDisplayName, err := s.DB.Setting.Query().Where(settingEnt.Key("email_display_name")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}

	emailCryptType, err := s.DB.Setting.Query().Where(settingEnt.Key("email_crypt_type")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}

	port, err := strconv.Atoi(emailPort.Value)
	if err != nil {
		logrus.Errorf("convert email port err: %v", err)
		return nil, err
	}

	cryptType, err := strconv.Atoi(emailCryptType.Value)
	if err != nil {
		logrus.Errorf("convert email crypt type err: %v", err)
		return nil, err
	}

	return &setting.EmailProviderInfo{
		Host:        &emailHost.Value,
		Port:        utils.GetPointer(uint32(port)),
		User:        &emailUser.Value,
		Password:    &emailPassword.Value,
		DisplayName: &emailDisplayName.Value,
		CryptType:   utils.GetPointer(uint32(cryptType)),
	}, nil
}

func (s SystemSettingService) SetSmsProvider(ctx context.Context, req *setting.SmsProviderInfo) (res *base.BaseResp, err error) {
	tx, err := s.DBAll.Tx(ctx)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	_, err = tx.Setting.Update().
		Where(settingEnt.Key("sms_type")).
		SetValue(strconv.Itoa(int(*req.Type))).
		Save(ctx)
	if err != nil {
		logrus.Errorf("set sms type err: %v", err)
		return nil, err
	}

	_, err = tx.Setting.Update().
		Where(settingEnt.Key("sms_access_id")).
		SetValue(*req.AccessId).
		Save(ctx)
	if err != nil {
		logrus.Errorf("set sms access id err: %v", err)
		return nil, err
	}

	_, err = tx.Setting.Update().
		Where(settingEnt.Key("sms_access_key")).
		SetValue(*req.AccessKey).
		Save(ctx)
	if err != nil {
		logrus.Errorf("set sms access key err: %v", err)
		return nil, err
	}

	_, err = tx.Setting.Update().
		Where(settingEnt.Key("sms_sign_name")).
		SetValue(*req.SignName).
		Save(ctx)
	if err != nil {
		logrus.Errorf("set sms sign name err: %v", err)
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		logrus.Errorf("commit transaction err: %v", err)
		return nil, err
	}

	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemSettingService) GetSmsProvider(ctx context.Context, req *base.Empty) (res *setting.SmsProviderInfo, err error) {
	smsType, err := s.DB.Setting.Query().Where(settingEnt.Key("sms_type")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}

	smsAccessId, err := s.DB.Setting.Query().Where(settingEnt.Key("sms_access_id")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}

	smsAccessKey, err := s.DB.Setting.Query().Where(settingEnt.Key("sms_access_key")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}

	smsSignName, err := s.DB.Setting.Query().Where(settingEnt.Key("sms_sign_name")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}

	smsTypeInt, err := strconv.Atoi(smsType.Value)
	if err != nil {
		logrus.Errorf("convert sms type err: %v", err)
		return nil, err
	}

	return &setting.SmsProviderInfo{
		Type:      utils.GetPointer(uint32(smsTypeInt)),
		AccessId:  &smsAccessId.Value,
		AccessKey: &smsAccessKey.Value,
		SignName:  &smsSignName.Value,
	}, nil
}
