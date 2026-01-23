package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Token struct {
	ent.Schema
}

func (Token) Fields() []ent.Field {
	return []ent.Field{
		field.Uint64("id"),
		field.String("token").
			Comment("Token | 令牌").
			Unique(),
		field.Uint64("user_id").
			Comment("User ID | 用户ID").
			Unique(),
	}
}
