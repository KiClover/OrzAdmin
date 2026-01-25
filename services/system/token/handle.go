package token

import (
	"context"
	"crypto/rand"
	"fmt"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/migrate"
	tokenEnt "github.com/KiClover/orzAdmin/ent/token"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/token"
	"github.com/KiClover/orzAdmin/pkg/kerr"
	"github.com/KiClover/orzAdmin/pkg/utils"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

const ServiceName = "system-token"

// SystemTokenService implements the last service interface defined in the IDL.
type SystemTokenService struct {
	fx.In
	DB    *db.TokenServiceDB
	Cfg   *conf.Config
	DBAll *ent.Client
}

var _ token.SystemTokenService = (*SystemTokenService)(nil)

func NewSystemTokenService(svc SystemTokenService) token.SystemTokenService {
	return &svc
}

func (s SystemTokenService) InitDatabase(ctx context.Context, req *base.Empty) (res *base.BaseResp, err error) {
	tables := []*schema.Table{migrate.TokensTable}
	if err := migrate.Create(ctx, s.DBAll.Schema, tables, schema.WithForeignKeys(false)); err != nil {
		return nil, err
	}
	return &base.BaseResp{Msg: "success"}, nil
}

func (s SystemTokenService) GetToken(ctx context.Context, req *base.IDInt64Req) (res *token.TokenInfo, err error) {
	exist, err := s.DB.Token.Query().Where(tokenEnt.UserID(uint64(req.Id))).Exist(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	var r *ent.Token
	if !exist {
		key, err := GenerateRandomString(32)
		if err != nil {
			logrus.Errorf("generate random string error: %v", err)
			return nil, err
		}
		sk := fmt.Sprintf("sk-%s", key)
		r, err = s.DB.Token.Create().
			SetUserID(uint64(req.Id)).
			SetToken(sk).
			Save(ctx)
		if err != nil {
			kerr.EntDefaultErr(err)
			return nil, err
		}
	} else {
		r, err = s.DB.Token.Query().Where(tokenEnt.UserID(uint64(req.Id))).
			Only(ctx)
		if err != nil {
			kerr.EntDefaultErr(err)
			return nil, err
		}
	}
	return &token.TokenInfo{
		Token: utils.GetPointer(r.Token),
	}, nil
}

func (s SystemTokenService) RefreshToken(ctx context.Context, req *base.IDInt64Req) (res *token.TokenInfo, err error) {
	r, err := s.DB.Token.Query().Where(tokenEnt.UserID(uint64(req.Id))).
		Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	key, err := GenerateRandomString(32)
	if err != nil {
		logrus.Errorf("generate random string error: %v", err)
		return nil, err
	}
	sk := fmt.Sprintf("sk-%s", key)

	_, err = s.DB.Token.UpdateOne(r).
		SetToken(sk).
		Save(ctx)
	return &token.TokenInfo{
		Token: utils.GetPointer(sk),
	}, nil
}

func GenerateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = charset[b[i]%byte(len(charset))]
	}
	return string(result), nil
}
