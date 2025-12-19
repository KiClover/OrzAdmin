namespace go auth_api.system

include "idl/base.thrift"

struct RegisterByEmail {
  1: string Email (api.body="email")
  2: string Code (api.body="code")
  3: string Password (api.body="password")
  4: string UserName (api.body="user_name")
}

struct RegisterByMobile {
  1: string Mobile (api.body="mobile")
  2: string Code (api.body="code")
  3: string Password (api.body="password")
  4: string UserName (api.body="user_name")
}

struct LoginByCodeReq {
  1: i32 Type (api.body="type")
  2: string Account (api.body="account")
  3: string Code (api.body="code")
}

struct LoginByPwdReq {
  1: i32 Type (api.body="type")
  2: string Account (api.body="account")
  3: string Password (api.body="password")
}

struct LoginResp  {
  1: string Token
  2: string RefreshToken
}

struct GetVerifyCodeReq {
  1: i32 Type (api.body="type")
  2: string Account (api.body="account")
  3: string CaptchaId (api.body="captcha_id")
  4: string Captcha (api.body="captcha")
}

struct GetCaptchaResp {
  1: string CaptchaId
  2: string Img
}

service HzAuthApi {
}