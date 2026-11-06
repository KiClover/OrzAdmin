package setting

import (
	"context"
	"strings"

	settingapi "github.com/KiClover/orzAdmin/api/model/system/setting"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	settingrpc "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzAdmin/pkg/hresp"
	"github.com/KiClover/orzAdmin/pkg/utils"
	settingsvc "github.com/KiClover/orzAdmin/services/system/setting"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

const ServiceName = "api.system.setting"

type HzSystemSettingServiceP struct {
	fx.In
	Cfg           *conf.Config
	SettingClient systemsettingservice.Client
	Casbin        *casbin.Enforcer
	Redis         redis.UniversalClient
}
type HzSystemSettingService struct {
	Cfg           *conf.Config
	SettingClient systemsettingservice.Client
	Casbin        *casbin.Enforcer
	Redis         redis.UniversalClient
}

func NewHzSystemSettingService(p HzSystemSettingServiceP) *HzSystemSettingService {
	return &HzSystemSettingService{
		Cfg:           p.Cfg,
		SettingClient: p.SettingClient,
		Redis:         p.Redis,
		Casbin:        p.Casbin,
	}
}

// SetRegUserRole 设置注册用户的默认角色
// POST /setting/register/role
func (s *HzSystemSettingService) SetRegUserRole(ctx context.Context, c *app.RequestContext) {
	var req settingapi.SetRegUserRoleReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	role := strings.TrimSpace(req.GetRole())
	if role == "" {
		hresp.BaseErrMsg(c, 2, "role is required", nil)
		return
	}

	_, err := s.SettingClient.ChangeRegUserRole(ctx, &settingrpc.ChangeRegUserRoleReq{
		Role: role,
	})
	if err != nil {
		hresp.KitexCallErr(c, settingsvc.ServiceName, err)
		return
	}
	hresp.BaseSuccessMsg(c, "set register role success")
}

// GetRegUserRole 获取注册用户的默认角色
// GET /setting/register/role
func (s *HzSystemSettingService) GetRegUserRole(ctx context.Context, c *app.RequestContext) {
	r, err := s.SettingClient.GetRegUserRole(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, settingsvc.ServiceName, err)
		return
	}
	hresp.BaseSuccessMsgWithBody(c, "get register role success", &settingapi.GetRegUserRoleResp{
		Role: utils.GetPointer(r.GetRole()),
	})
}

// SetEmailSetting 设置邮件服务配置
// POST /setting/email
func (s *HzSystemSettingService) SetEmailSetting(ctx context.Context, c *app.RequestContext) {
	var req settingapi.SetEmailSettingReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}

	// RPC 层会直接解引用这些字段，缺一个都会 panic，这里先兜住
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"host", req.Host},
		{"user", req.User},
		{"password", req.Password},
		{"from", req.From},
	} {
		if field.value == nil || strings.TrimSpace(*field.value) == "" {
			hresp.BaseErrMsg(c, 2, field.name+" is required", nil)
			return
		}
	}
	if req.Port == nil || *req.Port <= 0 || *req.Port > 65535 {
		hresp.BaseErrMsg(c, 2, "port must be between 1 and 65535", nil)
		return
	}
	// 1 SSL 2 TLS 3 None
	if req.CryptType == nil || *req.CryptType < 1 || *req.CryptType > 3 {
		hresp.BaseErrMsg(c, 2, "crypt_type must be 1(SSL), 2(TLS) or 3(None)", nil)
		return
	}

	displayName := utils.GetPointer("")
	if req.DisplayName != nil {
		displayName = req.DisplayName
	}

	_, err := s.SettingClient.SetEmailProvider(ctx, &settingrpc.EmailProviderInfo{
		Host:        req.Host,
		Port:        utils.GetPointer(uint32(*req.Port)),
		User:        req.User,
		Password:    req.Password,
		DisplayName: displayName,
		CryptType:   utils.GetPointer(uint32(*req.CryptType)),
		From:        req.From,
	})
	if err != nil {
		hresp.KitexCallErr(c, settingsvc.ServiceName, err)
		return
	}
	hresp.BaseSuccessMsg(c, "set email setting success")
}

// GetEmailSetting 获取邮件服务配置
// GET /setting/email
func (s *HzSystemSettingService) GetEmailSetting(ctx context.Context, c *app.RequestContext) {
	r, err := s.SettingClient.GetEmailProvider(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, settingsvc.ServiceName, err)
		return
	}

	port := int32(r.GetPort())
	cryptType := int32(r.GetCryptType())
	hresp.BaseSuccessMsgWithBody(c, "get email setting success", &settingapi.GetEmailSettingResp{
		Host:        utils.GetPointer(r.GetHost()),
		Port:        &port,
		CryptType:   &cryptType,
		User:        utils.GetPointer(r.GetUser()),
		Password:    utils.GetPointer(r.GetPassword()),
		DisplayName: utils.GetPointer(r.GetDisplayName()),
		From:        utils.GetPointer(r.GetFrom()),
	})
}

// SetSmsSetting 设置短信服务配置
// POST /setting/mobile
func (s *HzSystemSettingService) SetSmsSetting(ctx context.Context, c *app.RequestContext) {
	var req settingapi.SetSmsSettingReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}

	// RPC 层会直接解引用这些字段，缺一个都会 panic，这里先兜住
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"access_id", req.AccessID},
		{"access_key", req.AccessKey},
		{"sign_name", req.SignName},
	} {
		if field.value == nil || strings.TrimSpace(*field.value) == "" {
			hresp.BaseErrMsg(c, 2, field.name+" is required", nil)
			return
		}
	}
	// 1 SMSBao
	if req.Type == nil || *req.Type < 1 {
		hresp.BaseErrMsg(c, 2, "type must be 1(SMSBao)", nil)
		return
	}

	_, err := s.SettingClient.SetSmsProvider(ctx, &settingrpc.SmsProviderInfo{
		Type:      utils.GetPointer(uint32(*req.Type)),
		AccessId:  req.AccessID,
		AccessKey: req.AccessKey,
		SignName:  req.SignName,
	})
	if err != nil {
		hresp.KitexCallErr(c, settingsvc.ServiceName, err)
		return
	}
	hresp.BaseSuccessMsg(c, "set sms setting success")
}

// GetSmsSetting 获取短信服务配置
// GET /setting/mobile
func (s *HzSystemSettingService) GetSmsSetting(ctx context.Context, c *app.RequestContext) {
	r, err := s.SettingClient.GetSmsProvider(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, settingsvc.ServiceName, err)
		return
	}

	smsType := int32(r.GetType())
	hresp.BaseSuccessMsgWithBody(c, "get sms setting success", &settingapi.GetSmsSettingResp{
		Type:      &smsType,
		AccessID:  utils.GetPointer(r.GetAccessId()),
		AccessKey: utils.GetPointer(r.GetAccessKey()),
		SignName:  utils.GetPointer(r.GetSignName()),
	})
}
