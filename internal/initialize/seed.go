// Package initialize 提供钱包任务 2 起的统一初始化入口：按服务同步 Ent 表结构并
// 幂等补齐种子数据（路由、角色、默认策略、设置、默认管理员）。
//
// 设计约束：
//   - 只依赖配置、Ent、Casbin 等底层组件，不 import 业务 API 或业务服务包，避免循环依赖；
//   - 不提供任意 SQL、表名、数据库名或 DSN 入参，服务名走固定注册表；
//   - 所有种子只做“缺失补齐”，已存在的记录保持原值（不重置密码、不重建授权）。
package initialize

import (
	"encoding/json"
)

// APISeed 描述一条注册到 access 服务的 HTTP 路由。
type APISeed struct {
	Path        string
	Method      string
	Group       string
	Description string
}

// RoleSeed 描述一个默认角色。
type RoleSeed struct {
	Code   string
	Name   string
	Remark string
}

// SettingSeed 描述一条 setting 表的默认键值。
type SettingSeed struct {
	Key   string
	Value string
}

// AdminSeed 描述默认管理员账号。密码在构建期由占位符替换成真实值。
type AdminSeed struct {
	Name        string
	Password    string
	Description string
	Mobile      string
	Email       string
	Status      uint
	Gender      uint
}

// AdminName 是默认管理员用户名，初始化与登录都依赖这个稳定值。
const AdminName = "admin"

// AdminPassword 是默认管理员密码占位符，构建期会被替换成真实值。
const AdminPassword = "password"

// EditorPolicy 是 editor 角色的既有默认授权，保持与历史种子一致。
var EditorPolicy = [][2]string{
	{"/web/create", "POST"},
	{"/web/delete", "POST"},
}

// APISeeds 返回需要注册的路由。已存在的 (path, method) 不会被覆盖。
func APISeeds() []APISeed {
	return []APISeed{
		// 用户管理
		{Path: "/user/get/*", Method: "POST", Group: "user", Description: "user get"},
		{Path: "/user/create", Method: "POST", Group: "user", Description: "create user"},
		{Path: "/user/update", Method: "POST", Group: "user", Description: "update user"},
		{Path: "/user/delete", Method: "POST", Group: "user", Description: "delete user"},
		{Path: "/user/list", Method: "POST", Group: "user", Description: "user list"},

		// API 管理
		{Path: "/api/create", Method: "POST", Group: "permission", Description: "create api"},
		{Path: "/api/update", Method: "POST", Group: "permission", Description: "update api"},
		{Path: "/api/delete", Method: "POST", Group: "permission", Description: "delete api"},
		{Path: "/api/list", Method: "POST", Group: "permission", Description: "api list"},

		// 角色映射管理
		{Path: "/role/create", Method: "POST", Group: "permission", Description: "create role"},
		{Path: "/role/update", Method: "PUT", Group: "permission", Description: "update role"},
		{Path: "/role/delete", Method: "DELETE", Group: "permission", Description: "delete role"},
		{Path: "/role/list", Method: "POST", Group: "permission", Description: "role list"},

		// Casbin 管理
		{Path: "/casbin/role/update", Method: "POST", Group: "permission", Description: "update role casbin"},
		{Path: "/casbin/user/update", Method: "POST", Group: "permission", Description: "update user casbin"},
		{Path: "/casbin/role/get", Method: "POST", Group: "permission", Description: "get role casbin"},
		{Path: "/casbin/user/roles/get", Method: "POST", Group: "permission", Description: "get user roles"},
		{Path: "/casbin/role/users/get", Method: "POST", Group: "permission", Description: "get role users"},
		{Path: "/casbin/user/permissions/get", Method: "POST", Group: "permission", Description: "get user permissions"},

		// 钱包设置（任务 2 冻结；钱包管理员路由由任务 9 追加）
		{Path: "/setting/wallet", Method: "GET", Group: "setting", Description: "get wallet settings"},
		{Path: "/setting/wallet", Method: "PUT", Group: "setting", Description: "update wallet settings"},
	}
}

// RoleSeeds 返回默认角色。
func RoleSeeds() []RoleSeed {
	return []RoleSeed{
		{Code: "admin", Name: "管理员", Remark: "最高权限，勿动"},
		{Code: "user", Name: "普通用户", Remark: "默认普通用户"},
	}
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

// AdminUserSeed 返回默认管理员账号。
func AdminUserSeed() AdminSeed {
	return AdminSeed{
		Name:        AdminName,
		Password:    AdminPassword,
		Description: "administrator",
		Mobile:      "10000000000",
		Email:       "kiclover@email.cn",
		Status:      0,
		Gender:      0,
	}
}

// DefaultPolicies 返回默认 Casbin 策略：admin 拥有全部注册路由，editor 仅保留历史授权。
func DefaultPolicies() [][3]string {
	apis := APISeeds()
	policies := make([][3]string, 0, len(apis)+len(EditorPolicy))
	for _, api := range apis {
		policies = append(policies, [3]string{"admin", api.Path, api.Method})
	}
	for _, p := range EditorPolicy {
		policies = append(policies, [3]string{"editor", p[0], p[1]})
	}
	return policies
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
		panic("initialize: marshal wallet default config: " + err.Error())
	}
	return b
}
