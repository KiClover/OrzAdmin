package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Setting struct {
	ent.Schema
}

func (Setting) Fields() []ent.Field {
	return []ent.Field{
		field.Uint64("id"),
		field.String("key").
			Comment("Key | 键").
			Unique(),
		// 钱包配置以有大小限制的 JSON 形式保存在 wallet_config 行，
		// 需要 TEXT 才能容纳，因此 value 使用 field.Text（生成 TEXT/LONGTEXT）。
		field.Text("value").
			Comment("Value | 值"),
	}
}

func (Setting) Edges() []ent.Edge {
	return nil
}
