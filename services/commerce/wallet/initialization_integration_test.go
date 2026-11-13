//go:build integration

package wallet

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/go-sql-driver/mysql"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/redemptioncode"
)

// 集成测试需要真实 MySQL：WALLET_TEST_MYSQL_DSN，例如
// user:pass@tcp(127.0.0.1:3306)/orz_admin_test?parseTime=True
//
// 运行：go test -tags=integration ./services/commerce/wallet -run TestWalletInitializationMySQL -count=1
// 表级 CHECK 需要 MySQL >= 8.0.16；更低的版本会跳过与本测试相关的约束断言。

var walletTableNames = []string{
	"wallet_accounts",
	"wallet_transactions",
	"wallet_recharges",
	"redemption_codes",
}

func integrationWalletService(t *testing.T) (*CommerceWalletService, *ent.Client, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("WALLET_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("WALLET_TEST_MYSQL_DSN 未设置，跳过钱包初始化集成测试")
	}
	rawdb, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	rawdb.SetMaxOpenConns(8)
	if err := rawdb.Ping(); err != nil {
		_ = rawdb.Close()
		t.Fatalf("ping mysql: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB("mysql", rawdb)))
	t.Cleanup(func() {
		_ = client.Close()
		_ = rawdb.Close()
	})
	svc := &CommerceWalletService{
		DBAll: client,
		Cfg:   &conf.Config{},
	}
	return svc, client, rawdb
}

func dropWalletTables(t *testing.T, rawdb *sql.DB) {
	t.Helper()
	for _, name := range walletTableNames {
		if _, err := rawdb.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+name); err != nil {
			t.Fatalf("drop %s: %v", name, err)
		}
	}
}

func countWalletTables(t *testing.T, rawdb *sql.DB) map[string]struct{} {
	t.Helper()
	rows, err := rawdb.QueryContext(context.Background(),
		`SELECT TABLE_NAME FROM information_schema.TABLES
		 WHERE TABLE_SCHEMA = DATABASE()
		   AND TABLE_NAME IN ('wallet_accounts','wallet_transactions','wallet_recharges','redemption_codes')`)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer rows.Close()
	found := make(map[string]struct{}, 4)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		found[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}
	return found
}

func countForeignKeys(t *testing.T, rawdb *sql.DB) int {
	t.Helper()
	var count int
	err := rawdb.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS
		 WHERE TABLE_SCHEMA = DATABASE() AND CONSTRAINT_TYPE = 'FOREIGN KEY'
		   AND TABLE_NAME IN ('wallet_accounts','wallet_transactions','wallet_recharges','redemption_codes')`).Scan(&count)
	if err != nil {
		t.Fatalf("query foreign keys: %v", err)
	}
	return count
}

func checkConstraintNames(t *testing.T, rawdb *sql.DB) map[string]struct{} {
	t.Helper()
	rows, err := rawdb.QueryContext(context.Background(),
		`SELECT cc.CONSTRAINT_NAME
		   FROM information_schema.CHECK_CONSTRAINTS cc
		   JOIN information_schema.TABLE_CONSTRAINTS tc
		     ON tc.CONSTRAINT_SCHEMA = cc.CONSTRAINT_SCHEMA
		    AND tc.CONSTRAINT_NAME = cc.CONSTRAINT_NAME
		  WHERE cc.CONSTRAINT_SCHEMA = DATABASE()
		    AND tc.TABLE_NAME IN ('wallet_accounts','wallet_transactions','wallet_recharges','redemption_codes')`)
	if err != nil {
		t.Fatalf("query checks: %v", err)
	}
	defer rows.Close()
	names := make(map[string]struct{}, 20)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan check: %v", err)
		}
		names[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate checks: %v", err)
	}
	return names
}

func requireMySQLCheckSupport(t *testing.T, rawdb *sql.DB) {
	t.Helper()
	var version string
	if err := rawdb.QueryRowContext(context.Background(), "SELECT VERSION()").Scan(&version); err != nil {
		t.Fatalf("query version: %v", err)
	}
	// MySQL 8.0.16 起才真正执行表级 CHECK；MariaDB 需要 10.2.1。
	major, minor, patch, ok := parseMySQLVersion(version)
	if !ok {
		t.Logf("无法解析 MySQL 版本 %q，跳过约束断言", version)
		return
	}
	if major == 5 {
		t.Skipf("MySQL %s 不执行表级 CHECK，跳过约束断言", version)
	}
	if major == 8 && minor == 0 && patch < 16 {
		t.Skipf("MySQL %s 不执行表级 CHECK，跳过约束断言", version)
	}
}

func parseMySQLVersion(version string) (major, minor, patch int, ok bool) {
	core := version
	if idx := strings.IndexAny(core, "-+"); idx >= 0 {
		core = core[:idx]
	}
	parts := strings.SplitN(core, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	values := make([]int, 3)
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return 0, 0, 0, false
		}
		values[i] = value
	}
	return values[0], values[1], values[2], true
}

func TestWalletInitializationMySQL(t *testing.T) {
	svc, client, rawdb := integrationWalletService(t)
	requireMySQLCheckSupport(t, rawdb)
	ctx := context.Background()

	dropWalletTables(t, rawdb)

	t.Run("creates exactly the four wallet tables", func(t *testing.T) {
		if _, err := svc.InitDatabase(ctx, nil); err != nil {
			t.Fatalf("init database: %v", err)
		}
		found := countWalletTables(t, rawdb)
		if len(found) != len(walletTableNames) {
			t.Fatalf("wallet tables = %v, want %v", found, walletTableNames)
		}
		for _, name := range walletTableNames {
			if _, ok := found[name]; !ok {
				t.Fatalf("missing table %s", name)
			}
		}
		if countForeignKeys(t, rawdb) != 0 {
			t.Fatal("钱包表不允许包含外键")
		}
		checks := checkConstraintNames(t, rawdb)
		for _, name := range []string{
			"wallet_accounts_currency_cny",
			"wallet_transactions_delta_non_zero",
			"wallet_transactions_refund_relation",
			"wallet_recharges_status_valid",
			"redemption_codes_amount_positive",
			"redemption_codes_claim_consistent",
		} {
			if _, ok := checks[name]; !ok {
				t.Fatalf("缺少 CHECK 约束 %s，实际 %v", name, checks)
			}
		}
	})

	store := db.NewCommerceWalletDB(client)

	t.Run("empty tables report no business data", func(t *testing.T) {
		present, err := store.HasBusinessData(ctx)
		if err != nil {
			t.Fatalf("has business data: %v", err)
		}
		if present {
			t.Fatal("空表被判定为已有业务数据")
		}
	})

	var accountID uint64
	t.Run("writes business data", func(t *testing.T) {
		account, err := client.WalletAccount.Create().
			SetUserID(7001).
			Save(ctx)
		if err != nil {
			t.Fatalf("create account: %v", err)
		}
		accountID = account.ID
		present, err := store.HasBusinessData(ctx)
		if err != nil {
			t.Fatalf("has business data: %v", err)
		}
		if !present {
			t.Fatal("已有账户但未判定为业务数据")
		}
	})

	t.Run("repeated initialization preserves data", func(t *testing.T) {
		if _, err := svc.InitDatabase(ctx, nil); err != nil {
			t.Fatalf("second init database: %v", err)
		}
		total, err := client.WalletAccount.Query().Count(ctx)
		if err != nil {
			t.Fatalf("count accounts: %v", err)
		}
		if total != 1 {
			t.Fatalf("accounts after re-init = %d, want 1", total)
		}
		account, err := client.WalletAccount.Get(ctx, accountID)
		if err != nil {
			t.Fatalf("get account: %v", err)
		}
		if account.UserID != 7001 {
			t.Fatalf("user_id = %d, want 7001", account.UserID)
		}
		found := countWalletTables(t, rawdb)
		if len(found) != len(walletTableNames) {
			t.Fatalf("wallet tables after re-init = %v", found)
		}
	})

	t.Run("constraints reject invalid rows", func(t *testing.T) {
		if _, err := client.WalletAccount.Create().SetUserID(7002).SetCurrency("USD").Save(ctx); err == nil {
			t.Fatal("wallet_accounts 允许了非 CNY 币种")
		}
		if _, err := client.WalletAccount.Create().SetUserID(0).Save(ctx); err == nil {
			t.Fatal("wallet_accounts 允许了 user_id = 0")
		}
		_, err := client.WalletTransaction.Create().
			SetWalletID(accountID).
			SetType("RECHARGE_REFUND").
			SetDeltaAmount(100).
			SetBalanceAfter(100).
			SetOwnerNamespace("commerce").
			SetBusinessType("recharge").
			SetBusinessID("biz-1").
			SetActor("system").
			Save(ctx)
		if err == nil {
			t.Fatal("wallet_transactions 允许退款类型缺少关联流水")
		}
	})

	t.Run("query failures are not reported as empty", func(t *testing.T) {
		if _, err := rawdb.ExecContext(ctx, "DROP TABLE wallet_transactions"); err != nil {
			t.Fatalf("drop wallet_transactions: %v", err)
		}
		_, err := store.HasBusinessData(ctx)
		if err == nil {
			t.Fatal("表缺失时 HasBusinessData 把错误当作没有数据")
		}
	})
}

// walletTransactionTypeCheckClause 读取数据库里 wallet_transactions 类型 CHECK 的实际子句，
// 用于断言 InitDatabase 是否把存量五类约束升级为包含 ADMIN_ADJUSTMENT 的六类约束。
func walletTransactionTypeCheckClause(t *testing.T, rawdb *sql.DB) string {
	t.Helper()
	var clause string
	err := rawdb.QueryRowContext(context.Background(),
		`SELECT cc.CHECK_CLAUSE
		   FROM information_schema.CHECK_CONSTRAINTS cc
		   JOIN information_schema.TABLE_CONSTRAINTS tc
		     ON tc.CONSTRAINT_SCHEMA = cc.CONSTRAINT_SCHEMA
		    AND tc.CONSTRAINT_NAME = cc.CONSTRAINT_NAME
		  WHERE cc.CONSTRAINT_SCHEMA = DATABASE()
		    AND tc.TABLE_NAME = 'wallet_transactions'
		    AND tc.CONSTRAINT_NAME = 'wallet_transactions_type_valid'`).Scan(&clause)
	if err != nil {
		t.Fatalf("query wallet_transactions_type_valid: %v", err)
	}
	return clause
}

// walletTransactionIndexes 快照 wallet_transactions 的索引定义，用于确认同步 CHECK 不会重建索引。
func walletTransactionIndexes(t *testing.T, rawdb *sql.DB) map[string]string {
	t.Helper()
	rows, err := rawdb.QueryContext(context.Background(),
		`SELECT INDEX_NAME, COLUMN_NAME, NON_UNIQUE
		   FROM information_schema.STATISTICS
		  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'wallet_transactions'
		  ORDER BY INDEX_NAME, SEQ_IN_INDEX`)
	if err != nil {
		t.Fatalf("query statistics: %v", err)
	}
	defer rows.Close()
	indexes := make(map[string]string)
	for rows.Next() {
		var name string
		var column sql.NullString
		var nonUnique int
		if err := rows.Scan(&name, &column, &nonUnique); err != nil {
			t.Fatalf("scan statistics: %v", err)
		}
		indexes[name] += column.String + ":" + strconv.Itoa(nonUnique) + ","
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate statistics: %v", err)
	}
	return indexes
}

// createAdminAdjustmentTransaction 生成一条管理员调账流水，避免各断言重复拼装必填字段。
func createAdminAdjustmentTransaction(ctx context.Context, client *ent.Client, walletID uint64, businessID string, delta int64) (*ent.WalletTransaction, error) {
	return client.WalletTransaction.Create().
		SetWalletID(walletID).
		SetType("ADMIN_ADJUSTMENT").
		SetDeltaAmount(delta).
		SetBalanceAfter(500).
		SetOwnerNamespace(testOwnerNamespace).
		SetBusinessType("admin_adjustment").
		SetBusinessID(businessID).
		SetDescription("客服补偿扣回").
		SetActor("admin:9001").
		Save(ctx)
}

// TestWalletInitDatabaseUpdatesAdminAdjustmentCheckMySQL 验证用户要求的迁移路径：
// 存量库只通过 InitAPI -> Wallet InitDatabase -> migrate.Create 同步 CHECK，不执行额外 SQL。
func TestWalletInitDatabaseUpdatesAdminAdjustmentCheckMySQL(t *testing.T) {
	svc, client, rawdb := integrationWalletService(t)
	requireMySQLCheckSupport(t, rawdb)
	ctx := context.Background()

	dropWalletTables(t, rawdb)
	if _, err := svc.InitDatabase(ctx, nil); err != nil {
		t.Fatalf("init database: %v", err)
	}

	account, err := client.WalletAccount.Create().SetUserID(7101).Save(ctx)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	legacy, err := client.WalletTransaction.Create().
		SetWalletID(account.ID).
		SetType("RECHARGE_CREDIT").
		SetDeltaAmount(1000).
		SetBalanceAfter(1000).
		SetOwnerNamespace(testOwnerNamespace).
		SetBusinessType("recharge").
		SetBusinessID("legacy-1").
		SetActor("system").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed legacy transaction: %v", err)
	}
	indexesBefore := walletTransactionIndexes(t, rawdb)

	// 还原上线前的五类 CHECK，确保后续断言验证的是真实同步行为而不是新建表的结果。
	if _, err := rawdb.ExecContext(ctx, "ALTER TABLE wallet_transactions DROP CHECK wallet_transactions_type_valid"); err != nil {
		t.Fatalf("drop type check: %v", err)
	}
	if _, err := rawdb.ExecContext(ctx,
		"ALTER TABLE wallet_transactions ADD CONSTRAINT wallet_transactions_type_valid CHECK (`type` IN ('RECHARGE_CREDIT','BALANCE_DEBIT','BALANCE_REFUND','RECHARGE_REFUND','REDEMPTION_CREDIT'))"); err != nil {
		t.Fatalf("restore legacy type check: %v", err)
	}
	if clause := walletTransactionTypeCheckClause(t, rawdb); strings.Contains(clause, "ADMIN_ADJUSTMENT") {
		t.Fatalf("构造的存量 CHECK 仍包含 ADMIN_ADJUSTMENT: %s", clause)
	}
	if _, err := createAdminAdjustmentTransaction(ctx, client, account.ID, "probe-legacy", -500); err == nil {
		t.Fatal("存量 CHECK 允许写入 ADMIN_ADJUSTMENT 流水")
	}

	if _, err := svc.InitDatabase(ctx, nil); err != nil {
		t.Fatalf("init database 同步存量 CHECK: %v", err)
	}

	clause := walletTransactionTypeCheckClause(t, rawdb)
	for _, want := range []string{
		"RECHARGE_CREDIT",
		"BALANCE_DEBIT",
		"BALANCE_REFUND",
		"RECHARGE_REFUND",
		"REDEMPTION_CREDIT",
		"ADMIN_ADJUSTMENT",
	} {
		if !strings.Contains(clause, want) {
			t.Fatalf("同步后的 CHECK 缺少 %s: %s", want, clause)
		}
	}

	preserved, err := client.WalletTransaction.Get(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("get legacy transaction: %v", err)
	}
	if preserved.Type != "RECHARGE_CREDIT" || preserved.DeltaAmount != 1000 ||
		preserved.BalanceAfter != 1000 || preserved.BusinessID != "legacy-1" {
		t.Fatalf("存量流水被改写: %+v", preserved)
	}

	if _, err := createAdminAdjustmentTransaction(ctx, client, account.ID, "admin-1", -500); err != nil {
		t.Fatalf("同步后仍拒绝 ADMIN_ADJUSTMENT: %v", err)
	}
	if _, err := client.WalletTransaction.Create().
		SetWalletID(account.ID).
		SetType("NOT_A_TYPE").
		SetDeltaAmount(1).
		SetBalanceAfter(1).
		SetOwnerNamespace(testOwnerNamespace).
		SetBusinessType("admin_adjustment").
		SetBusinessID("bad-1").
		SetActor("admin:9001").
		Save(ctx); err == nil {
		t.Fatal("同步后 CHECK 允许了未知类型")
	}
	if _, err := createAdminAdjustmentTransaction(ctx, client, account.ID, "bad-2", 0); err == nil {
		t.Fatal("同步后允许了零金额调账流水")
	}
	if _, err := client.WalletTransaction.Create().
		SetWalletID(account.ID).
		SetType("BALANCE_REFUND").
		SetDeltaAmount(100).
		SetBalanceAfter(100).
		SetOwnerNamespace(testOwnerNamespace).
		SetBusinessType("refund").
		SetBusinessID("bad-3").
		SetActor("admin:9001").
		Save(ctx); err == nil {
		t.Fatal("同步后允许退款流水缺少关联流水")
	}

	// 重复初始化必须幂等：数据、CHECK、索引都不能被二次迁移破坏。
	if _, err := svc.InitDatabase(ctx, nil); err != nil {
		t.Fatalf("second init database: %v", err)
	}
	if got := walletTransactionTypeCheckClause(t, rawdb); !strings.Contains(got, "ADMIN_ADJUSTMENT") {
		t.Fatalf("二次初始化丢失 ADMIN_ADJUSTMENT: %s", got)
	}
	if indexesAfter := walletTransactionIndexes(t, rawdb); !reflect.DeepEqual(indexesBefore, indexesAfter) {
		t.Fatalf("二次初始化改动了索引: before=%v after=%v", indexesBefore, indexesAfter)
	}
	total, err := client.WalletTransaction.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if total != 2 {
		t.Fatalf("流水数量 = %d, want 2", total)
	}
}

// redemptionCodeCollation 读取 code 列的数据库排序规则。
func redemptionCodeCollation(t *testing.T, rawdb *sql.DB) string {
	t.Helper()
	var collation sql.NullString
	err := rawdb.QueryRowContext(context.Background(),
		`SELECT COLLATION_NAME FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = DATABASE()
		   AND TABLE_NAME = 'redemption_codes'
		   AND COLUMN_NAME = 'code'`).Scan(&collation)
	if err != nil {
		t.Fatalf("query code collation: %v", err)
	}
	return collation.String
}

// TestWalletInitDatabaseAppliesBinaryCodeCollationMySQL 验证存量库的 code 列
// 只通过 InitAPI -> Wallet InitDatabase -> migrate.Create 升级为二进制排序规则，
// 升级后大小写不同的兑换码才能共存并按原值精确匹配。
func TestWalletInitDatabaseAppliesBinaryCodeCollationMySQL(t *testing.T) {
	svc, client, rawdb := integrationWalletService(t)
	ctx := context.Background()

	dropWalletTables(t, rawdb)
	if _, err := svc.InitDatabase(ctx, nil); err != nil {
		t.Fatalf("init database: %v", err)
	}
	legacy, err := client.RedemptionCode.Create().
		SetRequestID("legacy-request-1").
		SetType("UNIQUE").
		SetCode("Vip-2026").
		SetAmount(500).
		SetCreatedBy("9001").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed redemption code: %v", err)
	}
	if got := redemptionCodeCollation(t, rawdb); got != "utf8mb4_bin" {
		t.Fatalf("新建库 code 排序规则 = %q, want utf8mb4_bin", got)
	}

	// 还原上线前的大小写不敏感排序规则，确保后续断言验证的是真实同步行为。
	if _, err := rawdb.ExecContext(ctx,
		"ALTER TABLE redemption_codes MODIFY code varchar(128) COLLATE utf8mb4_general_ci NOT NULL"); err != nil {
		t.Fatalf("restore legacy collation: %v", err)
	}
	if got := redemptionCodeCollation(t, rawdb); got != "utf8mb4_general_ci" {
		t.Fatalf("构造的存量排序规则 = %q, want utf8mb4_general_ci", got)
	}
	if count := countRedemptionCode(t, ctx, client, "vip-2026"); count != 1 {
		t.Fatalf("存量排序规则下小写查询命中 %d 行, want 1", count)
	}

	if _, err := svc.InitDatabase(ctx, nil); err != nil {
		t.Fatalf("init database 同步排序规则: %v", err)
	}

	if got := redemptionCodeCollation(t, rawdb); got != "utf8mb4_bin" {
		t.Fatalf("同步后 code 排序规则 = %q, want utf8mb4_bin", got)
	}
	if count := countRedemptionCode(t, ctx, client, "vip-2026"); count != 0 {
		t.Fatalf("同步后小写查询仍命中 %d 行, want 0", count)
	}
	if count := countRedemptionCode(t, ctx, client, "Vip-2026"); count != 1 {
		t.Fatalf("同步后原值查询命中 %d 行, want 1", count)
	}
	preserved, err := client.RedemptionCode.Get(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("get legacy redemption code: %v", err)
	}
	if preserved.Code != "Vip-2026" || preserved.Amount != 500 ||
		preserved.Type != "UNIQUE" || preserved.Status != "ACTIVE" {
		t.Fatalf("存量兑换码被改写: %+v", preserved)
	}

	// 唯一索引必须随列排序规则一起变得区分大小写。
	if _, err := client.RedemptionCode.Create().
		SetRequestID("legacy-request-2").
		SetType("UNIQUE").
		SetCode("vip-2026").
		SetAmount(500).
		SetCreatedBy("9001").
		Save(ctx); err != nil {
		t.Fatalf("同步后大小写不同的兑换码无法共存: %v", err)
	}

	// 重复初始化必须幂等：排序规则、数据都不能被二次迁移破坏。
	if _, err := svc.InitDatabase(ctx, nil); err != nil {
		t.Fatalf("second init database: %v", err)
	}
	if got := redemptionCodeCollation(t, rawdb); got != "utf8mb4_bin" {
		t.Fatalf("二次初始化后 code 排序规则 = %q, want utf8mb4_bin", got)
	}
	if total, err := client.RedemptionCode.Query().Count(ctx); err != nil {
		t.Fatalf("count redemption codes: %v", err)
	} else if total != 2 {
		t.Fatalf("兑换码数量 = %d, want 2", total)
	}
}

// countRedemptionCode 统计按 code 精确查询命中的行数。
func countRedemptionCode(t *testing.T, ctx context.Context, client *ent.Client, code string) int {
	t.Helper()
	count, err := client.RedemptionCode.Query().
		Where(redemptioncode.Code(code)).
		Count(ctx)
	if err != nil {
		t.Fatalf("count redemption code %q: %v", code, err)
	}
	return count
}
