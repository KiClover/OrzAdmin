package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/KiClover/orzAdmin/ent/mixins"
)

// WalletAccount 是用户的 CNY 钱包账户（设计 §6.1）。
// 一个用户只有一个钱包，余额允许为负，欠款状态由余额派生，不落 status 列。
type WalletAccount struct {
	ent.Schema
}

// Fields of the WalletAccount.
func (WalletAccount) Fields() []ent.Field {
	return []ent.Field{
		field.Uint64("user_id").
			Comment("Owner user id | 钱包所属用户，跨服务只存 ID，不建外键"),
		field.String("currency").
			Default("CNY").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(8)"}).
			Comment("Wallet currency | 钱包币种，首期固定 CNY"),
		field.Int64("balance_amount").
			Default(0).
			Comment("Balance in minor units | 有符号余额，负数表示欠款"),
		field.Uint64("revision").
			Default(1).
			Comment("Optimistic lock revision | 每次余额变化递增"),
	}
}

// Mixin of the WalletAccount.
func (WalletAccount) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.IDMixin{},
	}
}

// Indexes of the WalletAccount.
func (WalletAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "currency").
			Unique(),
	}
}

// Annotations of the WalletAccount.
func (WalletAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Checks(map[string]string{
			"wallet_accounts_currency_cny":     "`currency` = 'CNY'",
			"wallet_accounts_user_id_positive": "`user_id` > 0",
		}),
	}
}
