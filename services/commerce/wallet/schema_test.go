package wallet

import (
	"sort"
	"strings"
	"testing"

	"entgo.io/ent/dialect/sql/schema"
	"entgo.io/ent/schema/field"
	"github.com/KiClover/orzAdmin/ent/migrate"
)

// 这组测试冻结 doc/plan/WalletDesign.MD §6 的物理模型：
// 只有四张钱包业务表、没有跨服务外键、关键唯一约束与 CHECK 不因重构而丢失。

func TestWalletTablesAreExactlyTheFourBusinessTables(t *testing.T) {
	tables := walletTables()
	if len(tables) != 4 {
		t.Fatalf("wallet tables = %d, want 4", len(tables))
	}

	got := make([]string, 0, len(tables))
	for _, table := range tables {
		got = append(got, table.Name)
	}
	sort.Strings(got)

	want := []string{
		"redemption_codes",
		"wallet_accounts",
		"wallet_recharges",
		"wallet_transactions",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("wallet tables = %v, want %v", got, want)
	}

	// 生成结果里必须同时包含这四张表，InitDatabase 才能用 migrate.Create 建出来。
	generated := make(map[string]struct{}, len(migrate.Tables))
	for _, table := range migrate.Tables {
		generated[table.Name] = struct{}{}
	}
	for _, name := range want {
		if _, ok := generated[name]; !ok {
			t.Fatalf("ent/migrate 未生成表 %s", name)
		}
	}
}

func TestWalletAccountsSchema(t *testing.T) {
	table := walletTable(t, "wallet_accounts")
	requireColumns(t, table, "id", "user_id", "currency", "balance_amount", "revision", "created_at", "updated_at")

	if got := column(table, "currency").Default; got != "CNY" {
		t.Fatalf("currency default = %v, want CNY", got)
	}
	if got := column(table, "balance_amount").Type; got != field.TypeInt64 {
		t.Fatalf("balance_amount type = %v, want int64（必须支持负数）", got)
	}
	requireUniqueIndex(t, table, "user_id", "currency")
	requireCheck(t, table, "wallet_accounts_currency_cny")
	requireCheck(t, table, "wallet_accounts_user_id_positive")
}

func TestWalletTransactionsSchema(t *testing.T) {
	table := walletTable(t, "wallet_transactions")
	requireColumns(t, table,
		"id", "wallet_id", "type", "currency", "delta_amount", "balance_after",
		"owner_namespace", "business_type", "business_id", "related_transaction_id",
		"description", "actor", "created_at")

	for _, name := range []string{"wallet_id", "type", "owner_namespace", "business_type", "business_id", "actor"} {
		if column(table, name).Nullable {
			t.Fatalf("%s 不允许为空", name)
		}
	}
	if !column(table, "related_transaction_id").Nullable {
		t.Fatal("related_transaction_id 必须可空")
	}
	requireUniqueIndex(t, table, "owner_namespace", "type", "business_type", "business_id")
	requireUniqueIndex(t, table, "related_transaction_id")
	requireCheck(t, table, "wallet_transactions_delta_non_zero")
	requireCheck(t, table, "wallet_transactions_refund_relation")
	requireCheck(t, table, "wallet_transactions_type_valid")
	requireCheck(t, table, "wallet_transactions_currency_cny")

	// 管理员调账新增第六种流水类型；既有五类必须同时保留，避免生成时被替换。
	typeCheck := table.Annotation.Checks["wallet_transactions_type_valid"]
	for _, transactionType := range []string{
		"RECHARGE_CREDIT", "BALANCE_DEBIT", "BALANCE_REFUND",
		"RECHARGE_REFUND", "REDEMPTION_CREDIT", "ADMIN_ADJUSTMENT",
	} {
		if !strings.Contains(typeCheck, "'"+transactionType+"'") {
			t.Fatalf("type CHECK 缺少 %s: %s", transactionType, typeCheck)
		}
	}
}

func TestWalletRechargesSchema(t *testing.T) {
	table := walletTable(t, "wallet_recharges")
	requireColumns(t, table,
		"id", "wallet_id", "request_id", "merchant_order_no", "provider", "provider_environment",
		"payment_profile_id", "payment_profile_revision", "provider_product_id",
		"provider_checkout_id", "provider_order_id", "provider_transaction_id",
		"wallet_amount", "wallet_currency", "provider_amount", "provider_currency",
		"fx_rate_numerator", "fx_rate_denominator", "fx_setting_revision", "fx_quoted_at",
		"fx_rounding_mode", "status", "checkout_url", "paid_at", "canceled_at", "refunded_at",
		"review_reason", "review_blocks_debit", "reviewed_by", "reviewed_at",
		"refund_reason", "refund_requested_by", "refund_requested_at", "revision", "created_at", "updated_at")

	for _, name := range []string{"wallet_id", "request_id", "merchant_order_no", "provider", "provider_environment",
		"payment_profile_id", "payment_profile_revision", "provider_product_id", "wallet_amount",
		"wallet_currency", "provider_amount", "provider_currency", "fx_rate_numerator",
		"fx_rate_denominator", "fx_setting_revision", "fx_quoted_at", "fx_rounding_mode", "status"} {
		if column(table, name).Nullable {
			t.Fatalf("%s 不允许为空", name)
		}
	}
	for _, name := range []string{"provider_checkout_id", "provider_order_id", "provider_transaction_id",
		"checkout_url", "paid_at", "canceled_at", "refunded_at"} {
		if !column(table, name).Nullable {
			t.Fatalf("%s 必须可空", name)
		}
	}
	if got := column(table, "review_blocks_debit").Default; got != false {
		t.Fatalf("review_blocks_debit default = %v, want false", got)
	}
	requireUniqueIndex(t, table, "wallet_id", "request_id")
	requireUniqueIndex(t, table, "merchant_order_no")
	requireUniqueIndex(t, table, "provider", "provider_environment", "provider_checkout_id")
	requireUniqueIndex(t, table, "provider", "provider_environment", "provider_order_id")
	requireUniqueIndex(t, table, "provider", "provider_environment", "provider_transaction_id")
	requireIndex(t, table, "wallet_id", "review_blocks_debit", "status")
	for _, check := range []string{
		"wallet_recharges_status_valid",
		"wallet_recharges_provider_creem",
		"wallet_recharges_environment_valid",
		"wallet_recharges_amounts_positive",
		"wallet_recharges_currencies",
		"wallet_recharges_fx_positive",
	} {
		requireCheck(t, table, check)
	}

	statusCheck := table.Annotation.Checks["wallet_recharges_status_valid"]
	for _, status := range []string{"CREATED", "PENDING_PAYMENT", "SUCCEEDED", "CANCELED", "FAILED", "REVIEW", "REFUNDING", "REFUNDED"} {
		if !strings.Contains(statusCheck, "'"+status+"'") {
			t.Fatalf("status CHECK 缺少 %s: %s", status, statusCheck)
		}
	}
}

func TestRedemptionCodesSchema(t *testing.T) {
	table := walletTable(t, "redemption_codes")
	requireColumns(t, table,
		"id", "request_id", "type", "code", "amount", "currency", "status",
		"expires_at", "redeemed_count", "claimed_by_user_id", "claimed_at", "created_by",
		"created_at", "updated_at")

	for _, name := range []string{"request_id", "type", "code", "amount", "currency", "status", "created_by"} {
		if column(table, name).Nullable {
			t.Fatalf("%s 不允许为空", name)
		}
	}
	for _, name := range []string{"expires_at", "claimed_by_user_id", "claimed_at"} {
		if !column(table, name).Nullable {
			t.Fatalf("%s 必须可空", name)
		}
	}
	requireUniqueIndex(t, table, "request_id")
	requireUniqueIndex(t, table, "code")
	requireCheck(t, table, "redemption_codes_amount_positive")
	requireCheck(t, table, "redemption_codes_claim_consistent")
	requireCheck(t, table, "redemption_codes_type_valid")
	requireCheck(t, table, "redemption_codes_status_valid")

	// EXPIRED 由 API 派生，不落库。
	statusCheck := table.Annotation.Checks["redemption_codes_status_valid"]
	if strings.Contains(statusCheck, "EXPIRED") {
		t.Fatalf("status CHECK 不应包含派生状态 EXPIRED: %s", statusCheck)
	}
}

func TestWalletSchemaHasNoCrossServiceForeignKeys(t *testing.T) {
	for _, table := range walletTables() {
		if len(table.ForeignKeys) != 0 {
			t.Fatalf("%s 不允许有外键（钱包不引用其它服务的表）", table.Name)
		}
	}
}

func walletTable(t *testing.T, name string) *schema.Table {
	t.Helper()
	for _, table := range walletTables() {
		if table.Name == name {
			return table
		}
	}
	t.Fatalf("wallet table %s not found", name)
	return nil
}

func column(table *schema.Table, name string) *schema.Column {
	for _, col := range table.Columns {
		if col.Name == name {
			return col
		}
	}
	return nil
}

func requireColumns(t *testing.T, table *schema.Table, names ...string) {
	t.Helper()
	for _, name := range names {
		if column(table, name) == nil {
			t.Fatalf("%s 缺少列 %s", table.Name, name)
		}
	}
}

func indexColumns(idx *schema.Index) []string {
	names := make([]string, 0, len(idx.Columns))
	for _, col := range idx.Columns {
		names = append(names, col.Name)
	}
	return names
}

func findIndex(table *schema.Table, columns ...string) *schema.Index {
	want := strings.Join(columns, ",")
	for _, idx := range table.Indexes {
		if strings.Join(indexColumns(idx), ",") == want {
			return idx
		}
	}
	return nil
}

func requireUniqueIndex(t *testing.T, table *schema.Table, columns ...string) {
	t.Helper()
	idx := findIndex(table, columns...)
	if idx == nil {
		t.Fatalf("%s 缺少索引 (%s)", table.Name, strings.Join(columns, ","))
	}
	if !idx.Unique {
		t.Fatalf("%s 的索引 (%s) 必须唯一", table.Name, strings.Join(columns, ","))
	}
}

func requireIndex(t *testing.T, table *schema.Table, columns ...string) {
	t.Helper()
	if findIndex(table, columns...) == nil {
		t.Fatalf("%s 缺少索引 (%s)", table.Name, strings.Join(columns, ","))
	}
}

func requireCheck(t *testing.T, table *schema.Table, name string) {
	t.Helper()
	if table.Annotation == nil || table.Annotation.Checks[name] == "" {
		t.Fatalf("%s 缺少 CHECK %s", table.Name, name)
	}
}
