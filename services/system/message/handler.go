package message

import (
	"context"
	"math/rand"
	"strconv"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/ent"
	"github.com/KiClover/orzfrp/ent/migrate"
	"github.com/KiClover/orzfrp/kitex_gen/base"
	"github.com/KiClover/orzfrp/kitex_gen/system/message"
	"github.com/KiClover/orzfrp/kitex_gen/system/setting/systemsettingservice"
	"github.com/sirupsen/logrus"
	"github.com/wneessen/go-mail"
	"go.uber.org/fx"
)

const ServiceName = "system-message"

type SystemMessageService struct {
	fx.In
	Cfg           *conf.Config
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
		logrus.Errorf("init database err: %v", err)
		return nil, err
	}

	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s SystemMessageService) SendEmailCode(ctx context.Context, req *message.SendCodeReq) (res *message.SendCodeResp, err error) {
	setting, err := s.SettingClient.GetEmailProvider(ctx, &base.Empty{})
	if err != nil {
		logrus.Errorf("get email provider err: %v", err)
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
		logrus.Errorf("create mail client err: %v", err)
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
	htmlTpl := GetVerifyCodeTemplate()
	msg := mail.NewMsg()
	if err = msg.FromFormat(*setting.DisplayName, *setting.User); err != nil {
		logrus.Errorf("failed to set sender: %s", err)
		return nil, err
	}
	if err := msg.To(req.To); err != nil {
		logrus.Errorf("failed to set TO address: %s", err)
		return nil, err
	}
	msg.Subject("注册验证码验证")
	if err := msg.AddAlternativeHTMLTemplate(htmlTpl, data); err != nil {
		logrus.Errorf("failed to add HTML template to mail body: %s", err)
		return nil, err
	}
	if err := client.DialAndSend(msg); err != nil {
		logrus.Errorf("failed to deliver mail: %s", err)
		return nil, err
	}
	return &message.SendCodeResp{
		Code: code,
	}, nil
}

func (s SystemMessageService) SendSmsCode(ctx context.Context, req *message.SendCodeReq) (res *message.SendCodeResp, err error) {
	//TODO implement me
	panic("implement me")
}

func generateCode() string {
	randomNumber := rand.Intn(900000) + 100000
	return strconv.Itoa(randomNumber)
}
