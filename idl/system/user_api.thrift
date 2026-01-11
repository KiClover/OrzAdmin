namespace go system.user

include "idl/base.thrift"

struct UserInfo {
  1: optional i64 id (api.body="id")
  2: optional i64 created_at (api.body="created_at")
  3: optional i64 updated_at (api.body="updated_at"),
  4: optional i32 status (api.body="status"),
  5: optional string name (api.body="name"),
  6: optional string password (api.body="password"),
  7: optional string description (api.body="description"),
  8: optional string mobile (api.body="mobile"),
  9: optional string email (api.body="email"),
  10: optional string avatar (api.body="avatar"),
  11: optional list<string> role_code (api.body="role_codes"),
  12: optional list<string> role_name (api.body="role_names"),
  13: optional i32 gender (api.body="gender"),
}

struct UserInfoReq {
  1: optional i32 status (api.body="status"),
  2: optional string name (api.body="name"),
  3: optional string password (api.body="password"),
  4: optional string description (api.body="description"),
  5: optional string mobile (api.body="mobile"),
  6: optional string email (api.body="email"),
  7: optional string avatar (api.body="avatar"),
  8: optional i32 gender (api.body="gender"),
  9: optional list<string> role_codes (api.body="role_codes")
}