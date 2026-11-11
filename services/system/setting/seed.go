package setting

import (
	"context"
	"encoding/json"

	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	settingEnt "github.com/KiClover/orzAdmin/ent/setting"
)

// SettingSeed 描述一条 setting 表的默认键值。
type SettingSeed struct {
	Key   string
	Value string
}

// SettingSeeds 返回默认设置项。钱包配置的默认值只在此处定义一次，
// Setting 服务的严格解码测试会直接解码 WalletDefaultConfigJSON 校验一致性。
func SettingSeeds() []SettingSeed {
	return []SettingSeed{
		{Key: "reg_user_role", Value: "user"},

		// Email 配置
		{Key: "email_host", Value: ""},
		{Key: "email_port", Value: ""},
		{Key: "email_user", Value: ""},
		{Key: "email_from", Value: ""},
		{Key: "email_password", Value: ""},
		{Key: "email_display_name", Value: ""},
		{Key: "email_crypt_type", Value: ""}, // 1 SSL 2 TLS 3 None

		// SMS 配置
		{Key: "sms_type", Value: ""}, // 1 SMSBao
		{Key: "sms_access_id", Value: ""},
		{Key: "sms_access_key", Value: ""},
		{Key: "sms_sign_name", Value: ""},

		// 钱包配置（关闭态默认快照）
		{Key: "wallet_config", Value: string(WalletDefaultConfigJSON())},
	}
}

// seedSettings 幂等补齐默认设置项：已存在的 key 保持原值，不覆盖管理员修改过的内容。
func seedSettings(ctx context.Context, database *db.SettingServiceDB) error {
	for _, seed := range SettingSeeds() {
		exists, err := database.Setting.Query().Where(settingEnt.Key(seed.Key)).Exist(ctx)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		_, err = database.Setting.Create().SetKey(seed.Key).SetValue(seed.Value).Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				// 并发初始化时另一个请求已经写入，视为完成。
				continue
			}
			return err
		}
	}
	return nil
}

// walletDefaultConfig 是 wallet_config 的关闭态默认快照。
// 字段顺序固定，便于测试与人工比对；不得包含任何 Secret。
type walletDefaultConfig struct {
	Revision                    int64                            `json:"revision"`
	WalletCurrency              string                           `json:"wallet_currency"`
	ProviderCurrency            string                           `json:"provider_currency"`
	CNYPerUSDRate               string                           `json:"cny_per_usd_rate"`
	Enabled                     bool                             `json:"enabled"`
	RechargeMode                string                           `json:"recharge_mode"`
	RedemptionEnabled           bool                             `json:"redemption_enabled"`
	RedemptionRateLimit         uint32                           `json:"redemption_rate_limit"`
	RedemptionRateWindowSeconds uint32                           `json:"redemption_rate_window_seconds"`
	Profiles                    map[string]*walletDefaultProfile `json:"profiles"`
	UpdatedBy                   string                           `json:"updated_by"`
	UpdatedAt                   int64                            `json:"updated_at"`
}

type walletDefaultProfile struct {
	ProfileID       string                       `json:"profile_id"`
	ProfileRevision int64                        `json:"profile_revision"`
	APIKey          string                       `json:"api_key"`
	ProductID       string                       `json:"product_id"`
	WebhookID       string                       `json:"webhook_id"`
	WebhookSecrets  []walletDefaultWebhookSecret `json:"webhook_secrets"`
}

type walletDefaultWebhookSecret struct {
	Fingerprint string `json:"fingerprint"`
	Secret      string `json:"secret"`
	EndpointID  string `json:"endpoint_id"`
	DisabledAt  int64  `json:"disabled_at"`
	RetireAfter int64  `json:"retire_after"`
}

// WalletDefaultConfigJSON 返回钱包配置的关闭态默认快照（JSON）。
// 调用方不得修改返回的切片。
func WalletDefaultConfigJSON() []byte {
	cfg := walletDefaultConfig{
		Revision:                    0,
		WalletCurrency:              "CNY",
		ProviderCurrency:            "USD",
		CNYPerUSDRate:               "7.200000",
		Enabled:                     false,
		RechargeMode:                "DISABLED",
		RedemptionEnabled:           false,
		RedemptionRateLimit:         3,
		RedemptionRateWindowSeconds: 600,
		Profiles:                    map[string]*walletDefaultProfile{},
		UpdatedBy:                   "initialize",
		UpdatedAt:                   0,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		// 固定结构体的序列化不应失败；失败说明代码被改坏，直接 panic 早暴露。
		panic("setting: marshal wallet default config: " + err.Error())
	}
	return b
}
