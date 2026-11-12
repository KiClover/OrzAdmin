//go:build integration

package wallet

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/go-sql-driver/mysql"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
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
