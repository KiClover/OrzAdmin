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

// WalletRecharge 是 Creem 充值订单（设计 §6.3）。
// 订单冻结创建时的 Profile 身份、Product、汇率快照与 Provider ID，
// 重复 request_id 不再按当前 Setting 重新报价。
type WalletRecharge struct {
	ent.Schema
}

// Fields of the WalletRecharge.
func (WalletRecharge) Fields() []ent.Field {
	return []ent.Field{
		field.Uint64("wallet_id").
			Comment("Wallet id | 充值目标钱包，跨服务只存 ID，不建外键"),
		field.String("request_id").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(64)"}).
			Comment("Request id | 前端一次充值意图的稳定 ID"),
		field.String("merchant_order_no").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(64)"}).
			Comment("Merchant order no | 服务端生成的全局唯一跟踪号"),
		field.String("provider").
			Default("CREEM").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(32)"}).
			Comment("Payment provider | 首期固定 CREEM"),
		field.String("provider_environment").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(32)"}).
			Comment("Provider environment | TEST 或 PRODUCTION"),
		field.String("payment_profile_id").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(64)"}).
			Comment("Payment profile id | 创建订单时使用的不可变 Profile 身份"),
		field.Int64("payment_profile_revision").
			Comment("Payment profile revision | 创建订单时使用的配置代际"),
		field.String("provider_product_id").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(128)"}).
			Comment("Provider product id | 创建订单时使用的 Product，后续校验不读取当前 Product"),
		field.String("provider_checkout_id").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "varchar(128)"}).
			Comment("Provider checkout id | Checkout 创建后回填"),
		field.String("provider_order_id").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "varchar(128)"}).
			Comment("Provider order id | Provider 订单 ID，非空时按环境唯一"),
		field.String("provider_transaction_id").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "varchar(128)"}).
			Comment("Provider transaction id | Provider 交易 ID，非空时按环境唯一"),
		field.Int64("wallet_amount").
			Comment("Wallet amount | 最终进入钱包的 CNY 分值"),
		field.String("wallet_currency").
			Default("CNY").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(8)"}).
			Comment("Wallet currency | 钱包侧币种，固定 CNY"),
		field.Int64("provider_amount").
			Comment("Provider amount | Checkout 固化的 USD 美分值"),
		field.String("provider_currency").
			Default("USD").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(8)"}).
			Comment("Provider currency | Provider 侧币种，固定 USD"),
		field.Int64("fx_rate_numerator").
			Comment("FX rate numerator | 1 USD = N CNY 的分子"),
		field.Int64("fx_rate_denominator").
			Comment("FX rate denominator | 1 USD = N CNY 的分母"),
		field.Int64("fx_setting_revision").
			Comment("FX setting revision | 报价使用的 Setting 代际"),
		field.Time("fx_quoted_at").
			Comment("FX quoted at | 汇率报价时间"),
		field.String("fx_rounding_mode").
			Default("CEIL_PROVIDER_MINOR_UNIT").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(32)"}).
			Comment("FX rounding mode | 取整规则，固定 CEIL_PROVIDER_MINOR_UNIT"),
		field.String("status").
			Default("CREATED").
			NotEmpty().
			SchemaType(map[string]string{dialect.MySQL: "varchar(32)"}).
			Comment("Recharge status | CREATED/PENDING_PAYMENT/SUCCEEDED/CANCELED/FAILED/REVIEW/REFUNDING/REFUNDED"),
		field.String("checkout_url").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "varchar(1024)"}).
			Comment("Checkout url | 待付地址，不存伪精确的过期时间"),
		field.Time("paid_at").
			Optional().
			Nillable().
			Comment("Paid at | Provider 确认付款时间"),
		field.Time("canceled_at").
			Optional().
			Nillable().
			Comment("Canceled at | 用户取消本地订单时间"),
		field.Time("refunded_at").
			Optional().
			Nillable().
			Comment("Refunded at | Provider 退款确认时间"),
		field.String("review_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "varchar(512)"}).
			Comment("Review reason | 稳定原因码"),
		field.Bool("review_blocks_debit").
			Default(false).
			Comment("Review blocks debit | Provider 逆向事故期间阻断扣款"),
		field.String("reviewed_by").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "varchar(128)"}).
			Comment("Reviewed by | 审核操作人"),
		field.Time("reviewed_at").
			Optional().
			Nillable().
			Comment("Reviewed at | 审核时间"),
		field.String("refund_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "varchar(512)"}).
			Comment("Refund reason | 退款原因"),
		field.String("refund_requested_by").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "varchar(128)"}).
			Comment("Refund requested by | 退款发起操作人"),
		field.Time("refund_requested_at").
			Optional().
			Nillable().
			Comment("Refund requested at | 退款发起时间"),
		field.Uint64("revision").
			Default(1).
			Comment("Optimistic lock revision | 状态变化递增"),
	}
}

// Mixin of the WalletRecharge.
func (WalletRecharge) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.IDMixin{},
	}
}

// Indexes of the WalletRecharge.
func (WalletRecharge) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("wallet_id", "request_id").
			Unique(),
		index.Fields("merchant_order_no").
			Unique(),
		// Provider 侧 ID 在环境内唯一；NULL 允许重复（尚未回填）。
		index.Fields("provider", "provider_environment", "provider_checkout_id").
			Unique(),
		index.Fields("provider", "provider_environment", "provider_order_id").
			Unique(),
		index.Fields("provider", "provider_environment", "provider_transaction_id").
			Unique(),
		// 扣款事务快速检查钱包是否存在被阻断的订单。
		index.Fields("wallet_id", "review_blocks_debit", "status"),
	}
}

// Annotations of the WalletRecharge.
//
// 金额、币种与汇率必须为正的结构性约束固化在数据库；status、provider、
// environment 与取整策略属于会随业务增长的业务枚举，改由服务层状态机校验。
func (WalletRecharge) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Checks(map[string]string{
			"wallet_recharges_amounts_positive":  "`wallet_amount` > 0 AND `provider_amount` > 0",
			"wallet_recharges_currencies":        "`wallet_currency` = 'CNY' AND `provider_currency` = 'USD'",
			"wallet_recharges_fx_positive":       "`fx_rate_numerator` > 0 AND `fx_rate_denominator` > 0 AND `fx_setting_revision` > 0",
		}),
	}
}
