package jwt

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

// Option describes the jwt extra data
type Option struct {
	Key string
	Val any
}

// WithOption returns the option from k/v
func WithOption(key string, val any) Option {
	return Option{
		Key: key,
		Val: val,
	}
}

func NewJwtToken(secret string, iat, sec int64, opts ...Option) (string, error) {
	claim := jwt.MapClaims{}
	claim["exp"] = iat + sec
	claim["iat"] = iat

	for _, v := range opts {
		claim[v.Key] = v.Val
	}

	token := jwt.New(jwt.SigningMethodHS256)
	token.Claims = claim
	return token.SignedString([]byte(secret))
}

func ParseJwtToken(secret string, tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
