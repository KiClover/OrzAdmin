package message

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/migrate"
	internallog "github.com/KiClover/orzAdmin/internal/log"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/message"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting/systemsettingservice"
	"github.com/KiClover/orzAdmin/pkg/kerr"
	"github.com/go-resty/resty/v2"
	"github.com/wneessen/go-mail"
	"go.uber.org/fx"
)

const ServiceName = "system-message"

// logger 让 message 服务的日志统一带 [MESSAGE] 前缀。
var logger = internallog.WithModule(internallog.ModuleMessage)

type SystemMessageService struct {
	fx.In
	Cfg           *conf.Config
	DB            *db.MessageServiceDB
	DBAll         *ent.Client
	SettingClient systemsettingservice.Client
}

var _ message.SystemMessageService = (*SystemMessageService)(nil)

func NewSystemMessageService(svc SystemMessageService) message.SystemMessageService {
	return &svc
}

func (s SystemMessageService) InitDatabase(ctx context.Context, req *base.Empty) (res *base.BaseResp, err error) {
	tables := []*schema.Table{migrate.MsgsTable}
	if err := migrate.Create(ctx, s.DBAll.Schema, tables, schema.WithForeignKeys(false)); err != nil {
		logger.Errorf("init database err: %v", err)
		return nil, err
	}

	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemMessageService) SendEmailCode(ctx context.Context, req *message.SendCodeReq) (res *message.SendCodeResp, err error) {
	setting, err := s.SettingClient.GetEmailProvider(ctx, &base.Empty{})
	if err != nil {
		logger.Errorf("get email provider err: %v", err)
		return nil, err
	}

	var client *mail.Client
	switch *setting.CryptType {
	case 1: // SSL
		client, err = mail.NewClient(*setting.Host,
			mail.WithPort(int(*setting.Port)),
			mail.WithSSL(),
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(*setting.User),
			mail.WithPassword(*setting.Password),
		)
	case 2: // TLS
		client, err = mail.NewClient(*setting.Host,
			mail.WithPort(int(*setting.Port)),
			mail.WithTLSPortPolicy(mail.TLSMandatory),
			mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
			mail.WithUsername(*setting.User),
			mail.WithPassword(*setting.Password),
		)
	default: // No encryption
		client, err = mail.NewClient(*setting.Host,
			mail.WithPort(int(*setting.Port)),
			mail.WithTLSPortPolicy(mail.NoTLS),
			mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
			mail.WithUsername(*setting.User),
			mail.WithPassword(*setting.Password),
		)
	}
	if err != nil {
		logger.Errorf("create mail client err: %v", err)
		return nil, err
	}

	// 模板写入
	type CodeTemplateData struct {
		Code     string
		SignName string
	}
	code := generateCode()
	data := CodeTemplateData{
		Code:     code,
		SignName: *setting.DisplayName,
	}
	htmlTpl, err := GetVerifyCodeTemplate()
	if err != nil {
		return nil, err
	}
	msg := mail.NewMsg()
	if err = msg.FromFormat(*setting.DisplayName, *setting.From); err != nil {
		logger.Errorf("failed to set sender: %s", err)
		return nil, err
	}
	if err := msg.To(req.To); err != nil {
		logger.Errorf("failed to set TO address: %s", err)
		return nil, err
	}
	msg.Subject("注册验证码验证")
	if err := msg.AddAlternativeHTMLTemplate(htmlTpl, data); err != nil {
		logger.Errorf("failed to add HTML template to mail body: %s", err)
		return nil, err
	}
	if err := client.DialAndSend(msg); err != nil {
		logger.Errorf("failed to deliver mail: %s", err)
		_, err = s.DB.Msg.Create().
			SetType(1).SetTo(req.To).
			SetContent(fmt.Sprintf("验证码：%s", code)).
			SetSendStatus(2).
			Save(ctx)
		if err != nil {
			kerr.EntDefaultErr(err)
			return nil, err
		}
		return nil, err

	}
	_, err = s.DB.Msg.Create().
		SetType(1).SetTo(req.To).
		SetContent(fmt.Sprintf("验证码：%s", code)).
		SetSendStatus(1).
		Save(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &message.SendCodeResp{
		Code: code,
	}, nil
}

func (s SystemMessageService) SendSmsCode(ctx context.Context, req *message.SendCodeReq) (res *message.SendCodeResp, err error) {
	setting, err := s.SettingClient.GetSmsProvider(ctx, &base.Empty{})
	if err != nil {
		logger.Errorf("get sms provider err: %v", err)
		return nil, err
	}
	code := generateCode()
	switch *setting.Type {
	// SMSBao
	case 1:
		const smsbao = "https://api.smsbao.com/sms"

		m := "【" + *setting.SignName + "】您的验证码为：" + code + "，请尽快完成验证。"
		client := resty.New()
		resp, err := client.R().
			SetQueryParam("u", *setting.AccessId).
			SetQueryParam("p", *setting.AccessKey).
			SetQueryParam("m", req.To).
			SetQueryParam("c", m).
			Get(smsbao)
		if err != nil || string(resp.Body()) != "0" {
			logger.Errorf("send sms err: %v", err)
			_, err = s.DB.Msg.Create().
				SetType(2).SetTo(req.To).
				SetContent(fmt.Sprintf("验证码：%s", code)).
				SetSendStatus(2).
				Save(ctx)
			if err != nil {
				kerr.EntDefaultErr(err)
				return nil, err
			}
		}
	}
	_, err = s.DB.Msg.Create().
		SetType(2).SetTo(req.To).
		SetContent(fmt.Sprintf("验证码：%s", code)).
		SetSendStatus(1).
		Save(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &message.SendCodeResp{
		Code: code,
	}, nil
}

func generateCode() string {
	randomNumber := rand.Intn(900000) + 100000
	return strconv.Itoa(randomNumber)
}
