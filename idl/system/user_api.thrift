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
  10: optional i64 id (api.body="id")
}

struct UpdateUserProfileReq {
  1: optional i32 status (api.body="status"),
  2: optional string name (api.body="name"),
  3: optional string description (api.body="description"),
  4: optional string mobile (api.body="mobile"),
  5: optional string email (api.body="email"),
  6: optional string avatar (api.body="avatar"),
  7: optional i32 gender (api.body="gender"),
}

struct GetUserListReq {
  1: optional i32 page (api.body="page")
  2: optional i32 page_size (api.body="page_size")
  3: optional string name (api.body="name")
  4: optional string mobile (api.body="mobile")
  5: optional string email (api.body="email")
  6: optional i32 status (api.body="status")
}

struct GetUserProfileResp {
  1: optional i64 id (api.body="id")
  2: optional i32 status (api.body="status"),
  3: optional string name (api.body="name"),
  6: optional string description (api.body="description"),
  7: optional string mobile (api.body="mobile"),
  8: optional string email (api.body="email"),
  9: optional string avatar (api.body="avatar"),
  10: optional list<string> role_name (api.body="role_names"),
  11: optional i32 gender (api.body="gender"),
}

struct GetUserById {
  1: i64 id (api.path="id")
}

struct GetUserListResp {
  1: i64 total (api.body="total")
  2: list<UserInfo> data (api.body="list")
}

struct ChangeUserPwdReq {
  1: optional string old_password (api.body="old_password")
  2: optional string new_password (api.body="new_password")
}

struct UserBasicInfo {
  1: optional i64 id (api.body="id")
  2: optional string name (api.body="name")
  3: optional string avatar (api.body="avatar")
  4: optional string description (api.body="description")
}