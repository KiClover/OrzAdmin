package db

import "github.com/KiClover/orzAdmin/ent"

type UserServiceDB struct {
	User *ent.UserClient
}

func NewSystemUserDB(client *ent.Client) *UserServiceDB {
	return &UserServiceDB{
		User: client.User,
	}
}
