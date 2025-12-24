namespace go user_api.system

include "idl/base.thrift"

struct UserInfo {
  1: optional i64 id,
  2: optional i64 created_at,
  3: optional i64 updated_at,
  4: optional i32 status,
  5: optional string name,
  6: optional string password,
  7: optional string description,
  8: optional string mobile,
  9: optional string email,
  10: optional string avatar,
  11: optional list<string> role_code,
  12: optional list<string> role_name,
  13: optional i32 gender,
}