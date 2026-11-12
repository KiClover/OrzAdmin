package wallet

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzAdmin/ent/migrate"
	internallog "github.com/KiClover/orzAdmin/internal/log"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
)

// logger 让钱包服务的日志统一带 [WALLET] 前缀。
var logger = internallog.WithModule(internallog.ModuleWallet)

// walletTables 返回钱包服务在初始化阶段需要同步的四张业务表。
//
// 钱包只同步自己的表：不建 users，不建 Casbin 表，也不碰其它服务的表。
func walletTables() []*schema.Table {
	return []*schema.Table{
		migrate.WalletAccountsTable,
		migrate.WalletTransactionsTable,
		migrate.WalletRechargesTable,
		migrate.RedemptionCodesTable,
	}
}

// InitDatabase 是维护类 RPC：只把钱包自身的四张业务表同步到当前数据库。
//
// 它不创建钱包账户、不写入任何资金数据、不访问 Provider，重复执行只做
// CREATE TABLE IF NOT EXISTS 之类的幂等同步，已有数据保持不变。
func (s CommerceWalletService) InitDatabase(ctx context.Context, req *base.Empty) (res *base.BaseResp, err error) {
	if s.DBAll == nil {
		return nil, fmt.Errorf("wallet database client is not configured")
	}
	if err := migrate.Create(ctx, s.DBAll.Schema, walletTables(), schema.WithForeignKeys(false)); err != nil {
		logger.Errorf("wallet init database err: %v", err)
		return nil, err
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}
