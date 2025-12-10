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
		field.String("value").
			Comment("Value | 值"),
	}
}

func (Setting) Edges() []ent.Edge {
	return nil
}
