namespace go auth_api.system

struct RegisterByEmail {
  1: string Email (api.query="email")
  2: string VerifyCode (api.query="verify_code")
  3: string Password (api.query="password")
}

struct RegisterByMobile {
  1: string Mobile (api.query="mobile")
  2: string VerifyCode (api.query="verify_code")
  3: string Password (api.query="password")
}

struct LoginByEmailReq {
  1: string Email (api.query="email")
  2: string Password (api.query="password")
  3: string VerifyCode (api.query="verify_code")
}

service HzAuthApi {
  // 注册
  string RegisterByEmail(1: RegisterByEmail req) (api.post="/register/email")
  string RegisterByMobile(1: RegisterByMobile req) (api.post="/register/mobile")
  // 登录
  string LoginByEmail(1: LoginByEmailReq req) (api.post="/login/email")
}