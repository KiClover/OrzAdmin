package setting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/KiClover/orzAdmin/kitex_gen/base"
	settingrpc "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	settingsvc "github.com/KiClover/orzAdmin/services/system/setting"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/kitex/client/callopt"
	"github.com/cloudwego/kitex/pkg/kerrors"
)

// fakeSettingClient 只实现钱包相关方法，其余方法保持空实现。
type fakeSettingClient struct {
	masked      *settingrpc.WalletSettingsMasked
	maskedErr   error
	updateErr   error
	lastUpdate  *settingrpc.UpdateWalletSettingsReq
	updateCalls int
}

func (f *fakeSettingClient) InitDatabase(ctx context.Context, req *base.Empty, opts ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}

func (f *fakeSettingClient) ChangeRegUserRole(ctx context.Context, req *settingrpc.ChangeRegUserRoleReq, opts ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}

func (f *fakeSettingClient) GetRegUserRole(ctx context.Context, req *base.Empty, opts ...callopt.Option) (*settingrpc.GetRegUserRoleResp, error) {
	return &settingrpc.GetRegUserRoleResp{}, nil
}

func (f *fakeSettingClient) SetEmailProvider(ctx context.Context, req *settingrpc.EmailProviderInfo, opts ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}

func (f *fakeSettingClient) GetEmailProvider(ctx context.Context, req *base.Empty, opts ...callopt.Option) (*settingrpc.EmailProviderInfo, error) {
	return &settingrpc.EmailProviderInfo{}, nil
}

func (f *fakeSettingClient) SetSmsProvider(ctx context.Context, req *settingrpc.SmsProviderInfo, opts ...callopt.Option) (*base.BaseResp, error) {
	return &base.BaseResp{}, nil
}

func (f *fakeSettingClient) GetSmsProvider(ctx context.Context, req *base.Empty, opts ...callopt.Option) (*settingrpc.SmsProviderInfo, error) {
	return &settingrpc.SmsProviderInfo{}, nil
}

func (f *fakeSettingClient) GetWalletSettingsMasked(ctx context.Context, req *base.Empty, opts ...callopt.Option) (*settingrpc.WalletSettingsMasked, error) {
	if f.maskedErr != nil {
		return nil, f.maskedErr
	}
	return f.masked, nil
}

func (f *fakeSettingClient) UpdateWalletSettings(ctx context.Context, req *settingrpc.UpdateWalletSettingsReq, opts ...callopt.Option) (*settingrpc.WalletSettingsMasked, error) {
	f.updateCalls++
	f.lastUpdate = req
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.masked, nil
}

func (f *fakeSettingClient) GetWalletRuntimeSettings(ctx context.Context, req *base.Empty, opts ...callopt.Option) (*settingrpc.WalletRuntimeSettings, error) {
	return &settingrpc.WalletRuntimeSettings{}, nil
}

// newWalletTestEngine 构造只包含钱包路由的测试引擎，并用注入身份替代 JWT 中间件。
func newWalletTestEngine(t *testing.T, client *fakeSettingClient, identity uint64) *server.Hertz {
	t.Helper()
	svc := &HzSystemSettingService{SettingClient: client}
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	if identity > 0 {
		h.Use(func(ctx context.Context, c *app.RequestContext) {
			c.Set("userId", identity)
			c.Next(ctx)
		})
	}
	h.GET("/setting/wallet", svc.GetWalletSetting)
	h.PUT("/setting/wallet", svc.UpdateWalletSetting)
	return h
}

// maskedResponse 返回一份带掩码字段的代表性响应。
func maskedResponse() *settingrpc.WalletSettingsMasked {
	rateLimit := uint32(3)
	window := uint32(600)
	total := int64(10000)
	provider := int64(1389)
	apiKeyMasked := "********"
	configured := true
	fingerprint := strings.Repeat("a", 64)
	return &settingrpc.WalletSettingsMasked{
		Revision:                     4,
		Enabled:                      true,
		RechargeMode:                 "TEST",
		RedemptionEnabled:            false,
		RedemptionRateLimit:          rateLimit,
		RedemptionRateWindowSeconds:  window,
		WalletCurrency:               "CNY",
		ProviderCurrency:             "USD",
		CnyPerUsdRate:                "7.200000",
		ExchangeSampleWalletAmount:   total,
		ExchangeSampleProviderAmount: provider,
		TestProfile: &settingrpc.WalletProviderProfileMasked{
			Environment:       "TEST",
			ProfileId:         "wp_abc",
			ProfileRevision:   1,
			ProductId:         "prod_123",
			WebhookId:         "wh_123",
			ApiKeyMasked:      apiKeyMasked,
			ApiKeyConfigured:  configured,
			ApiKeyFingerprint: fingerprint,
			WebhookSecrets: []*settingrpc.WalletWebhookSecretMasked{{
				EndpointId:       "ep_1",
				Fingerprint:      fingerprint,
				SecretMasked:     apiKeyMasked,
				SecretConfigured: true,
				RetireAfter:      1735689600000,
			}},
			VerifiedAt: 1735689600000,
		},
		UpdatedBy: "7",
		UpdatedAt: 1735689600000,
	}
}

func decodeWalletResp(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("响应不是合法 JSON: %v: %s", err, body)
	}
	return decoded
}

// jsonRequest 构造带 JSON Content-Type 的请求体。
func jsonRequest(payload string) (*ut.Body, ut.Header) {
	return &ut.Body{Body: strings.NewReader(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"}
}

func TestGetWalletSettingSuccess(t *testing.T) {
	client := &fakeSettingClient{masked: maskedResponse()}
	h := newWalletTestEngine(t, client, 7)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/setting/wallet", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200", w.Code)
	}

	body := w.Body.String()
	for _, secret := range []string{"creem_key_real", "whsec_real"} {
		if strings.Contains(body, secret) {
			t.Fatalf("响应泄漏了秘密 %q", secret)
		}
	}
	if !strings.Contains(body, `"revision":4`) {
		t.Fatalf("响应缺少 revision: %s", body)
	}
	if !strings.Contains(body, `"2025-01-01T00:00:00.000Z"`) {
		t.Fatalf("时间未按 RFC3339 毫秒输出: %s", body)
	}
}

func TestGetWalletSettingMapsErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"invalid", bizErr(settingsvc.WalletSettingCodeInvalidArgument), http.StatusBadRequest},
		{"not_found", bizErr(settingsvc.WalletSettingCodeNotFound), http.StatusNotFound},
		{"revision_conflict", bizErr(settingsvc.WalletSettingCodeRevisionConflict), http.StatusConflict},
		{"provider_rejected", bizErr(settingsvc.WalletSettingCodeProviderRejected), http.StatusUnprocessableEntity},
		{"unavailable", bizErr(settingsvc.WalletSettingCodeUnavailable), http.StatusServiceUnavailable},
		{"internal", bizErr(settingsvc.WalletSettingCodeInternal), http.StatusInternalServerError},
		{"transport", errors.New("rpc transport broken"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeSettingClient{maskedErr: tc.err}
			h := newWalletTestEngine(t, client, 7)
			w := ut.PerformRequest(h.Engine, http.MethodGet, "/setting/wallet", nil)
			if w.Code != tc.want {
				t.Fatalf("状态码 = %d，期望 %d（body=%s）", w.Code, tc.want, w.Body.String())
			}
			decoded := decodeWalletResp(t, w.Body.Bytes())
			if _, ok := decoded["code"]; !ok {
				t.Fatalf("错误响应缺少 code 字段: %s", w.Body.String())
			}
		})
	}
}

func bizErr(code int32) error {
	return kerrors.NewBizStatusError(code, "business error")
}

func TestUpdateWalletSettingUsesAuthenticatedOperator(t *testing.T) {
	client := &fakeSettingClient{masked: maskedResponse()}
	h := newWalletTestEngine(t, client, 42)

	body, header := jsonRequest(`{
		"expected_revision": 4,
		"enabled": true,
		"recharge_mode": "TEST",
		"cny_per_usd_rate": "7.300000",
		"operator_id": "999",
		"profiles": [{
			"environment": "TEST",
			"product_id": "prod_123",
			"webhook_id": "wh_123",
			"api_key": "creem_key_real",
			"webhook_secrets": [{"secret": "whsec_real", "endpoint_id": "ep_1"}]
		}]
	}`)
	w := ut.PerformRequest(h.Engine, http.MethodPut, "/setting/wallet", body, header)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200（body=%s）", w.Code, w.Body.String())
	}
	if client.lastUpdate == nil {
		t.Fatalf("未调用 UpdateWalletSettings")
	}
	if client.lastUpdate.GetOperatorId() != "42" {
		t.Fatalf("operator_id = %q，期望来自 JWT 的 42", client.lastUpdate.GetOperatorId())
	}
	if client.lastUpdate.GetExpectedRevision() != 4 {
		t.Fatalf("expected_revision = %d", client.lastUpdate.GetExpectedRevision())
	}
	if client.lastUpdate.GetCnyPerUsdRate() != "7.300000" {
		t.Fatalf("cny_per_usd_rate = %q", client.lastUpdate.GetCnyPerUsdRate())
	}
	profiles := client.lastUpdate.GetProfiles()
	if len(profiles) != 1 {
		t.Fatalf("profiles 数量 = %d", len(profiles))
	}
	if profiles[0].GetEnvironment() != "TEST" || profiles[0].GetApiKey() != "creem_key_real" {
		t.Fatalf("Profile 转换不正确: %+v", profiles[0])
	}
	if len(profiles[0].GetWebhookSecrets()) != 1 || profiles[0].GetWebhookSecrets()[0].GetSecret() != "whsec_real" {
		t.Fatalf("Webhook Secret 转换不正确: %+v", profiles[0].GetWebhookSecrets())
	}
}

func TestUpdateWalletSettingRequiresIdentity(t *testing.T) {
	client := &fakeSettingClient{masked: maskedResponse()}
	h := newWalletTestEngine(t, client, 0)

	body, header := jsonRequest(`{"expected_revision":0}`)
	w := ut.PerformRequest(h.Engine, http.MethodPut, "/setting/wallet", body, header)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d，期望 401", w.Code)
	}
	if client.updateCalls != 0 {
		t.Fatalf("未认证请求不得调用服务")
	}
}

func TestUpdateWalletSettingRejectsNegativeLimits(t *testing.T) {
	client := &fakeSettingClient{masked: maskedResponse()}
	h := newWalletTestEngine(t, client, 7)

	body, header := jsonRequest(`{"expected_revision":0,"redemption_rate_limit":-1}`)
	w := ut.PerformRequest(h.Engine, http.MethodPut, "/setting/wallet", body, header)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 400", w.Code)
	}
	if client.updateCalls != 0 {
		t.Fatalf("非法输入不得调用服务")
	}
}

func TestUpdateWalletSettingMapsRevisionConflict(t *testing.T) {
	client := &fakeSettingClient{
		updateErr: bizErr(settingsvc.WalletSettingCodeRevisionConflict),
	}
	h := newWalletTestEngine(t, client, 7)

	body, header := jsonRequest(`{"expected_revision":1}`)
	w := ut.PerformRequest(h.Engine, http.MethodPut, "/setting/wallet", body, header)
	if w.Code != http.StatusConflict {
		t.Fatalf("状态码 = %d，期望 409（body=%s）", w.Code, w.Body.String())
	}
	decoded := decodeWalletResp(t, w.Body.Bytes())
	if fmt.Sprint(decoded["code"]) != fmt.Sprint(float64(settingsvc.WalletSettingCodeRevisionConflict)) {
		t.Fatalf("错误码 = %v", decoded["code"])
	}
}

func TestWalletSettingsToAPIOmitsZeroTimes(t *testing.T) {
	resp := walletSettingsToAPI(&settingrpc.WalletSettingsMasked{Revision: 0})
	if resp.UpdatedAt != nil {
		t.Fatalf("updated_at 为 0 时不应输出时间")
	}
	if resp.GetUpdatedBy() != "" {
		t.Fatalf("updated_by 应为空")
	}
}
