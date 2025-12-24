package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Role struct {
	ent.Schema
}

func (Role) Fields() []ent.Field {
	return []ent.Field{
		field.Uint64("id"),
		field.String("role_code").
			Comment("Role Code | 角色代码").
			Unique().
			NotEmpty(),
		field.String("role_name").
			Comment("Role Name | 角色名称").
			NotEmpty(),
		field.String("remark").
			Comment("Remark | 备注").
			Default("").
			Optional(),
	}
}

func (Role) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("role_name"),
		index.Fields("role_code").
			Unique(),
	}
}
