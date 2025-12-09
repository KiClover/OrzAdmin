namespace go auth_api.system

include "idl/base.thrift"

struct RegisterByEmail {
  1: string Email (api.body="email")
  2: string VerifyCode (api.body="verify_code")
  3: string Password (api.body="password")
  4: string UserName (api.body="user_name")
}

struct RegisterByMobile {
  1: string Mobile (api.body="mobile")
  2: string VerifyCode (api.body="verify_code")
  3: string Password (api.body="password")
  4: string UserName (api.body="user_name")
}

struct LoginByEmailReq {
  1: string Email (api.body="email")
  2: string Password (api.body="password")
  3: string VerifyCode (api.body="verify_code")
}

struct LoginResp  {
  1: string Token
  2: string RefreshToken
}

service HzAuthApi {
  // 注册
  string RegisterByEmail(1: RegisterByEmail req) (api.post="/register/email")
  string RegisterByMobile(1: RegisterByMobile req) (api.post="/register/mobile")
  // 登录
  string LoginByEmail(1: LoginByEmailReq req) (api.post="/login/email")
}