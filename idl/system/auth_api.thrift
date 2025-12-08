namespace go auth_api.system

include "idl/base.thrift"

struct RegisterByEmail {
  1: string Email (api.query="email")
  2: string VerifyCode (api.query="verify_code")
  3: string Password (api.query="password")
  4: string UserName (api.query="user_name")
}

struct RegisterByMobile {
  1: string Mobile (api.query="mobile")
  2: string VerifyCode (api.query="verify_code")
  3: string Password (api.query="password")
  4: string UserName (api.query="user_name")
}

struct LoginByEmailReq {
  1: string Email (api.query="email")
  2: string Password (api.query="password")
  3: string VerifyCode (api.query="verify_code")
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