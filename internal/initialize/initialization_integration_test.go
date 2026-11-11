//go:build integration

package initialize

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/migrate"
	userEnt "github.com/KiClover/orzAdmin/ent/user"
	"github.com/casbin/casbin/v2"
	entadapter "github.com/casbin/ent-adapter"
	casbinEnt "github.com/casbin/ent-adapter/ent"
	_ "github.com/go-sql-driver/mysql"
)

// testDSN 读取集成测试数据库连接串；未配置时跳过。
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("WALLET_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("未配置 WALLET_TEST_MYSQL_DSN，跳过初始化集成测试")
	}
	return dsn
}

// casbinModelPath 返回仓库内 Casbin 模型的绝对路径。
func casbinModelPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "etc", "casbin.conf"))
	if err != nil {
		t.Fatalf("解析 casbin 模型路径失败: %v", err)
	}
	return path
}

// newIntegrationInitializer 构造一个指向测试库的初始化器：
// 生产实现从 conf.DatabaseConf 拼 DSN，这里直接注入测试 DSN。
func newIntegrationInitializer(t *testing.T, dsn string) *Initializer {
	t.Helper()
	initr := NewInitializer(&conf.Config{})
	initr.openEnt = func(conf.DatabaseConf) (*ent.Client, *sql.DB, error) {
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			db.Close()
			return nil, nil, err
		}
		return ent.NewClient(ent.Driver(entsql.OpenDB("mysql", db))), db, nil
	}
	initr.openCasbin = func(*conf.Config) (*casbin.Enforcer, func() error, error) {
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, nil, err
		}
		client := casbinEnt.NewClient(casbinEnt.Driver(entsql.OpenDB("mysql", db)))
		adapter, err := entadapter.NewAdapterWithClient(client)
		if err != nil {
			client.Close()
			return nil, nil, err
		}
		enforcer, err := casbin.NewEnforcer(casbinModelPath(t), adapter)
		if err != nil {
			client.Close()
			return nil, nil, err
		}
		return enforcer, client.Close, nil
	}
	t.Cleanup(func() { _ = initr.Close() })
	return initr
}

// resetDatabase 删除本次测试涉及的全部表，保证从空库开始。
func resetDatabase(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	defer db.Close()

	tables := []string{
		migrate.UsersTable.Name, migrate.ApisTable.Name, migrate.RolesTable.Name,
		migrate.SettingsTable.Name, migrate.MsgsTable.Name, migrate.TokensTable.Name,
		"casbin_rule",
	}
	for _, table := range tables {
		if _, err := db.Exec("DROP TABLE IF EXISTS `" + table + "`"); err != nil {
			t.Fatalf("删除表 %s 失败: %v", table, err)
		}
	}
}

// openTestDB 打开测试库连接。
func openTestDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestInitializationMySQL 验证"全部初始化可重复执行、只补齐缺失内容"。
func TestInitializationMySQL(t *testing.T) {
	dsn := testDSN(t)
	resetDatabase(t, dsn)

	initr := newIntegrationInitializer(t, dsn)
	ctx := context.Background()

	first, err := initr.InitializeAll(ctx)
	if err != nil {
		t.Fatalf("首次初始化失败: %v", err)
	}
	if want := len(ServiceNames()) * 2; len(first) != want {
		t.Fatalf("结果条数 = %d，期望 %d", len(first), want)
	}
	for _, result := range first {
		if result.Status != StatusSuccess {
			t.Fatalf("首次初始化存在非成功结果: %+v", result)
		}
	}

	second, err := initr.InitializeAll(ctx)
	if err != nil {
		t.Fatalf("重复初始化应成功（幂等），实际失败: %v", err)
	}
	for _, result := range second {
		if result.Status != StatusSuccess {
			t.Fatalf("重复初始化存在非成功结果: %+v", result)
		}
	}

	client, db, err := initr.connection()
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}

	users, err := client.User.Query().Where(userEnt.Name(AdminName)).Count(ctx)
	if err != nil || users != 1 {
		t.Fatalf("默认管理员数量 = %d err=%v，期望 1", users, err)
	}
	apis, err := client.Api.Query().Count(ctx)
	if err != nil || apis != len(APISeeds()) {
		t.Fatalf("路由数量 = %d err=%v，期望 %d", apis, err, len(APISeeds()))
	}
	roles, err := client.Role.Query().Count(ctx)
	if err != nil || roles != len(RoleSeeds()) {
		t.Fatalf("角色数量 = %d err=%v，期望 %d", roles, err, len(RoleSeeds()))
	}
	settings, err := client.Setting.Query().Count(ctx)
	if err != nil || settings != len(SettingSeeds()) {
		t.Fatalf("设置数量 = %d err=%v，期望 %d", settings, err, len(SettingSeeds()))
	}

	// 后台接口不能出现重复的 (path, method)。
	rows, err := db.QueryContext(ctx,
		"SELECT path, method, COUNT(*) c FROM `"+migrate.ApisTable.Name+"` GROUP BY path, method HAVING c > 1")
	if err != nil {
		t.Fatalf("查询重复路由失败: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var path, method string
		var count int
		if err := rows.Scan(&path, &method, &count); err != nil {
			t.Fatalf("扫描重复路由失败: %v", err)
		}
		t.Fatalf("路由重复: %s %s x%d", method, path, count)
	}

	// 管理员角色绑定必须存在且只有一条。
	var bindings int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM casbin_rule WHERE ptype='g' AND v0=? AND v1='admin'",
		strconv.FormatUint(adminID(t, client), 10),
	).Scan(&bindings); err != nil {
		t.Fatalf("统计管理员角色绑定失败: %v", err)
	}
	if bindings != 1 {
		t.Fatalf("管理员角色绑定条数 = %d，期望 1", bindings)
	}
}

// TestInitializationServiceMySQL 验证单个服务只初始化自己，且缺少前置时报错。
func TestInitializationServiceMySQL(t *testing.T) {
	dsn := testDSN(t)
	resetDatabase(t, dsn)

	initr := newIntegrationInitializer(t, dsn)
	ctx := context.Background()

	// 用户表与管理员都不存在时，初始化 access 应被拒绝。
	if _, err := initr.InitializeService(ctx, ServiceAccess); !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("缺少前置时应返回 ErrMissingDependency，实际 %v", err)
	}

	if _, err := initr.InitializeService(ctx, ServiceUser); err != nil {
		t.Fatalf("初始化 system-user 失败: %v", err)
	}
	_, db, err := initr.connection()
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}
	if ok, err := tableExists(ctx, db, migrate.SettingsTable.Name); err != nil || ok {
		t.Fatalf("system-user 不应创建 settings 表: ok=%v err=%v", ok, err)
	}

	// 补齐用户后再初始化 access 应成功，且只建自己的表。
	if _, err := initr.InitializeService(ctx, ServiceAccess); err != nil {
		t.Fatalf("初始化 system-access 失败: %v", err)
	}
	if ok, err := tableExists(ctx, db, migrate.ApisTable.Name); err != nil || !ok {
		t.Fatalf("system-access 应创建 apis 表: ok=%v err=%v", ok, err)
	}
	if ok, err := tableExists(ctx, db, migrate.SettingsTable.Name); err != nil || ok {
		t.Fatalf("system-access 不应创建 settings 表: ok=%v err=%v", ok, err)
	}

	// 指定服务初始化 setting 时，wallet_config 应在无钱包业务数据时补齐。
	if _, err := initr.InitializeService(ctx, ServiceSetting); err != nil {
		t.Fatalf("初始化 system-setting 失败: %v", err)
	}
	var walletValue string
	if err := db.QueryRowContext(ctx,
		"SELECT value FROM `"+migrate.SettingsTable.Name+"` WHERE `key`='wallet_config'",
	).Scan(&walletValue); err != nil {
		t.Fatalf("读取 wallet_config 失败: %v", err)
	}
	if walletValue == "" {
		t.Fatalf("wallet_config 值不应为空")
	}

	// 已有钱包业务数据时，配置缺失必须报错，不能生成关闭态覆盖现场。
	if _, err := db.ExecContext(ctx, "CREATE TABLE wallet_accounts (id BIGINT PRIMARY KEY)"); err != nil {
		t.Fatalf("准备钱包表失败: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO wallet_accounts (id) VALUES (1)"); err != nil {
		t.Fatalf("准备钱包数据失败: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM `"+migrate.SettingsTable.Name+"` WHERE `key`='wallet_config'"); err != nil {
		t.Fatalf("删除 wallet_config 失败: %v", err)
	}
	if _, err := initr.InitializeService(ctx, ServiceSetting); err == nil {
		t.Fatalf("钱包数据存在而配置缺失时应报错，实际成功")
	}
	if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS wallet_accounts"); err != nil {
		t.Fatalf("清理钱包表失败: %v", err)
	}
}

// TestTableHelpersMySQL 覆盖 tableExists 与 walletDataPresent 的基本行为。
func TestTableHelpersMySQL(t *testing.T) {
	dsn := testDSN(t)
	db := openTestDB(t, dsn)
	ctx := context.Background()

	ok, err := tableExists(ctx, db, "definitely_missing_table_xyz")
	if err != nil {
		t.Fatalf("tableExists 失败: %v", err)
	}
	if ok {
		t.Fatalf("不存在的表不应返回 true")
	}

	present, err := walletDataPresent(ctx, db)
	if err != nil {
		t.Fatalf("walletDataPresent 失败: %v", err)
	}
	if present {
		t.Fatalf("空库不应判定存在钱包数据")
	}
}

// adminID 返回默认管理员的数值 ID。
func adminID(t *testing.T, client *ent.Client) uint64 {
	t.Helper()
	admin, err := client.User.Query().Where(userEnt.Name(AdminName)).Only(context.Background())
	if err != nil {
		t.Fatalf("查询默认管理员失败: %v", err)
	}
	return admin.ID
}
