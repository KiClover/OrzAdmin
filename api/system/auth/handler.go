package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/KiClover/orzfrp/api/model/auth_api/system"
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/kitex_gen/base"
	"github.com/KiClover/orzfrp/kitex_gen/system/message"
	"github.com/KiClover/orzfrp/kitex_gen/system/message/systemmessageservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzfrp/kitex_gen/system/user"
	"github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	captcha2 "github.com/KiClover/orzfrp/pkg/captcha"
	"github.com/KiClover/orzfrp/pkg/hresp"
	"github.com/KiClover/orzfrp/pkg/jwt"
	"github.com/KiClover/orzfrp/pkg/utils"
	message2 "github.com/KiClover/orzfrp/services/system/message"
	"github.com/KiClover/orzfrp/services/system/setting"
	user2 "github.com/KiClover/orzfrp/services/system/user"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/mojocn/base64Captcha"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

const HzServiceName = "api.system.auth"

type HzSystemAuthServiceP struct {
	fx.In
	UserClient    systemuserservice.Client
	Cfg           *conf.Config
	Casbin        *casbin.Enforcer
	SettingClient systemsettingservice.Client
	Redis         redis.UniversalClient
	MsgClient     systemmessageservice.Client
}
type HzSystemAuthService struct {
	UserClient    systemuserservice.Client
	Cfg           *conf.Config
	Casbin        *casbin.Enforcer
	SettingClient systemsettingservice.Client
	Redis         redis.UniversalClient
	MsgClient     systemmessageservice.Client
}

func NewHzSystemAuthService(p HzSystemAuthServiceP) *HzSystemAuthService {
	return &HzSystemAuthService{
		UserClient:    p.UserClient,
		Cfg:           p.Cfg,
		Casbin:        p.Casbin,
		SettingClient: p.SettingClient,
		Redis:         p.Redis,
		MsgClient:     p.MsgClient,
	}
}

func (s *HzSystemAuthService) InitDatabase(ctx context.Context, c *app.RequestContext) {
	_, err := s.UserClient.InitDatabase(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}
	_, err = s.SettingClient.InitDatabase(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, setting.ServiceName, err)
	}
	_, err = s.MsgClient.InitDatabase(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, message2.ServiceName, err)
	}
	hresp.BaseSuccessMsg(c, "初始化数据库成功！")
	return
}

const captchaPrefix = "captcha:"
const SentPrefix = "auth:sent:"
const CodePrefix = "auth:code:"

func (s *HzSystemAuthService) RegisterByEmail(ctx context.Context, c *app.RequestContext) {
	var req system.RegisterByEmail
	err := c.BindAndValidate(&req)
	if err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	// 检查是否已注册
	exist, err := s.UserClient.GetUserIsExist(ctx, &user.GetUserIsExistReq{
		Email: utils.GetPointer(req.Email),
	})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}
	if exist.IsExist {
		hresp.BaseErrMsg(c, 3, "该邮箱已注册", err)
		return
	}

	// 校验验证码
	key := fmt.Sprintf("%s%s", CodePrefix, req.Email)
	storedCode, err := s.Redis.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		hresp.BaseErrMsg(c, 3, "验证码已过期或不存在", nil)
		return
	}
	if err != nil {
		hresp.BaseErrMsg(c, 4, "获取验证码失败", err)
		return
	}
	if storedCode != req.Code {
		hresp.BaseErrMsg(c, 4, "验证码错误", nil)
		return
	}
	defer s.Redis.Del(ctx, key)

	gr, err := s.SettingClient.GetRegUserRole(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, setting.ServiceName, err)
		return
	}

	uc, err := s.UserClient.CreateUser(ctx, &user.UserInfo{
		Email:    &req.Email,
		Password: &req.Password,
		Name:     &req.UserName,
	})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}

	_, err = s.Casbin.AddRoleForUser(strconv.FormatInt(uc.Id, 10), gr.Role)
	if err != nil {
		hresp.BaseErrMsg(c, 4, "Casbin处理用户角色失败", err)
		return
	}
	hresp.BaseSuccessMsg(c, fmt.Sprintf("用户 %s 注册成功！", req.UserName))
	return
}

func (s *HzSystemAuthService) RegisterByMobile(ctx context.Context, c *app.RequestContext) {
	var req system.RegisterByMobile
	err := c.BindAndValidate(&req)
	if err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	// 检查是否已注册
	exist, err := s.UserClient.GetUserIsExist(ctx, &user.GetUserIsExistReq{
		Mobile: utils.GetPointer(req.Mobile),
	})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}
	if exist.IsExist {
		hresp.BaseErrMsg(c, 3, "该手机号已注册", err)
		return
	}

	// 校验验证码
	key := fmt.Sprintf("%s%s", CodePrefix, req.Mobile)
	storedCode, err := s.Redis.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		hresp.BaseErrMsg(c, 3, "验证码已过期或不存在", nil)
		return
	}
	if err != nil {
		hresp.BaseErrMsg(c, 4, "获取验证码失败", err)
		return
	}
	if storedCode != req.Code {
		hresp.BaseErrMsg(c, 4, "验证码错误", nil)
		return
	}
	defer s.Redis.Del(ctx, key)

	gr, err := s.SettingClient.GetRegUserRole(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, setting.ServiceName, err)
		return
	}

	uc, err := s.UserClient.CreateUser(ctx, &user.UserInfo{
		Mobile:   &req.Mobile,
		Password: &req.Password,
		Name:     &req.UserName,
	})
	if err != nil {
		hresp.KitexCallErr(c, user2.ServiceName, err)
		return
	}

	_, err = s.Casbin.AddRoleForUser(strconv.FormatInt(uc.Id, 10), gr.Role)
	if err != nil {
		hresp.BaseErrMsg(c, 4, "Casbin处理用户角色失败", err)
		return
	}
	hresp.BaseSuccessMsg(c, fmt.Sprintf("用户 %s 注册成功！", req.UserName))
	return
}

func (s *HzSystemAuthService) LoginByPwd(ctx context.Context, c *app.RequestContext) {
	var req system.LoginByPwdReq
	err := c.BindAndValidate(&req)
	if err != nil {
		hresp.ReqBindErr(c, err)
		return
	}
	var userInfo *user.UserInfo

	if req.Type == 1 {
		e, err := s.UserClient.GetUserIsExist(ctx,
			&user.GetUserIsExistReq{
				Email: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
		if !e.IsExist {
			hresp.BaseErrMsg(c, 3, "该邮箱未注册", nil)
			return
		}
		userInfo, err = s.UserClient.GetUserByEmail(ctx,
			&user.GetUserByEmailReq{
				Email: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
	} else if req.Type == 2 {
		m, err := s.UserClient.GetUserIsExist(ctx,
			&user.GetUserIsExistReq{
				Mobile: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
		if !m.IsExist {
			hresp.BaseErrMsg(c, 3, "该手机号未注册", nil)
			return
		}
		userInfo, err = s.UserClient.GetUserByMobile(ctx,
			&user.GetUserByMobileReq{
				Mobile: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
	} else {
		hresp.BaseErrMsg(c, 3, "登录方式错误", nil)
		return
	}

	v := utils.CheckPasswordHash(req.Password, *userInfo.Password)
	if v != nil {
		hresp.BaseErrMsg(c, 3, "密码错误", nil)
		return
	}

	// 签发JWT Token
	token, err := jwt.NewJwtToken(s.Cfg.AuthConf.AccessSecret, time.Now().Unix(), s.Cfg.AuthConf.AccessExpire,
		jwt.WithOption("userId", userInfo.Id),
	)
	if err != nil {
		hresp.BaseErrMsg(c, 4, "token签发失败", err)
		return
	}
	hresp.BaseSuccessMsgWithBody(c, "登录成功！", system.LoginResp{
		Token:        token,
		RefreshToken: "",
	})
	return
}

func (s *HzSystemAuthService) LoginByCode(ctx context.Context, c *app.RequestContext) {
	var req system.LoginByCodeReq
	err := c.BindAndValidate(&req)
	if err != nil {
		hresp.ReqBindErr(c, err)
		return
	}

	// 校验验证码
	key := fmt.Sprintf("%s%s", CodePrefix, req.Account)
	storedCode, err := s.Redis.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		hresp.BaseErrMsg(c, 3, "验证码已过期或不存在", nil)
		return
	}
	if err != nil {
		hresp.BaseErrMsg(c, 4, "获取验证码失败", err)
		return
	}
	if storedCode != req.Code {
		hresp.BaseErrMsg(c, 4, "验证码错误", nil)
		return
	}
	defer s.Redis.Del(ctx, key)

	var userInfo *user.UserInfo
	if req.Type == 1 {
		e, err := s.UserClient.GetUserIsExist(ctx,
			&user.GetUserIsExistReq{
				Email: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
		if !e.IsExist {
			hresp.BaseErrMsg(c, 3, "该邮箱未注册", nil)
			return
		}
		userInfo, err = s.UserClient.GetUserByEmail(ctx,
			&user.GetUserByEmailReq{
				Email: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
	} else if req.Type == 2 {
		m, err := s.UserClient.GetUserIsExist(ctx,
			&user.GetUserIsExistReq{
				Mobile: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
		if !m.IsExist {
			hresp.BaseErrMsg(c, 3, "该手机号未注册", nil)
			return
		}
		userInfo, err = s.UserClient.GetUserByMobile(ctx,
			&user.GetUserByMobileReq{
				Mobile: utils.GetPointer(req.Account),
			})
		if err != nil {
			hresp.KitexCallErr(c, user2.ServiceName, err)
			return
		}
	} else {
		hresp.BaseErrMsg(c, 3, "登录方式错误", nil)
		return
	}

	// 签发JWT Token
	token, err := jwt.NewJwtToken(s.Cfg.AuthConf.AccessSecret, time.Now().Unix(), s.Cfg.AuthConf.AccessExpire,
		jwt.WithOption("userId", userInfo.Id),
	)
	if err != nil {
		hresp.BaseErrMsg(c, 4, "token签发失败", err)
		return
	}
	hresp.BaseSuccessMsgWithBody(c, "登录成功！", system.LoginResp{
		Token:        token,
		RefreshToken: "",
	})
	return
}

func (s *HzSystemAuthService) GetVerifyCode(ctx context.Context, c *app.RequestContext) {
	var req system.GetVerifyCodeReq
	err := c.BindAndValidate(&req)
	if err != nil {
		hresp.ReqBindErr(c, err)
		return
	}

	// Captcha验证码校验
	store := captcha2.NewRedisStore(s.Redis, captchaPrefix)
	captcha := base64Captcha.NewCaptcha(&base64Captcha.DriverString{}, store)
	if !captcha.Verify(req.CaptchaId, req.Captcha, true) {
		hresp.BaseErrMsg(c, 3, "验证码错误", nil)
		return
	}

	// 验证是否发送频繁
	isSent := s.Redis.Get(ctx, fmt.Sprintf("%s%s", SentPrefix, req.Account))
	if !errors.Is(isSent.Err(), redis.Nil) {
		hresp.BaseErrMsg(c, 3, "验证码已发送，请勿频繁发送", nil)
		return
	}
	if isSent.Err() != nil && !errors.Is(isSent.Err(), redis.Nil) {
		hresp.BaseErrMsg(c, 4, "获取验证码失败", err)
		return
	}

	// 发送验证码
	var r *message.SendCodeResp
	switch req.Type {
	case 1:
		r, err = s.MsgClient.SendEmailCode(ctx, &message.SendCodeReq{
			To: req.Account,
		})
		if err != nil {
			hresp.KitexCallErr(c, message2.ServiceName, err)
			return
		}
	case 2:
		r, err = s.MsgClient.SendSmsCode(ctx, &message.SendCodeReq{
			To: req.Account,
		})
		if err != nil {
			hresp.KitexCallErr(c, message2.ServiceName, err)
			return
		}
	default:
		hresp.BaseErrMsg(c, 3, "验证码类型错误", errors.New("captcha type error"))
		return
	}

	// 存储验证码和已发送计时的Store
	authCodeStore := s.Redis.Set(ctx, fmt.Sprintf("%s%s", CodePrefix, req.Account), r.Code, 15*time.Minute)
	if authCodeStore.Err() != nil {
		hresp.BaseErrMsg(c, 4, "写入Redis错误", authCodeStore.Err())
		return
	}
	sentStore := s.Redis.Set(ctx, fmt.Sprintf("%s%s", SentPrefix, req.Account), 1, 1*time.Minute)
	if sentStore.Err() != nil {
		hresp.BaseErrMsg(c, 4, "写入Redis错误", sentStore.Err())
		return
	}

	var codeType string
	if req.Type == 1 {
		codeType = "邮箱"
	} else {
		codeType = "短信"
	}
	hresp.BaseSuccessMsg(c, fmt.Sprintf("%s验证码已发送", codeType))
	return
}

func (s *HzSystemAuthService) GetCaptcha(ctx context.Context, c *app.RequestContext) {
	var driver = base64Captcha.DriverString{
		Height:          60,
		Width:           240,
		NoiseCount:      1,
		ShowLineOptions: 1,
		Length:          4,
		Source:          "0123456789qazwsxedcrfvtgbyhnujmikolpABCDEFGHJKLMNPQRSTUVWXYZ",
	}
	store := captcha2.NewRedisStore(s.Redis, captchaPrefix)
	captcha := base64Captcha.NewCaptcha(&driver, store)
	id, b64s, _, err := captcha.Generate()
	if err != nil {
		hresp.BaseErrMsg(c, 4, "获取验证码失败", err)
		return
	}
	hresp.BaseSuccessMsgWithBody(c, "获取验证码成功！", system.GetCaptchaResp{
		CaptchaId: id,
		Img:       b64s,
	})
	return
}
