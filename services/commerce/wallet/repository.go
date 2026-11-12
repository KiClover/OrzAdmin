package wallet

import (
	"context"
	"fmt"

	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/walletaccount"
	"github.com/KiClover/orzAdmin/ent/walletrecharge"
	"github.com/KiClover/orzAdmin/ent/wallettransaction"
)

// walletRepository 收拢钱包资金表的全部写操作。
//
// 设计 §7 要求 Handler 不得直接写账户或流水：所有余额变更都经过
// applyBalanceChange -> walletRepository，顺序固定为「锁账户 -> 查业务键 -> 校验 -> 改余额 -> 记流水」。
type walletRepository struct {
	client *ent.Client
}

func newWalletRepository(client *ent.Client) *walletRepository {
	return &walletRepository{client: client}
}

// runInTx 在单个数据库事务里执行 fn；fn 返回错误时回滚并保持原错误。
func (r *walletRepository) runInTx(ctx context.Context, fn func(tx *ent.Tx) error) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// lockWallet 取得并锁定用户的 CNY 钱包；不存在时在同一事务内创建零余额钱包。
//
// 并发首笔资金可能同时创建同一 (user_id, currency)：唯一索引冲突说明另一个事务
// 已经建好，重新 SELECT ... FOR UPDATE 即可拿到同一行。
func (r *walletRepository) lockWallet(ctx context.Context, tx *ent.Tx, userID uint64) (*ent.WalletAccount, error) {
	account, err := tx.WalletAccount.Query().
		Where(walletaccount.UserID(userID), walletaccount.Currency(currencyCNY)).
		ForUpdate().
		Only(ctx)
	if err == nil {
		return account, nil
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}

	created, createErr := tx.WalletAccount.Create().
		SetUserID(userID).
		SetCurrency(currencyCNY).
		Save(ctx)
	if createErr == nil {
		return created, nil
	}
	if !ent.IsConstraintError(createErr) {
		return nil, createErr
	}

	return tx.WalletAccount.Query().
		Where(walletaccount.UserID(userID), walletaccount.Currency(currencyCNY)).
		ForUpdate().
		Only(ctx)
}

// lockWalletByID 按钱包 ID 取得行锁，用于已持有订单但需要同时锁定钱包的场景（充值与退款）。
func (r *walletRepository) lockWalletByID(ctx context.Context, tx *ent.Tx, walletID uint64) (*ent.WalletAccount, error) {
	return tx.WalletAccount.Query().
		Where(walletaccount.ID(walletID)).
		ForUpdate().
		Only(ctx)
}

// findTransaction 按不可变的业务键回查已有交易；不存在返回 (nil, nil)。
func (r *walletRepository) findTransaction(ctx context.Context, tx *ent.Tx, cmd balanceCommand) (*ent.WalletTransaction, error) {
	transaction, err := tx.WalletTransaction.Query().
		Where(
			wallettransaction.OwnerNamespace(cmd.OwnerNamespace),
			wallettransaction.Type(cmd.TransactionType),
			wallettransaction.BusinessType(cmd.BusinessType),
			wallettransaction.BusinessID(cmd.BusinessID),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return transaction, err
}

// debitBlocked 判断钱包是否存在 Provider 逆向事故标记（review_blocks_debit=true）。
// 设计 §7.1：只要存在该标记就拒绝新的余额消费，直到人工处理完成。
func (r *walletRepository) debitBlocked(ctx context.Context, tx *ent.Tx, walletID uint64) (bool, error) {
	return tx.WalletRecharge.Query().
		Where(walletrecharge.WalletID(walletID), walletrecharge.ReviewBlocksDebit(true)).
		Exist(ctx)
}

// applyBalance 更新余额并递增 revision；锁行保证同一钱包的并发更新串行。
func (r *walletRepository) applyBalance(ctx context.Context, tx *ent.Tx, account *ent.WalletAccount, newBalance int64) error {
	affected, err := tx.WalletAccount.Update().
		Where(walletaccount.ID(account.ID)).
		SetBalanceAmount(newBalance).
		AddRevision(1).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("wallet account %d was not updated", account.ID)
	}

	account.BalanceAmount = newBalance
	account.Revision++
	return nil
}

// insertTransaction 追加一条不可变流水；余额字段与增量必须由调用方用同一事务的结果生成。
func (r *walletRepository) insertTransaction(
	ctx context.Context,
	tx *ent.Tx,
	cmd balanceCommand,
	walletID uint64,
	delta int64,
	balanceAfter int64,
) (*ent.WalletTransaction, error) {
	builder := tx.WalletTransaction.Create().
		SetWalletID(walletID).
		SetType(cmd.TransactionType).
		SetCurrency(cmd.Currency).
		SetDeltaAmount(delta).
		SetBalanceAfter(balanceAfter).
		SetOwnerNamespace(cmd.OwnerNamespace).
		SetBusinessType(cmd.BusinessType).
		SetBusinessID(cmd.BusinessID).
		SetDescription(cmd.Description).
		SetActor(cmd.Actor)
	if cmd.RelatedTransactionID != nil {
		builder.SetRelatedTransactionID(*cmd.RelatedTransactionID)
	}
	return builder.Save(ctx)
}

// recordOf 抽取幂等比较所需的不可变字段。
func recordOf(transaction *ent.WalletTransaction) transactionRecord {
	return transactionRecord{
		WalletID:             transaction.WalletID,
		Currency:             transaction.Currency,
		Type:                 transaction.Type,
		DeltaAmount:          transaction.DeltaAmount,
		RelatedTransactionID: transaction.RelatedTransactionID,
	}
}
