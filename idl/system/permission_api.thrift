namespace go system.permission

include "idl/base.thrift"

struct ApiInfo {
  1: optional i64 id (api.body="id")
  2: optional string path (api.body="path")
  3: optional string description (api.body="description")
  4: optional string group (api.body="group")
  5: optional string method (api.body="method")
}


struct RoleInfo {
  1: optional i64 id (api.body="id")
  2: optional string role_code (api.body="role_code")
  3: optional string role_name (api.body="role_name")
  4: optional string remark (api.body="remark")
}


struct CasbinRule {
  1: optional string method (api.body="method")
  2: optional string path (api.body="path")
}

struct UpdateCasbinReq {
  1: optional string role_code (api.body="role_code")
  2: optional list<CasbinRule> rules (api.body="rules")
}