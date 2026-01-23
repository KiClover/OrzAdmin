package db

import "github.com/KiClover/orzAdmin/ent"

type SettingServiceDB struct {
	Setting *ent.SettingClient
}

func NewSystemSettingDB(client *ent.Client) *SettingServiceDB {
	return &SettingServiceDB{
		Setting: client.Setting,
	}
}
