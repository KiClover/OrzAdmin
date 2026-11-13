package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WalletTransaction 是钱包的不可变流水（设计 §6.2）。
// 该表只追加：本 Schema 不提供 updated_at，业务层禁止 UPDATE/DELETE。
// 流水查询需要的类型/币种取值范围与退款关联用数据库 CHECK 固化。
type WalletTransaction struct {
	ent.Schema
}

// Fields of the WalletTransaction.
func (WalletTransaction) Fields() []ent.Field {
	return []ent.Field{
		field.Uint64("id"),
		field.Uint64("wallet_id").
			Comment("Wallet id | 所属钱包，跨服务只存 ID，不建外键"),
		field.String("type").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(32)"}).
			Comment("Transaction type | RECHARGE_CREDIT/BALANCE_DEBIT/BALANCE_REFUND/RECHARGE_REFUND/REDEMPTION_CREDIT/ADMIN_ADJUSTMENT"),
		field.String("currency").
			Default("CNY").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(8)"}).
			Comment("Currency | 流水币种，首期固定 CNY"),
		field.Int64("delta_amount").
			Comment("Delta in minor units | 有符号变动额，禁止为 0"),
		field.Int64("balance_after").
			Comment("Balance after | 同一事务提交后的钱包余额"),
		field.String("owner_namespace").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(64)"}).
			Comment("Owner namespace | 服务端派生的可信命名空间，客户端不可覆盖"),
		field.String("business_type").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(64)"}).
			Comment("Business type | 命名空间内稳定的业务类型"),
		field.String("business_id").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(128)"}).
			Comment("Business id | 命名空间内稳定的业务 ID"),
		field.Uint64("related_transaction_id").
			Optional().
			Nillable().
			Comment("Related transaction id | 仅退款类流水关联原交易"),
		field.String("description").
			Default("").
			SchemaType(map[string]string{dialect.MySQL: "varchar(512)"}).
			Comment("Description | 审计描述"),
		field.String("actor").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(128)"}).
			Comment("Actor | 可信来源生成的简短审计标识，不接受用户覆盖"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			Comment("Create Time | 流水只追加，创建后不可修改"),
	}
}

// Indexes of the WalletTransaction.
func (WalletTransaction) Indexes() []ent.Index {
	return []ent.Index{
		// 同一命名空间下的业务意图只能落一次流水。
		index.Fields("owner_namespace", "type", "business_type", "business_id").
			Unique(),
		// 一次原交易只能被同一类退款关联一次；NULL 可重复。
		index.Fields("related_transaction_id").
			Unique(),
		index.Fields("wallet_id", "id"),
	}
}

// Annotations of the WalletTransaction.
func (WalletTransaction) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Checks(map[string]string{
			"wallet_transactions_type_valid":      "`type` IN ('RECHARGE_CREDIT','BALANCE_DEBIT','BALANCE_REFUND','RECHARGE_REFUND','REDEMPTION_CREDIT','ADMIN_ADJUSTMENT')",
			"wallet_transactions_currency_cny":    "`currency` = 'CNY'",
			"wallet_transactions_delta_non_zero":  "`delta_amount` <> 0",
			"wallet_transactions_wallet_positive": "`wallet_id` > 0",
			"wallet_transactions_business_key":    "`owner_namespace` <> '' AND `business_type` <> '' AND `business_id` <> ''",
			"wallet_transactions_refund_relation": "((`type` IN ('BALANCE_REFUND','RECHARGE_REFUND') AND `related_transaction_id` IS NOT NULL) OR (`type` NOT IN ('BALANCE_REFUND','RECHARGE_REFUND') AND `related_transaction_id` IS NULL))",
		}),
	}
}
