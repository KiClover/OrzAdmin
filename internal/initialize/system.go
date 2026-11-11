package initialize

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/ent"
	apiEnt "github.com/KiClover/orzAdmin/ent/api"
	"github.com/KiClover/orzAdmin/ent/migrate"
	roleEnt "github.com/KiClover/orzAdmin/ent/role"
	settingEnt "github.com/KiClover/orzAdmin/ent/setting"
	userEnt "github.com/KiClover/orzAdmin/ent/user"
	internallog "github.com/KiClover/orzAdmin/internal/log"
	"github.com/KiClover/orzAdmin/pkg/utils"
	"github.com/casbin/casbin/v2"
	entadapter "github.com/casbin/ent-adapter"
	casbinEnt "github.com/casbin/ent-adapter/ent"
)

// logger 让初始化日志统一带 [INITIALIZE] 前缀。
var logger = internallog.WithModule(internallog.ModuleSetting)

// walletTables 是钱包业务四表；存在其中任意一张即视为已有钱包数据。
var walletTables = []string{
	"wallet_accounts",
	"wallet_transactions",
	"wallet_recharges",
	"redemption_codes",
}

// Initializer 执行按服务的表结构同步与种子补齐。
//
// 它按需创建数据库连接：只有真正执行某个目标时才连接对应数据库，
// 因此通用的 API 装配阶段不会提前创建任何表。
type Initializer struct {
	cfg *conf.Config

	// 便于测试替换；生产实现走 conf。
	openEnt      func(databaseConf conf.DatabaseConf) (*ent.Client, *sql.DB, error)
	openCasbin   func(cfg *conf.Config) (*casbin.Enforcer, func() error, error)
	walletExists func(ctx context.Context, db *sql.DB) (bool, error)

	mu       sync.Mutex
	client   *ent.Client
	db       *sql.DB
	enforcer *casbin.Enforcer
	casbinDB func() error
}

// NewInitializer 创建初始化器。此时不连接任何数据库。
func NewInitializer(cfg *conf.Config) *Initializer {
	i := &Initializer{cfg: cfg}
	i.openEnt = openEntClient
	i.openCasbin = openCasbinEnforcer
	i.walletExists = walletDataPresent
	return i
}

// Close 释放初始化器持有的数据库连接。
func (i *Initializer) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	var errs []error
	if i.client != nil {
		if err := i.client.Close(); err != nil {
			errs = append(errs, err)
		}
		i.client = nil
	}
	if i.db != nil {
		if err := i.db.Close(); err != nil {
			errs = append(errs, err)
		}
		i.db = nil
	}
	if i.casbinDB != nil {
		if err := i.casbinDB(); err != nil {
			errs = append(errs, err)
		}
		i.casbinDB = nil
		i.enforcer = nil
	}
	return errors.Join(errs...)
}

// InitializeAll 先为全部服务同步表结构，再按依赖顺序补齐种子；失败即停止。
func (i *Initializer) InitializeAll(ctx context.Context) ([]ServiceResult, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	services := Services()
	results := make([]ServiceResult, 0, len(services)*2)

	// 阶段一：所有目标的表结构。
	for _, svc := range services {
		result, err := i.runStage(ctx, svc, StageSchema)
		results = append(results, result)
		if err != nil {
			results = append(results, skippedRest(services, svc.Name, StageSchema)...)
			return results, err
		}
	}

	// 阶段二：按依赖顺序补齐种子。
	for _, svc := range services {
		result, err := i.runStage(ctx, svc, StageSeed)
		results = append(results, result)
		if err != nil {
			results = append(results, skippedRest(services, svc.Name, StageSeed)...)
			return results, err
		}
	}
	return results, nil
}

// InitializeService 只初始化指定服务，不会顺带初始化其它服务。
func (i *Initializer) InitializeService(ctx context.Context, service string) (ServiceResult, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	svc, err := LookupService(service)
	if err != nil {
		return ServiceResult{Service: service, Status: StatusFailed, Message: "unknown service"}, err
	}
	if err := i.checkDependencies(ctx, svc); err != nil {
		return ServiceResult{Service: svc.Name, Stage: StageSchema, Status: StatusFailed, Message: err.Error()}, err
	}
	for _, stage := range []Stage{StageSchema, StageSeed} {
		result, err := i.runStage(ctx, svc, stage)
		if err != nil {
			return result, err
		}
		if stage == StageSeed {
			return result, nil
		}
	}
	return ServiceResult{Service: svc.Name, Status: StatusSuccess}, nil
}

// checkDependencies 校验依赖服务的表与关键数据已经存在。
func (i *Initializer) checkDependencies(ctx context.Context, svc ServiceInfo) error {
	if len(svc.DependsOn) == 0 {
		return nil
	}
	client, db, err := i.connection()
	if err != nil {
		return err
	}
	for _, dep := range svc.DependsOn {
		switch dep {
		case ServiceUser:
			ok, err := tableExists(ctx, db, migrate.UsersTable.Name)
			if err != nil {
				return fmt.Errorf("check %s dependency: %w", dep, err)
			}
			if !ok {
				return fmt.Errorf("%w: table %s is not initialized", ErrMissingDependency, ServiceUser)
			}
			exists, err := client.User.Query().Where(userEnt.Name(AdminName)).Exist(ctx)
			if err != nil {
				return fmt.Errorf("check %s dependency: %w", dep, err)
			}
			if !exists {
				return fmt.Errorf("%w: default administrator is not initialized", ErrMissingDependency)
			}
		default:
			ok, err := tableExists(ctx, db, tableNameOf(dep))
			if err != nil {
				return fmt.Errorf("check %s dependency: %w", dep, err)
			}
			if !ok {
				return fmt.Errorf("%w: service %s is not initialized", ErrMissingDependency, dep)
			}
		}
	}
	return nil
}

// runStage 执行一个服务的一个阶段。
func (i *Initializer) runStage(ctx context.Context, svc ServiceInfo, stage Stage) (ServiceResult, error) {
	result := ServiceResult{Service: svc.Name, Stage: stage, Status: StatusSuccess}
	var err error
	switch stage {
	case StageSchema:
		err = i.migrateService(ctx, svc.Name)
	case StageSeed:
		err = i.seedService(ctx, svc.Name)
	default:
		err = fmt.Errorf("unknown stage %q", stage)
	}
	if err != nil {
		result.Status = StatusFailed
		result.Message = safeMessage(err)
		logger.Errorf("initialize %s stage %s err: %v", svc.Name, stage, err)
		return result, err
	}
	logger.Infof("initialize %s stage %s done", svc.Name, stage)
	return result, nil
}

// migrateService 同步指定服务拥有的表。
func (i *Initializer) migrateService(ctx context.Context, service string) error {
	client, _, err := i.connection()
	if err != nil {
		return err
	}
	var tables []*schema.Table
	switch service {
	case ServiceUser:
		tables = []*schema.Table{migrate.UsersTable}
	case ServiceAccess:
		tables = []*schema.Table{migrate.ApisTable, migrate.RolesTable}
	case ServiceSetting:
		tables = []*schema.Table{migrate.SettingsTable}
	case ServiceMessage:
		tables = []*schema.Table{migrate.MsgsTable}
	case ServiceToken:
		tables = []*schema.Table{migrate.TokensTable}
	default:
		return fmt.Errorf("%w: %s", ErrUnknownService, service)
	}
	if err := migrate.Create(ctx, client.Schema, tables, schema.WithForeignKeys(false)); err != nil {
		return err
	}
	return nil
}

// seedService 补齐指定服务缺失的种子数据。
func (i *Initializer) seedService(ctx context.Context, service string) error {
	client, db, err := i.connection()
	if err != nil {
		return err
	}
	switch service {
	case ServiceUser:
		return i.seedAdminUser(ctx, client)
	case ServiceAccess:
		return i.seedAccess(ctx, client)
	case ServiceSetting:
		return i.seedSettings(ctx, client, db)
	case ServiceMessage, ServiceToken:
		// 只有表结构，没有默认数据。
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnknownService, service)
	}
}

// seedAdminUser 只在管理员不存在时创建；已存在时绝不重置密码、资料或角色。
func (i *Initializer) seedAdminUser(ctx context.Context, client *ent.Client) error {
	exists, err := client.User.Query().Where(userEnt.Name(AdminName)).Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	seed := AdminUserSeed()
	pwd, err := utils.HashPassword(seed.Password)
	if err != nil {
		return err
	}
	_, err = client.User.Create().
		SetName(seed.Name).
		SetPwd(pwd).
		SetDescription(seed.Description).
		SetMobile(seed.Mobile).
		SetEmail(seed.Email).
		SetStatus(seed.Status).
		SetGender(seed.Gender).
		Save(ctx)
	if err != nil {
		// 并发初始化时唯一键冲突说明别的实例刚创建完成，重读确认即可。
		if ent.IsConstraintError(err) {
			exists, qerr := client.User.Query().Where(userEnt.Name(AdminName)).Exist(ctx)
			if qerr != nil {
				return qerr
			}
			if exists {
				return nil
			}
		}
		return err
	}
	return nil
}

// seedAccess 逐条补齐路由、角色与默认 Casbin 策略。
func (i *Initializer) seedAccess(ctx context.Context, client *ent.Client) error {
	for _, seed := range APISeeds() {
		if err := ensureAPI(ctx, client, seed); err != nil {
			return err
		}
	}
	for _, seed := range RoleSeeds() {
		if err := ensureRole(ctx, client, seed); err != nil {
			return err
		}
	}

	enforcer, err := i.casbinEnforcer()
	if err != nil {
		return err
	}
	for _, policy := range DefaultPolicies() {
		if _, err := enforcer.AddPolicy(policy[0], policy[1], policy[2]); err != nil {
			return fmt.Errorf("add policy %v: %w", policy, err)
		}
	}

	admin, err := client.User.Query().Where(userEnt.Name(AdminName)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: default administrator is not initialized", ErrMissingDependency)
		}
		return err
	}
	if _, err := enforcer.AddRoleForUser(fmt.Sprintf("%d", admin.ID), "admin"); err != nil {
		return fmt.Errorf("bind administrator role: %w", err)
	}
	return nil
}

// seedSettings 逐条补齐设置；钱包配置需要额外的数据存在性判断。
func (i *Initializer) seedSettings(ctx context.Context, client *ent.Client, db *sql.DB) error {
	for _, seed := range SettingSeeds() {
		exists, err := client.Setting.Query().Where(settingEnt.Key(seed.Key)).Exist(ctx)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if seed.Key == "wallet_config" {
			present, err := i.walletExists(ctx, db)
			if err != nil {
				return fmt.Errorf("check wallet data before seeding wallet_config: %w", err)
			}
			if present {
				return errors.New("wallet_config is missing while wallet business data exists")
			}
		}
		_, err = client.Setting.Create().SetKey(seed.Key).SetValue(seed.Value).Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				// 并发插入：确认记录已存在即可。
				exists, qerr := client.Setting.Query().Where(settingEnt.Key(seed.Key)).Exist(ctx)
				if qerr != nil {
					return qerr
				}
				if exists {
					continue
				}
			}
			return err
		}
	}
	return nil
}

func ensureAPI(ctx context.Context, client *ent.Client, seed APISeed) error {
	exists, err := client.Api.Query().
		Where(apiEnt.Path(seed.Path), apiEnt.Method(seed.Method)).
		Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = client.Api.Create().
		SetPath(seed.Path).
		SetMethod(seed.Method).
		SetGroup(seed.Group).
		SetDescription(seed.Description).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			exists, qerr := client.Api.Query().
				Where(apiEnt.Path(seed.Path), apiEnt.Method(seed.Method)).
				Exist(ctx)
			if qerr != nil {
				return qerr
			}
			if exists {
				return nil
			}
		}
		return err
	}
	return nil
}

func ensureRole(ctx context.Context, client *ent.Client, seed RoleSeed) error {
	exists, err := client.Role.Query().Where(roleEnt.RoleCode(seed.Code)).Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = client.Role.Create().
		SetRoleCode(seed.Code).
		SetRoleName(seed.Name).
		SetRemark(seed.Remark).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			exists, qerr := client.Role.Query().Where(roleEnt.RoleCode(seed.Code)).Exist(ctx)
			if qerr != nil {
				return qerr
			}
			if exists {
				return nil
			}
		}
		return err
	}
	return nil
}

// connection 惰性创建主库连接，返回 Ent 客户端与底层 *sql.DB。
func (i *Initializer) connection() (*ent.Client, *sql.DB, error) {
	if i.client != nil && i.db != nil {
		return i.client, i.db, nil
	}
	client, db, err := i.openEnt(i.cfg.DatabaseConf)
	if err != nil {
		return nil, nil, err
	}
	i.client = client
	i.db = db
	return client, db, nil
}

// casbinEnforcer 惰性创建不带 Redis watcher 的初始化 Enforcer。
// 业务进程仍使用 instance.NewCasbin，并在启动时自行加载策略。
func (i *Initializer) casbinEnforcer() (*casbin.Enforcer, error) {
	if i.enforcer != nil {
		return i.enforcer, nil
	}
	enforcer, closer, err := i.openCasbin(i.cfg)
	if err != nil {
		return nil, err
	}
	i.enforcer = enforcer
	i.casbinDB = closer
	return enforcer, nil
}

// openEntClient 用配置创建一个可关闭的 Ent 客户端（失败时返回 error，不退出进程）。
func openEntClient(databaseConf conf.DatabaseConf) (*ent.Client, *sql.DB, error) {
	db, err := sql.Open(databaseConf.Type, databaseConf.GetDSN())
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("ping database: %w", err)
	}
	if databaseConf.MaxOpenConn > 0 {
		db.SetMaxOpenConns(databaseConf.MaxOpenConn)
	}
	return ent.NewClient(ent.Driver(entsql.OpenDB(databaseConf.Type, db))), db, nil
}

// openCasbinEnforcer 用 Casbin 库的独立 Ent 客户端构造 Enforcer，便于释放连接。
func openCasbinEnforcer(cfg *conf.Config) (*casbin.Enforcer, func() error, error) {
	casbinConf := cfg.CasbinDatabaseConf
	if casbinConf.Host == "" {
		casbinConf = cfg.DatabaseConf
	}
	db, err := sql.Open(casbinConf.Type, casbinConf.GetDSN())
	if err != nil {
		return nil, nil, fmt.Errorf("open casbin database: %w", err)
	}
	client := casbinEnt.NewClient(casbinEnt.Driver(entsql.OpenDB(casbinConf.Type, db)))
	adapter, err := entadapter.NewAdapterWithClient(client)
	if err != nil {
		client.Close()
		return nil, nil, fmt.Errorf("create casbin adapter: %w", err)
	}
	enforcer, err := casbin.NewEnforcer("etc/casbin.conf", adapter)
	if err != nil {
		client.Close()
		return nil, nil, fmt.Errorf("create casbin enforcer: %w", err)
	}
	return enforcer, client.Close, nil
}

// walletDataPresent 在钱包四表存在时检查是否有业务数据；表都不存在视为无数据。
func walletDataPresent(ctx context.Context, db *sql.DB) (bool, error) {
	for _, table := range walletTables {
		exists, err := tableExists(ctx, db, table)
		if err != nil {
			return false, err
		}
		if !exists {
			continue
		}
		rows, err := db.QueryContext(ctx, "SELECT 1 FROM `"+table+"` LIMIT 1")
		if err != nil {
			return false, err
		}
		found := rows.Next()
		rows.Close()
		if err := rows.Err(); err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

// tableExists 查询当前数据库是否已有该表。
func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	rows, err := db.QueryContext(
		ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?",
		table,
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// tableNameOf 返回依赖服务的主表名，用于存在性检查。
func tableNameOf(service string) string {
	switch service {
	case ServiceUser:
		return migrate.UsersTable.Name
	case ServiceAccess:
		return migrate.ApisTable.Name
	case ServiceSetting:
		return migrate.SettingsTable.Name
	case ServiceMessage:
		return migrate.MsgsTable.Name
	case ServiceToken:
		return migrate.TokensTable.Name
	default:
		return ""
	}
}

// skippedRest 为失败点之后的服务补 skipped 结果，便于调用方看清哪些没执行。
func skippedRest(services []ServiceInfo, after string, stage Stage) []ServiceResult {
	if after == "" {
		return nil
	}
	results := make([]ServiceResult, 0, len(services))
	skipping := false
	for _, svc := range services {
		if skipping {
			results = append(results, ServiceResult{
				Service: svc.Name,
				Stage:   stage,
				Status:  StatusSkipped,
				Message: "skipped after failure",
			})
			continue
		}
		if svc.Name == after {
			skipping = true
		}
	}
	return results
}

// safeMessage 返回可安全写入 HTTP 响应的错误文案（不含 DSN、SQL 细节）。
func safeMessage(err error) string {
	switch {
	case errors.Is(err, ErrUnknownService):
		return "unknown service"
	case errors.Is(err, ErrMissingDependency):
		return "missing dependency: initialize the required service first"
	default:
		return "initialization failed"
	}
}
