package utils

import "github.com/golang-jwt/jwt/v5"

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
