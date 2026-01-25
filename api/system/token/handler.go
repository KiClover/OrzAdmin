package token

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/token/systemtokenservice"
	"github.com/KiClover/orzAdmin/pkg/hresp"
	tokenSvc "github.com/KiClover/orzAdmin/services/system/token"
	"github.com/cloudwego/hertz/pkg/app"
	"go.uber.org/fx"
)

const ServiceName = "api.system.token"

type HzSystemTokenServiceP struct {
	fx.In
	Client systemtokenservice.Client
	Cfg    *conf.Config
}

type HzSystemTokenService struct {
	Client systemtokenservice.Client
	Cfg    *conf.Config
}

func NewHzSystemTokenService(p HzSystemTokenServiceP) *HzSystemTokenService {
	return &HzSystemTokenService{
		Client: p.Client,
		Cfg:    p.Cfg,
	}
}

type TokenResp struct {
	Token string `json:"token"`
}

func getUserIDInt64(c *app.RequestContext) (int64, error) {
	v, ok := c.Get("userId")
	if !ok {
		return 0, errors.New("get user id failed")
	}
	switch t := v.(type) {
	case int64:
		return t, nil
	case int:
		return int64(t), nil
	case uint64:
		return int64(t), nil
	case float64:
		return int64(t), nil
	case json.Number:
		id, err := t.Int64()
		if err != nil {
			return 0, err
		}
		return id, nil
	default:
		return 0, errors.New("invalid user id type")
	}
}

func (s *HzSystemTokenService) GetToken(ctx context.Context, c *app.RequestContext) {
	userID, err := getUserIDInt64(c)
	if err != nil {
		hresp.BaseErrMsg(c, 1, err.Error(), nil)
		return
	}
	res, err := s.Client.GetToken(ctx, &base.IDInt64Req{Id: userID})
	if err != nil {
		hresp.KitexCallErr(c, tokenSvc.ServiceName, err)
		return
	}
	var tokenStr string
	if res != nil && res.Token != nil {
		tokenStr = *res.Token
	}
	hresp.BaseSuccessMsgWithBody(c, "success", TokenResp{Token: tokenStr})
}

func (s *HzSystemTokenService) InitDatabase(ctx context.Context, c *app.RequestContext) {
	_, err := s.Client.InitDatabase(ctx, &base.Empty{})
	if err != nil {
		hresp.KitexCallErr(c, tokenSvc.ServiceName, err)
		return
	}
	hresp.BaseSuccessMsg(c, "init database success")
}

func (s *HzSystemTokenService) RefreshToken(ctx context.Context, c *app.RequestContext) {
	userID, err := getUserIDInt64(c)
	if err != nil {
		hresp.BaseErrMsg(c, 1, err.Error(), nil)
		return
	}
	res, err := s.Client.RefreshToken(ctx, &base.IDInt64Req{Id: userID})
	if err != nil {
		hresp.KitexCallErr(c, tokenSvc.ServiceName, err)
		return
	}
	var tokenStr string
	if res != nil && res.Token != nil {
		tokenStr = *res.Token
	}
	hresp.BaseSuccessMsgWithBody(c, "success", TokenResp{Token: tokenStr})
}
