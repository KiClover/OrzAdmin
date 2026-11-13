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

// RedemptionCode 是兑换码（设计 §6.4）。
// 只存 ACTIVE/DISABLED/CLAIMED 三个状态，过期为 API 派生，不启动调度器改状态。
type RedemptionCode struct {
	ent.Schema
}

// Fields of the RedemptionCode.
func (RedemptionCode) Fields() []ent.Field {
	return []ent.Field{
		field.String("request_id").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(64)"}).
			Comment("Request id | 管理员创建请求的全局唯一键"),
		field.String("type").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(32)"}).
			Comment("Code type | UNIQUE 全局一次，SHARED 每人一次"),
		field.String("code").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(128)"}).
			Annotations(entsql.Annotation{Collation: "utf8mb4_bin"}).
			Comment("Code | 完整明文兑换码，按原值精确匹配"),
		field.Int64("amount").
			Comment("Amount | 入账 CNY 分值，必须为正"),
		field.String("currency").
			Default("CNY").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(8)"}).
			Comment("Currency | 兑换币种，固定 CNY"),
		field.String("status").
			Default("ACTIVE").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(32)"}).
			Comment("Status | ACTIVE/DISABLED/CLAIMED"),
		field.Time("expires_at").
			Optional().
			Nillable().
			Comment("Expires at | 过期时间为空表示长期有效"),
		field.Uint32("redeemed_count").
			Default(0).
			Comment("Redeemed count | 只作管理展示，防重以流水唯一键为准"),
		field.Uint64("claimed_by_user_id").
			Optional().
			Nillable().
			Comment("Claimed by user id | 仅 UNIQUE 码使用"),
		field.Time("claimed_at").
			Optional().
			Nillable().
			Comment("Claimed at | 仅 UNIQUE 码使用"),
		field.String("created_by").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(128)"}).
			Comment("Created by | 创建该码的管理员"),
	}
}

// Mixin of the RedemptionCode.
func (RedemptionCode) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.IDMixin{},
	}
}

// Indexes of the RedemptionCode.
func (RedemptionCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id").
			Unique(),
		index.Fields("code").
			Unique(),
		index.Fields("status", "expires_at"),
	}
}

// Annotations of the RedemptionCode.
//
// 只固化金额为正、币种与领取一致性；type/status 是业务枚举，由服务层校验，
// 避免新增枚举值需要改同名 CHECK 表达式。
func (RedemptionCode) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Checks(map[string]string{
			"redemption_codes_amount_positive":  "`amount` > 0",
			"redemption_codes_currency_cny":     "`currency` = 'CNY'",
			"redemption_codes_claim_consistent": "((`claimed_by_user_id` IS NULL AND `claimed_at` IS NULL) OR (`claimed_by_user_id` IS NOT NULL AND `claimed_at` IS NOT NULL))",
		}),
	}
}
