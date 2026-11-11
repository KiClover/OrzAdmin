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
	internallog "github.com/KiClover/orzAdmin/internal/log"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/KiClover/orzAdmin/pkg/kerr"
	"github.com/KiClover/orzAdmin/pkg/utils"
	"github.com/casbin/casbin/v2"
	"go.uber.org/fx"
)

const ServiceName = "system-setting"

// logger 让 setting 服务的日志统一带 [SETTING] 前缀。
var logger = internallog.WithModule(internallog.ModuleSetting)

type SystemSettingService struct {
	fx.In
	DB     *db.SettingServiceDB
	Cfg    *conf.Config
	DBAll  *ent.Client
	Casbin *casbin.Enforcer

	// Validator 在写入钱包配置前远程校验 Creem Profile；未配置时拒绝保存 Profile。
	Validator WalletProfileValidator `optional:"true"`
}

var _ setting.SystemSettingService = (*SystemSettingService)(nil)

func NewSystemSettingService(svc SystemSettingService) setting.SystemSettingService {
	return &svc
}

// Logic

// InitDatabase 同步 settings 表并补齐缺失的默认设置项，重复执行不产生重复数据。
func (s SystemSettingService) InitDatabase(ctx context.Context, req *base.Empty) (res *base.BaseResp, err error) {
	tables := []*schema.Table{migrate.SettingsTable}
	if err := migrate.Create(ctx, s.DBAll.Schema, tables, schema.WithForeignKeys(false)); err != nil {
		logger.Errorf("init database err: %v", err)
		return nil, err
	}
	if err := seedSettings(ctx, s.DB); err != nil {
		logger.Errorf("init settings seed err: %v", err)
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
		// 用生成的 Get* 取值：字段是 optional（指针），前端漏传时为 nil，
		// 直接解引用会 panic，Get* 会退化成零值。
		{"email_host", req.GetHost()},
		{"email_port", strconv.Itoa(int(req.GetPort()))},
		{"email_user", req.GetUser()},
		{"email_password", req.GetPassword()},
		{"email_display_name", req.GetDisplayName()},
		{"email_crypt_type", strconv.Itoa(int(req.GetCryptType()))},
		{"email_from", req.GetFrom()},
	}

	for _, update := range updates {
		_, err = tx.Setting.Update().
			Where(settingEnt.Key(update.key)).
			SetValue(update.value).
			Save(ctx)
		if err != nil {
			tx.Rollback()
			logger.Errorf("set email provider err: %v", err)
			return nil, err
		}
	}

	err = tx.Commit()
	if err != nil {
		logger.Errorf("tx commit transaction err: %v", err)
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

	emailFrom, err := s.DB.Setting.Query().Where(settingEnt.Key("email_from")).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}

	// 未配置时数据库里存的是空串，这里按 0 兜底，避免初始化后 GET 直接报错
	port := atoiOrZero(emailPort.Value)
	cryptType := atoiOrZero(emailCryptType.Value)

	return &setting.EmailProviderInfo{
		Host:        &emailHost.Value,
		Port:        utils.GetPointer(uint32(port)),
		User:        &emailUser.Value,
		Password:    &emailPassword.Value,
		DisplayName: &emailDisplayName.Value,
		CryptType:   utils.GetPointer(uint32(cryptType)),
		From:        &emailFrom.Value,
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
		SetValue(strconv.Itoa(int(req.GetType()))).
		Save(ctx)
	if err != nil {
		logger.Errorf("set sms type err: %v", err)
		return nil, err
	}

	_, err = tx.Setting.Update().
		Where(settingEnt.Key("sms_access_id")).
		SetValue(req.GetAccessId()).
		Save(ctx)
	if err != nil {
		logger.Errorf("set sms access id err: %v", err)
		return nil, err
	}

	_, err = tx.Setting.Update().
		Where(settingEnt.Key("sms_access_key")).
		SetValue(req.GetAccessKey()).
		Save(ctx)
	if err != nil {
		logger.Errorf("set sms access key err: %v", err)
		return nil, err
	}

	_, err = tx.Setting.Update().
		Where(settingEnt.Key("sms_sign_name")).
		SetValue(req.GetSignName()).
		Save(ctx)
	if err != nil {
		logger.Errorf("set sms sign name err: %v", err)
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		logger.Errorf("commit transaction err: %v", err)
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

	// 未配置时数据库里存的是空串，这里按 0 兜底
	smsTypeInt := atoiOrZero(smsType.Value)

	return &setting.SmsProviderInfo{
		Type:      utils.GetPointer(uint32(smsTypeInt)),
		AccessId:  &smsAccessId.Value,
		AccessKey: &smsAccessKey.Value,
		SignName:  &smsSignName.Value,
	}, nil
}

// atoiOrZero 把 setting 表里以字符串形式保存的数字配置转成 int，
// 未配置（空串）或值非法时按 0 处理，表示"尚未配置"。
func atoiOrZero(value string) int {
	num, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return num
}

// ============================ 钱包设置 ============================
//
// 三套接口严格分离：
//   - GetWalletSettingsMasked：管理视图，只返回 configured/掩码/指纹；
//   - UpdateWalletSettings：expected_revision CAS 更新，远程校验 Profile；
//   - GetWalletRuntimeSettings：内部运行时视图，含完整 Secret，仅供 Commerce 调用。
//
// 快照解码、校验、脱敏与补丁合并都在 wallet_config.go 中实现。

// GetWalletSettingsMasked 返回钱包设置的管理视图（掩码）。
func (s SystemSettingService) GetWalletSettingsMasked(ctx context.Context, req *base.Empty) (res *setting.WalletSettingsMasked, err error) {
	return s.getWalletSettingsMasked(ctx)
}

// UpdateWalletSettings 更新钱包设置并返回新的掩码视图。
func (s SystemSettingService) UpdateWalletSettings(ctx context.Context, req *setting.UpdateWalletSettingsReq) (res *setting.WalletSettingsMasked, err error) {
	return s.updateWalletSettings(ctx, req)
}

// GetWalletRuntimeSettings 返回钱包设置的运行时视图（含完整 Secret），仅 Commerce 调用。
func (s SystemSettingService) GetWalletRuntimeSettings(ctx context.Context, req *base.Empty) (res *setting.WalletRuntimeSettings, err error) {
	return s.getWalletRuntimeSettings(ctx)
}
