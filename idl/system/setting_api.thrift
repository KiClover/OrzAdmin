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
