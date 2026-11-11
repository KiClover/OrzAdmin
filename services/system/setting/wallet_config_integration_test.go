//go:build integration

package setting

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"
	_ "github.com/go-sql-driver/mysql"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/migrate"
	settingEnt "github.com/KiClover/orzAdmin/ent/setting"
	settingrpc "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/cloudwego/kitex/pkg/kerrors"
)

// 集成测试需要真实 MySQL：WALLET_TEST_MYSQL_DSN，例如
// user:pass@tcp(127.0.0.1:3306)/orz_admin_test?parseTime=True
func integrationSettingService(t *testing.T) (*SystemSettingService, *ent.Client, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("WALLET_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("WALLET_TEST_MYSQL_DSN 未设置，跳过钱包配置集成测试")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	db.SetMaxOpenConns(8)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatalf("ping mysql: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB("mysql", db)))
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})
	svc := &SystemSettingService{
		DBAll: client,
		Cfg:   &conf.Config{},
	}
	return svc, client, db
}

func dropSettingsTable(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), "DROP TABLE IF EXISTS settings"); err != nil {
		t.Fatalf("drop settings: %v", err)
	}
}

func createSettingsTable(t *testing.T, client *ent.Client) {
	t.Helper()
	err := migrate.Create(context.Background(), client.Schema,
		[]*schema.Table{migrate.SettingsTable}, schema.WithForeignKeys(false))
	if err != nil {
		t.Fatalf("create settings table: %v", err)
	}
}

func TestWalletConfigMySQLConcurrentCAS(t *testing.T) {
	svc, client, db := integrationSettingService(t)
	dropSettingsTable(t, db)
	createSettingsTable(t, client)
	ctx := context.Background()

	// 写入关闭态默认配置，revision = 0。
	if _, err := client.Setting.Create().
		SetKey(WalletConfigKey).
		SetValue(string(WalletDefaultConfigJSON())).
		Save(ctx); err != nil {
		t.Fatalf("seed wallet_config: %v", err)
	}

	const workers = 4
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		success  int
		conflict int
	)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := svc.updateWalletSettings(ctx, &settingrpc.UpdateWalletSettingsReq{
				ExpectedRevision: 0,
				OperatorId:       "admin-" + strconv.Itoa(i),
				CnyPerUsdRate:    ratePtr(i),
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				success++
				return
			}
			var biz *kerrors.BizStatusError
			if errors.As(err, &biz) && biz.BizStatusCode() == WalletSettingCodeRevisionConflict {
				conflict++
				return
			}
			t.Errorf("worker %d 意外错误: %v", i, err)
		}(i)
	}
	wg.Wait()

	if success != 1 {
		t.Fatalf("并发 CAS 成功数 = %d，期望恰好 1", success)
	}
	if conflict != workers-1 {
		t.Fatalf("并发 CAS 冲突数 = %d，期望 %d", conflict, workers-1)
	}

	row, err := client.Setting.Query().Where(settingEnt.Key(WalletConfigKey)).Only(ctx)
	if err != nil {
		t.Fatalf("read wallet_config: %v", err)
	}
	snapshot, err := decodeWalletSnapshot([]byte(row.Value))
	if err != nil {
		t.Fatalf("decode wallet_config: %v", err)
	}
	if snapshot.Revision != 1 {
		t.Fatalf("revision = %d，期望 1", snapshot.Revision)
	}
}

func TestWalletConfigMySQLMaskingRoundTrip(t *testing.T) {
	svc, client, db := integrationSettingService(t)
	dropSettingsTable(t, db)
	createSettingsTable(t, client)
	ctx := context.Background()

	if _, err := client.Setting.Create().
		SetKey(WalletConfigKey).
		SetValue(string(WalletDefaultConfigJSON())).
		Save(ctx); err != nil {
		t.Fatalf("seed wallet_config: %v", err)
	}

	const secret = "{{SECRET_m0b33l81}}"
	const apiKey = "{{SECRET_p9x7kq2r}}"
	_, err := svc.updateWalletSettings(ctx, &settingrpc.UpdateWalletSettingsReq{
		ExpectedRevision: 0,
		OperatorId:       "admin-1",
		Enabled:          boolPtr(true),
		RechargeMode:     strPtr(RechargeModeTest),
		Profiles: []*settingrpc.WalletProviderProfilePatch{{
			Environment: ProfileEnvironmentTest,
			ProductId:   strPtr("prod_test_1"),
			WebhookId:   strPtr("wh_test_1"),
			ApiKey:      strPtr(apiKey),
			WebhookSecrets: []*settingrpc.WalletWebhookSecretPatch{{
				Secret:     secret,
				EndpointId: "ep_test_1",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("update wallet settings: %v", err)
	}

	masked, err := svc.getWalletSettingsMasked(ctx)
	if err != nil {
		t.Fatalf("get masked: %v", err)
	}
	profile := masked.TestProfile
	if profile == nil {
		t.Fatal("masked TEST profile 缺失")
	}
	if profile.ApiKeyMasked != walletSecretMasked {
		t.Fatalf("api key 掩码 = %q，期望 %q", profile.ApiKeyMasked, walletSecretMasked)
	}
	if !profile.ApiKeyConfigured || profile.ApiKeyFingerprint != FingerprintSecret(apiKey) {
		t.Fatal("api key 的 configured/fingerprint 不正确")
	}
	if len(profile.WebhookSecrets) != 1 {
		t.Fatalf("webhook secrets 数量 = %d", len(profile.WebhookSecrets))
	}
	if profile.WebhookSecrets[0].SecretMasked != walletSecretMasked {
		t.Fatalf("webhook secret 掩码 = %q", profile.WebhookSecrets[0].SecretMasked)
	}
	if profile.WebhookSecrets[0].Fingerprint != FingerprintSecret(secret) {
		t.Fatal("webhook secret fingerprint 不正确")
	}

	runtime, err := svc.getWalletRuntimeSettings(ctx)
	if err != nil {
		t.Fatalf("get runtime: %v", err)
	}
	if runtime.TestProfile == nil || runtime.TestProfile.ApiKey != apiKey {
		t.Fatal("运行时视图应携带完整 api key")
	}
	if len(runtime.TestProfile.WebhookSecrets) != 1 ||
		runtime.TestProfile.WebhookSecrets[0].Secret != secret {
		t.Fatal("运行时视图应携带完整 webhook secret")
	}
	if runtime.CnyPerUsdNumerator != 36 || runtime.CnyPerUsdDenominator != 5 {
		t.Fatalf("固定汇率分子/分母 = %d/%d，期望 36/5",
			runtime.CnyPerUsdNumerator, runtime.CnyPerUsdDenominator)
	}

	// 省略 Secret 的更新必须保留旧 Secret。
	_, err = svc.updateWalletSettings(ctx, &settingrpc.UpdateWalletSettingsReq{
		ExpectedRevision: 1,
		OperatorId:       "admin-1",
		Profiles: []*settingrpc.WalletProviderProfilePatch{{
			Environment: ProfileEnvironmentTest,
			ProductId:   strPtr("prod_test_1"),
			WebhookId:   strPtr("wh_test_1"),
		}},
	})
	if err != nil {
		t.Fatalf("第二次更新: %v", err)
	}
	runtime, err = svc.getWalletRuntimeSettings(ctx)
	if err != nil {
		t.Fatalf("get runtime again: %v", err)
	}
	if runtime.TestProfile.ApiKey != apiKey ||
		runtime.TestProfile.WebhookSecrets[0].Secret != secret {
		t.Fatal("省略 Secret 的更新不应清空已保存的 Secret")
	}
	if runtime.TestProfile.ProfileRevision != 2 {
		t.Fatalf("profile revision = %d，期望 2", runtime.TestProfile.ProfileRevision)
	}
}

// settings.value 在旧库里是 VARCHAR(255)，Ent 自动初始化必须把它扩成 TEXT 及以上的文本类型，
// 否则 64 KiB 的钱包配置会被截断或写不进去。
func TestWalletConfigMySQLSettingsValueIsText(t *testing.T) {
	_, client, db := integrationSettingService(t)
	dropSettingsTable(t, db)
	ctx := context.Background()

	legacy := "CREATE TABLE settings (" +
		"id BIGINT NOT NULL AUTO_INCREMENT," +
		"`key` VARCHAR(255) NOT NULL," +
		"value VARCHAR(255) NOT NULL," +
		"PRIMARY KEY (id)," +
		"UNIQUE KEY setting_key (`key`)" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
	if _, err := db.ExecContext(ctx, legacy); err != nil {
		t.Fatalf("create legacy settings: %v", err)
	}

	createSettingsTable(t, client)

	var dataType string
	err := db.QueryRowContext(ctx,
		"SELECT DATA_TYPE FROM information_schema.COLUMNS "+
			"WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'settings' AND COLUMN_NAME = 'value'",
	).Scan(&dataType)
	if err != nil {
		t.Fatalf("inspect value type: %v", err)
	}
	switch dataType {
	case "text", "mediumtext", "longtext":
	default:
		t.Fatalf("settings.value 类型 = %q，期望 text/mediumtext/longtext", dataType)
	}

	// 30 KiB 配置必须能写入（VARCHAR(255) 下会被截断或报错）。
	big := make([]byte, 30<<10)
	for i := range big {
		big[i] = 'a'
	}
	row, err := client.Setting.Create().
		SetKey("wallet_config_large").
		SetValue(string(big)).
		Save(ctx)
	if err != nil {
		t.Fatalf("write large value: %v", err)
	}
	if len(row.Value) != len(big) {
		t.Fatalf("写入长度 = %d，期望 %d", len(row.Value), len(big))
	}
	readBack, err := client.Setting.Query().Where(settingEnt.Key("wallet_config_large")).Only(ctx)
	if err != nil {
		t.Fatalf("read large value: %v", err)
	}
	if len(readBack.Value) != len(big) {
		t.Fatalf("读回长度 = %d，期望 %d", len(readBack.Value), len(big))
	}
}

func boolPtr(v bool) *bool    { return &v }
func strPtr(v string) *string { return &v }

// ratePtr 返回 0.100000..100.000000 范围内互不相同的合法汇率。
func ratePtr(i int) *string {
	v := []string{"7.100000", "7.200000", "7.300000", "7.400000"}[i%4]
	return &v
}
