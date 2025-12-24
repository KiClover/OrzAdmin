package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Api struct {
	ent.Schema
}

func (Api) Fields() []ent.Field {
	return []ent.Field{
		field.Uint64("id"),
		field.String("path").
			Comment("Path | API 路径").
			NotEmpty(),
		field.String("description").
			Comment("Description | API 描述").
			Optional(),
		field.String("group").
			Comment("Group | API 组").
			NotEmpty(),
		field.String("method").
			Comment("Method | 方法").
			Default("POST").
			NotEmpty(),
	}
}

func (Api) Edges() []ent.Edge {
	return nil
}

// Indexes of the SysApi.
func (Api) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("path", "method").
			Unique(),
	}
}
