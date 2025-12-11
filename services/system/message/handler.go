package message

import (
	"context"

	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/ent"
	"github.com/KiClover/orzfrp/kitex_gen/base"
	"github.com/KiClover/orzfrp/kitex_gen/system/message"
	"go.uber.org/fx"
)

const ServiceName = "system-message"

type SystemMessageService struct {
	fx.In
	Cfg   *conf.Config
	DBAll *ent.Client
}

var _ message.SystemMessageService = (*SystemMessageService)(nil)

func NewSystemMessageService(svc SystemMessageService) message.SystemMessageService {
	return &svc
}

func (s SystemMessageService) InitDatabase(ctx context.Context, req *base.Empty) (res *base.BaseResp, err error) {
	//TODO implement me
	panic("implement me")
}

func (s SystemMessageService) CreateEmailProvider(ctx context.Context, req *message.EmailProviderInfo) (res *base.BaseResp, err error) {
	//TODO implement me
	panic("implement me")
}

func (s SystemMessageService) CreateSmsProvider(ctx context.Context, req *message.SmsProviderInfo) (res *base.BaseResp, err error) {
	//TODO implement me
	panic("implement me")
}
