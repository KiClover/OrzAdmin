package mixins

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// IDMixin is the mixin with uint64 type ID field
// and the created_at, updated_at fields.
type IDMixin struct {
	mixin.Schema
}

func (IDMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Uint64("id"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			Comment("Create Time | 创建日期"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("Update Time | 修改日期"),
	}
}
