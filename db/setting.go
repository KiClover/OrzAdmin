package db

import "github.com/KiClover/orzfrp/ent"

type SettingServiceDB struct {
	Setting *ent.SettingClient
}

func NewSystemSettingDB(client *ent.Client) *SettingServiceDB {
	return &SettingServiceDB{
		Setting: client.Setting,
	}
}
