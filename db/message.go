package db

import "github.com/KiClover/orzfrp/ent"

type MessageServiceDB struct {
	Msg *ent.MsgClient
}

func NewSystemMessageDB(client *ent.Client) *MessageServiceDB {
	return &MessageServiceDB{
		Msg: client.Msg,
	}
}
