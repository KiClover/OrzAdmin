//go:build integration

package access

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"
	_ "github.com/go-sql-driver/mysql"

	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	apiEnt "github.com/KiClover/orzAdmin/ent/api"
	"github.com/KiClover/orzAdmin/ent/migrate"
)

// TestSeedAccessMySQLIsIdempotent 在真实 MySQL 上验证种子补齐：
// 首次补齐全部 API 与角色，重复执行不产生重复行、也不改动既有记录。
func TestSeedAccessMySQLIsIdempotent(t *testing.T) {
	dsn := os.Getenv("WALLET_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("WALLET_TEST_MYSQL_DSN is not set")
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.MySQL, sqlDB)))
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if _, err := sqlDB.ExecContext(ctx, "DROP TABLE IF EXISTS apis, roles"); err != nil {
		t.Fatalf("drop access tables: %v", err)
	}
	if err := migrate.Create(ctx, client.Schema,
		[]*schema.Table{migrate.RolesTable, migrate.ApisTable},
		schema.WithForeignKeys(false)); err != nil {
		t.Fatalf("create access tables: %v", err)
	}

	svc := SystemAccessService{DB: db.NewSystemAccessDB(client)}
	if err := svc.seedAccess(ctx); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	firstAPIs, err := client.Api.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count apis: %v", err)
	}
	if firstAPIs != len(APISeeds()) {
		t.Fatalf("expected %d api rows, got %d", len(APISeeds()), firstAPIs)
	}

	// 模拟"旧库升级"：已存在的路由与角色不受影响。
	if _, err := client.Api.Update().
		Where(apiEnt.Path("/admin/wallets/list"), apiEnt.Method("POST")).
		SetDescription("customized by operator").
		Save(ctx); err != nil {
		t.Fatalf("customize existing api: %v", err)
	}

	if err := svc.seedAccess(ctx); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	secondAPIs, err := client.Api.Query().Count(ctx)
	if err != nil {
		t.Fatalf("recount apis: %v", err)
	}
	if secondAPIs != firstAPIs {
		t.Fatalf("seed must be idempotent: %d -> %d", firstAPIs, secondAPIs)
	}
	roles, err := client.Role.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count roles: %v", err)
	}
	if roles != len(RoleSeeds()) {
		t.Fatalf("expected %d roles, got %d", len(RoleSeeds()), roles)
	}

	row, err := client.Api.Query().
		Where(apiEnt.Path("/admin/wallets/list"), apiEnt.Method("POST")).
		Only(ctx)
	if err != nil {
		t.Fatalf("load admin wallet api: %v", err)
	}
	if row.Description != "customized by operator" {
		t.Fatalf("existing api row must be preserved, got %q", row.Description)
	}

	for _, route := range walletAdminRoutes() {
		exists, err := client.Api.Query().
			Where(apiEnt.Path(route[1]), apiEnt.Method(route[0])).
			Exist(ctx)
		if err != nil {
			t.Fatalf("query %s %s: %v", route[0], route[1], err)
		}
		if !exists {
			t.Fatalf("missing admin route %s %s after seeding", route[0], route[1])
		}
	}
}
