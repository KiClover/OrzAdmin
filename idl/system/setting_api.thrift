namespace go system.setting

include "idl/base.thrift"

// 注册默认角色：POST /setting/register/role
struct SetRegUserRoleReq {
  1: optional string role (api.body="role")
}

// 注册默认角色：GET /setting/register/role
struct GetRegUserRoleResp {
  1: optional string role (api.body="role")
}

// 邮件服务配置：POST /setting/email
struct SetEmailSettingReq {
  1: optional string host (api.body="host")
  2: optional i32 port (api.body="port")
  // 1 SSL 2 TLS 3 None
  3: optional i32 crypt_type (api.body="crypt_type")
  4: optional string user (api.body="user")
  5: optional string password (api.body="password")
  6: optional string display_name (api.body="display_name")
  7: optional string from (api.body="from")
}

// 邮件服务配置：GET /setting/email
struct GetEmailSettingResp {
  1: optional string host (api.body="host")
  2: optional i32 port (api.body="port")
  // 1 SSL 2 TLS 3 None
  3: optional i32 crypt_type (api.body="crypt_type")
  4: optional string user (api.body="user")
  5: optional string password (api.body="password")
  6: optional string display_name (api.body="display_name")
  7: optional string from (api.body="from")
}

// 短信服务配置：POST /setting/mobile
struct SetSmsSettingReq {
  // 1 SMSBao
  1: optional i32 type (api.body="type")
  2: optional string access_id (api.body="access_id")
  3: optional string access_key (api.body="access_key")
  4: optional string sign_name (api.body="sign_name")
}

// 短信服务配置：GET /setting/mobile
struct GetSmsSettingResp {
  // 1 SMSBao
  1: optional i32 type (api.body="type")
  2: optional string access_id (api.body="access_id")
  3: optional string access_key (api.body="access_key")
  4: optional string sign_name (api.body="sign_name")
}

// ============================ 钱包设置（doc/plan/WalletDesign.MD §10.3） ============================
// 本文件只承载 System Setting API 独占的 /setting/wallet 配置项。
// 钱包业务接口（/wallet、/admin/*）由 Finance API 独占，不在此文件、也不由本服务代理。

// Webhook 签名 Secret 的掩码视图：只返回配置状态、固定掩码与 SHA-256 指纹，永不返回明文。
struct WalletSettingSecretMasked {
  1: optional string endpoint_id (api.body="endpoint_id")
  2: optional string fingerprint (api.body="fingerprint")
  3: optional string secret_masked (api.body="secret_masked")
  4: optional bool secret_configured (api.body="secret_configured")
  5: optional string disabled_at (api.body="disabled_at")
  6: optional string retire_after (api.body="retire_after")
}

// Provider Profile 的掩码视图。profile_revision 只读，官方换算由服务端维护。
struct WalletSettingProfileMasked {
  // TEST | PRODUCTION
  1: optional string environment (api.body="environment")
  2: optional string profile_id (api.body="profile_id")
  3: optional i64 profile_revision (api.body="profile_revision")
  4: optional string product_id (api.body="product_id")
  5: optional string webhook_id (api.body="webhook_id")
  6: optional string api_key_masked (api.body="api_key_masked")
  7: optional bool api_key_configured (api.body="api_key_configured")
  8: optional string api_key_fingerprint (api.body="api_key_fingerprint")
  9: optional list<WalletSettingSecretMasked> webhook_secrets (api.body="webhook_secrets")
  10: optional string verified_at (api.body="verified_at")
}

// 钱包设置：GET /setting/wallet
struct GetWalletSettingResp {
  1: optional i64 revision (api.body="revision")
  2: optional bool enabled (api.body="enabled")
  // DISABLED | TEST | PRODUCTION
  3: optional string recharge_mode (api.body="recharge_mode")
  4: optional bool redemption_enabled (api.body="redemption_enabled")
  5: optional i32 redemption_rate_limit (api.body="redemption_rate_limit")
  6: optional i32 redemption_rate_window_seconds (api.body="redemption_rate_window_seconds")
  7: optional string wallet_currency (api.body="wallet_currency")
  8: optional string provider_currency (api.body="provider_currency")
  // 规范十进制字符串：1 USD = N CNY，最多 6 位小数。
  9: optional string cny_per_usd_rate (api.body="cny_per_usd_rate")
  10: optional i64 exchange_sample_wallet_amount (api.body="exchange_sample_wallet_amount")
  11: optional i64 exchange_sample_provider_amount (api.body="exchange_sample_provider_amount")
  12: optional WalletSettingProfileMasked test_profile (api.body="test_profile")
  13: optional WalletSettingProfileMasked production_profile (api.body="production_profile")
  14: optional string updated_by (api.body="updated_by")
  15: optional string updated_at (api.body="updated_at")
  16: optional bool stale (api.body="stale")
}

// Webhook Secret 只写补丁：省略表示保留旧值；首期不提供清空能力。
struct WalletSettingSecretPatch {
  1: optional string secret (api.body="secret")
  2: optional string endpoint_id (api.body="endpoint_id")
}

// Provider Profile 补丁：省略字段表示保留旧值，只有提供新值时才轮换 Secret。
struct WalletSettingProfilePatch {
  // TEST | PRODUCTION
  1: optional string environment (api.body="environment")
  2: optional string profile_id (api.body="profile_id")
  3: optional string product_id (api.body="product_id")
  4: optional string webhook_id (api.body="webhook_id")
  5: optional string api_key (api.body="api_key")
  6: optional list<WalletSettingSecretPatch> webhook_secrets (api.body="webhook_secrets")
}

// 钱包设置：PUT /setting/wallet
// 操作人身份来自 JWT，不作为 body 字段接收；expected_revision 用于乐观并发控制。
struct UpdateWalletSettingReq {
  1: optional i64 expected_revision (api.body="expected_revision")
  2: optional bool enabled (api.body="enabled")
  3: optional string recharge_mode (api.body="recharge_mode")
  4: optional bool redemption_enabled (api.body="redemption_enabled")
  5: optional i32 redemption_rate_limit (api.body="redemption_rate_limit")
  6: optional i32 redemption_rate_window_seconds (api.body="redemption_rate_window_seconds")
  7: optional string cny_per_usd_rate (api.body="cny_per_usd_rate")
  8: optional list<WalletSettingProfilePatch> profiles (api.body="profiles")
  // 首期不支持清空已配置的 Secret 或删除 Profile。
}
