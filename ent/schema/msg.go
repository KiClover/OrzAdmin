package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Msg struct {
	ent.Schema
}

func (Msg) Fields() []ent.Field {
	return []ent.Field{
		field.Uint64("id"),
		field.Uint32("type").
			Comment("Type | 发送类型(1 Email 2 Mobile"),
		field.String("to").
			Comment("To | 目标"),
		field.String("content").
			Comment("Content | 内容"),
		field.Uint8("send_status").
			Comment("The send status, 0 unknown 1 success 2 failed | 发送的状态, 0 未知， 1 成功， 2 失败"),
		field.Time("sent_at").
			Immutable().
			Default(time.Now).
			Comment("Send Time | 发送时间"),
	}
}
