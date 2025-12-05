package system

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"github.com/KiClover/orzfrp/ent/mixins"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Comment("User Name | 用户名"),
		// 个人资料
		field.String("avatar").
			SchemaType(map[string]string{dialect.MySQL: "varchar(512)"}).
			Optional().
			Comment("Avatar | 头像路径"),
		// Gender: 1 Man 2 Woman 3 Secret
		field.Uint("gender").Comment("Gender | 性别").Default(3),
		// 隐私信息
		field.String("phone").Comment("Phone | 手机号").Unique().Optional(),
		field.String("email").Comment("Email | 邮箱").Unique().Optional(),
		field.String("pwd").Comment("Password | 密码"),
	}
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.IDMixin{},
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return nil
}
