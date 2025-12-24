package db

import "github.com/KiClover/orzfrp/ent"

type AccessServiceDB struct {
	Api  *ent.APIClient
	Role *ent.RoleClient
}

func NewSystemAccessDB(client *ent.Client) *AccessServiceDB {
	return &AccessServiceDB{
		Role: client.Role,
		Api:  client.Api,
	}
}
