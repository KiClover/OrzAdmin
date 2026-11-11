package setting

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	"github.com/cloudwego/kitex/pkg/kerrors"
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

// GetWalletSetting 读取钱包配置的管理视图（掩码、configured、指纹）。
// GET /setting/wallet
func (s *HzSystemSettingService) GetWalletSetting(ctx context.Context, c *app.RequestContext) {
	r, err := s.SettingClient.GetWalletSettingsMasked(ctx, &base.Empty{})
	if err != nil {
		writeWalletSettingsError(c, err)
		return
	}
	hresp.BaseSuccessMsgWithBody(c, "get wallet setting success", walletSettingsToAPI(r))
}

// UpdateWalletSetting 以 expected_revision 做乐观并发控制更新钱包配置。
// PUT /setting/wallet
func (s *HzSystemSettingService) UpdateWalletSetting(ctx context.Context, c *app.RequestContext) {
	var req settingapi.UpdateWalletSettingReq
	if err := c.BindAndValidate(&req); err != nil {
		hresp.ReqBindErr(c, err)
		return
	}

	operatorID, ok := walletOperatorID(c)
	if !ok {
		hresp.NotSuccessErr(c, http.StatusUnauthorized, walletSettingsCodeInvalidArgument, "admin identity is required")
		return
	}
	// 负数是上游字段类型（int32）与 RPC 类型（uint32）之间的非法输入。
	if (req.RedemptionRateLimit != nil && *req.RedemptionRateLimit < 0) ||
		(req.RedemptionRateWindowSeconds != nil && *req.RedemptionRateWindowSeconds < 0) {
		hresp.NotSuccessErr(c, http.StatusBadRequest, walletSettingsCodeInvalidArgument,
			"redemption rate limit and window must not be negative")
		return
	}

	r, err := s.SettingClient.UpdateWalletSettings(ctx, walletUpdateToRPC(operatorID, &req))
	if err != nil {
		writeWalletSettingsError(c, err)
		return
	}
	hresp.BaseSuccessMsgWithBody(c, "update wallet setting success", walletSettingsToAPI(r))
}

// walletSettingsCodeInvalidArgument 是钱包配置接口对外的参数错误码。
const walletSettingsCodeInvalidArgument int32 = 2

// walletSettingsHTTPStatus 把 Setting 服务返回的业务错误码映射为稳定的 HTTP 状态码。
//
// 参数/配置非法 → 400；配置不存在 → 404；revision 冲突 → 409；
// Provider 拒绝 → 422；暂时不可用 → 503；其余一律 500。
var walletSettingsHTTPStatus = map[int32]int{
	settingsvc.WalletSettingCodeInvalidArgument:  http.StatusBadRequest,
	settingsvc.WalletSettingCodeNotFound:         http.StatusNotFound,
	settingsvc.WalletSettingCodeRevisionConflict: http.StatusConflict,
	settingsvc.WalletSettingCodeProviderRejected: http.StatusUnprocessableEntity,
	settingsvc.WalletSettingCodeUnavailable:      http.StatusServiceUnavailable,
	settingsvc.WalletSettingCodeInternal:         http.StatusInternalServerError,
}

// writeWalletSettingsError 输出稳定的钱包配置错误响应。
func writeWalletSettingsError(c *app.RequestContext, err error) {
	var bizErr *kerrors.BizStatusError
	if errors.As(err, &bizErr) {
		status, ok := walletSettingsHTTPStatus[bizErr.BizStatusCode()]
		if !ok {
			status = http.StatusInternalServerError
		}
		msg := bizErr.BizMessage()
		if strings.TrimSpace(msg) == "" {
			msg = "wallet settings request failed"
		}
		hresp.NotSuccessErr(c, status, bizErr.BizStatusCode(), msg)
		return
	}
	hresp.NotSuccessErr(c, http.StatusInternalServerError, settingsvc.WalletSettingCodeInternal,
		"wallet settings are temporarily unavailable")
}

// walletOperatorID 从 JWT 中间件写入的 userId 中取出管理员 ID。
func walletOperatorID(c *app.RequestContext) (string, bool) {
	raw, ok := c.Get("userId")
	if !ok {
		return "", false
	}
	switch v := raw.(type) {
	case uint64:
		if v == 0 {
			return "", false
		}
		return strconv.FormatUint(v, 10), true
	case int64:
		if v <= 0 {
			return "", false
		}
		return strconv.FormatInt(v, 10), true
	case string:
		if v == "" {
			return "", false
		}
		return v, true
	default:
		return "", false
	}
}

// walletSettingsToAPI 把 Setting 服务的掩码视图转换为 HTTP DTO。
func walletSettingsToAPI(in *settingrpc.WalletSettingsMasked) *settingapi.GetWalletSettingResp {
	if in == nil {
		return &settingapi.GetWalletSettingResp{}
	}
	rateLimit := int32(in.GetRedemptionRateLimit())
	rateWindow := int32(in.GetRedemptionRateWindowSeconds())
	return &settingapi.GetWalletSettingResp{
		Revision:                     utils.GetPointer(in.GetRevision()),
		Enabled:                      utils.GetPointer(in.GetEnabled()),
		RechargeMode:                 utils.GetPointer(in.GetRechargeMode()),
		RedemptionEnabled:            utils.GetPointer(in.GetRedemptionEnabled()),
		RedemptionRateLimit:          &rateLimit,
		RedemptionRateWindowSeconds:  &rateWindow,
		WalletCurrency:               utils.GetPointer(in.GetWalletCurrency()),
		ProviderCurrency:             utils.GetPointer(in.GetProviderCurrency()),
		CnyPerUsdRate:                utils.GetPointer(in.GetCnyPerUsdRate()),
		ExchangeSampleWalletAmount:   utils.GetPointer(in.GetExchangeSampleWalletAmount()),
		ExchangeSampleProviderAmount: utils.GetPointer(in.GetExchangeSampleProviderAmount()),
		TestProfile:                  walletProfileToAPI(in.GetTestProfile()),
		ProductionProfile:            walletProfileToAPI(in.GetProductionProfile()),
		UpdatedBy:                    utils.GetPointer(in.GetUpdatedBy()),
		UpdatedAt:                    formatMillisPtr(in.GetUpdatedAt()),
		Stale:                        utils.GetPointer(in.GetStale()),
	}
}

// walletProfileToAPI 转换单个 Profile 的掩码视图。
func walletProfileToAPI(in *settingrpc.WalletProviderProfileMasked) *settingapi.WalletSettingProfileMasked {
	if in == nil {
		return nil
	}
	out := &settingapi.WalletSettingProfileMasked{
		Environment:       utils.GetPointer(in.GetEnvironment()),
		ProfileID:         utils.GetPointer(in.GetProfileId()),
		ProfileRevision:   utils.GetPointer(in.GetProfileRevision()),
		ProductID:         utils.GetPointer(in.GetProductId()),
		WebhookID:         utils.GetPointer(in.GetWebhookId()),
		APIKeyMasked:      utils.GetPointer(in.GetApiKeyMasked()),
		APIKeyConfigured:  utils.GetPointer(in.GetApiKeyConfigured()),
		APIKeyFingerprint: utils.GetPointer(in.GetApiKeyFingerprint()),
		VerifiedAt:        formatMillisPtr(in.GetVerifiedAt()),
	}
	for _, secret := range in.GetWebhookSecrets() {
		out.WebhookSecrets = append(out.WebhookSecrets, &settingapi.WalletSettingSecretMasked{
			EndpointID:       utils.GetPointer(secret.GetEndpointId()),
			Fingerprint:      utils.GetPointer(secret.GetFingerprint()),
			SecretMasked:     utils.GetPointer(secret.GetSecretMasked()),
			SecretConfigured: utils.GetPointer(secret.GetSecretConfigured()),
			DisabledAt:       formatMillisPtr(secret.GetDisabledAt()),
			RetireAfter:      formatMillisPtr(secret.GetRetireAfter()),
		})
	}
	return out
}

// walletUpdateToRPC 把 HTTP 补丁转换为内部 RPC 请求。
//
// operator_id 只能来自认证上下文，HTTP 请求体无法指定。
func walletUpdateToRPC(operatorID string, req *settingapi.UpdateWalletSettingReq) *settingrpc.UpdateWalletSettingsReq {
	out := &settingrpc.UpdateWalletSettingsReq{
		ExpectedRevision:  req.GetExpectedRevision(),
		Enabled:           req.Enabled,
		RechargeMode:      req.RechargeMode,
		RedemptionEnabled: req.RedemptionEnabled,
		CnyPerUsdRate:     req.CnyPerUsdRate,
		OperatorId:        operatorID,
	}
	if req.RedemptionRateLimit != nil {
		value := uint32(*req.RedemptionRateLimit)
		out.RedemptionRateLimit = &value
	}
	if req.RedemptionRateWindowSeconds != nil {
		value := uint32(*req.RedemptionRateWindowSeconds)
		out.RedemptionRateWindowSeconds = &value
	}
	for _, profile := range req.Profiles {
		patch := &settingrpc.WalletProviderProfilePatch{
			ProfileId:      profile.ProfileID,
			ProductId:      profile.ProductID,
			WebhookId:      profile.WebhookID,
			ApiKey:         profile.APIKey,
			WebhookSecrets: nil,
		}
		if profile.Environment != nil {
			patch.Environment = *profile.Environment
		}
		for _, secret := range profile.WebhookSecrets {
			patch.WebhookSecrets = append(patch.WebhookSecrets, &settingrpc.WalletWebhookSecretPatch{
				Secret:     secret.GetSecret(),
				EndpointId: secret.GetEndpointID(),
			})
		}
		out.Profiles = append(out.Profiles, patch)
	}
	return out
}

// formatMillisPtr 把 Unix 毫秒时间戳格式化为 RFC3339 毫秒字符串；0 表示"从未发生"。
func formatMillisPtr(ms int64) *string {
	if ms <= 0 {
		return nil
	}
	formatted := time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")
	return &formatted
}
