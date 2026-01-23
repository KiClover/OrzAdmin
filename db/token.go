package db

import "github.com/KiClover/orzAdmin/ent"

type TokenServiceDB struct {
	Token *ent.TokenClient
}

func NewSystemTokenDB(client *ent.Client) *TokenServiceDB {
	return &TokenServiceDB{
		Token: client.Token,
	}
}
