package db

import (
	"context"
	"fmt"

	"github.com/KiClover/orzAdmin/ent"
)

// WalletServiceDB 暴露钱包服务自己的四张业务表。
//
// 钱包只访问这四个 Ent 客户端，不读写其它服务的数据表，也不直接查询用户表。
type WalletServiceDB struct {
	Account        *ent.WalletAccountClient
	Transaction    *ent.WalletTransactionClient
	Recharge       *ent.WalletRechargeClient
	RedemptionCode *ent.RedemptionCodeClient
}

// NewCommerceWalletDB 用钱包服务自己的 Ent 实例构造数据访问集合。
func NewCommerceWalletDB(client *ent.Client) *WalletServiceDB {
	return &WalletServiceDB{
		Account:        client.WalletAccount,
		Transaction:    client.WalletTransaction,
		Recharge:       client.WalletRecharge,
		RedemptionCode: client.RedemptionCode,
	}
}

// HasBusinessData 检查四张钱包业务表中是否已有任何数据。
//
// 四张表都会真实查询一遍：任何一张表存在数据就返回 true，同时任何一次查询失败
// 都必须把错误返回给调用方，不能因为前面的表已有数据或为空就跳过后续检查，
// 否则运行时会把查询故障误判成已初始化或未初始化。
func (w *WalletServiceDB) HasBusinessData(ctx context.Context) (bool, error) {
	if w == nil || w.Account == nil || w.Transaction == nil || w.Recharge == nil || w.RedemptionCode == nil {
		return false, fmt.Errorf("wallet database is not configured")
	}
	checks := []struct {
		table string
		exist func(context.Context) (bool, error)
	}{
		{table: "wallet_accounts", exist: w.Account.Query().Exist},
		{table: "wallet_transactions", exist: w.Transaction.Query().Exist},
		{table: "wallet_recharges", exist: w.Recharge.Query().Exist},
		{table: "redemption_codes", exist: w.RedemptionCode.Query().Exist},
	}
	present := false
	for _, check := range checks {
		found, err := check.exist(ctx)
		if err != nil {
			return false, fmt.Errorf("check %s: %w", check.table, err)
		}
		if found {
			present = true
		}
	}
	return present, nil
}
