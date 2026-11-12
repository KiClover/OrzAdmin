package wallet

import (
	"context"

	"github.com/KiClover/orzAdmin/ent"
)

// balanceCommand 描述一次资金变更的不可变输入。
//
// owner_namespace 与 actor 由服务端派生，客户端只能提交金额、币种与业务键。
type balanceCommand struct {
	UserID               uint64
	Currency             string
	TransactionType      string
	Amount               int64
	OwnerNamespace       string
	BusinessType         string
	BusinessID           string
	RelatedTransactionID *uint64
	Description          string
	Actor                string
}

// balanceChangeAttempts 是并发同业务键时的最大尝试次数：
// 第一次撞唯一索引说明另一事务已提交，第二次即可读到那条交易并判定重放或冲突。
const balanceChangeAttempts = 2

// balanceTypeBalanceRefund 是余额补偿在业务键中的稳定 business_type。
const businessTypeBalanceRefund = "balance_refund"

// applyBalanceChange 是钱包余额变更的唯一入口（设计 §7）。
//
// 事务顺序固定：锁钱包行 -> 按业务键回查 -> 幂等/冲突判定 -> 余额方向校验 ->
// 带溢出检查地更新余额与 revision -> 追加不可变流水 -> 提交。
// 已存在的完全一致交易直接返回，不会被重复记账。
func (s *CommerceWalletService) applyBalanceChange(ctx context.Context, cmd balanceCommand) (*ent.WalletTransaction, error) {
	if err := validateBalanceCommand(cmd); err != nil {
		return nil, err
	}

	repository := s.repository()

	var lastErr error
	for attempt := 0; attempt < balanceChangeAttempts; attempt++ {
		transaction, err := s.attemptBalanceChange(ctx, repository, cmd)
		if err == nil {
			return transaction, nil
		}
		lastErr = err
		// 只有唯一索引冲突值得重试：重试事务会读到赢家写入的交易。
		if !ent.IsConstraintError(err) {
			break
		}
	}

	return nil, wrapInternal("balance change", lastErr)
}

func (s *CommerceWalletService) attemptBalanceChange(
	ctx context.Context,
	repository *walletRepository,
	cmd balanceCommand,
) (*ent.WalletTransaction, error) {
	var result *ent.WalletTransaction

	err := repository.runInTx(ctx, func(tx *ent.Tx) error {
		account, err := repository.lockWallet(ctx, tx, cmd.UserID)
		if err != nil {
			return err
		}

		existing, err := repository.findTransaction(ctx, tx, cmd)
		if err != nil {
			return err
		}
		if existing != nil {
			if !replayMatches(recordOf(existing), account.ID, cmd) {
				return idempotencyConflict()
			}
			result = existing
			return nil
		}

		if err := validateBalanceDirection(ctx, repository, tx, account, cmd); err != nil {
			return err
		}

		delta, err := signedDelta(cmd.TransactionType, cmd.Amount)
		if err != nil {
			return err
		}
		newBalance, ok := addAmount(account.BalanceAmount, delta)
		if !ok {
			return walletInternal("wallet balance overflow")
		}

		if err := repository.applyBalance(ctx, tx, account, newBalance); err != nil {
			return err
		}

		transaction, err := repository.insertTransaction(ctx, tx, cmd, account.ID, delta, newBalance)
		if err != nil {
			return err
		}

		result = transaction
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errNilTransaction
	}
	return result, nil
}

// validateBalanceDirection 校验「是否允许朝这个方向改动余额」。
//
//   - 消费（BALANCE_DEBIT）：余额为负、余额不足或钱包被 Provider 逆向事故阻断时拒绝；
//   - 充值退款（RECHARGE_REFUND）：允许把余额扣成负数，但仍受事故阻断约束；
//   - 余额补偿（BALANCE_REFUND）：只增不减，不做余额下限检查。
func validateBalanceDirection(
	ctx context.Context,
	repository *walletRepository,
	tx *ent.Tx,
	account *ent.WalletAccount,
	cmd balanceCommand,
) error {
	blockedTypes := cmd.TransactionType == txTypeBalanceDebit || cmd.TransactionType == txTypeRechargeRefund
	if blockedTypes {
		blocked, err := repository.debitBlocked(ctx, tx, account.ID)
		if err != nil {
			return err
		}
		if blocked {
			return balanceBlocked()
		}
	}

	if cmd.TransactionType != txTypeBalanceDebit {
		return nil
	}
	if account.BalanceAmount < 0 || account.BalanceAmount < cmd.Amount {
		return insufficientBalance()
	}
	return nil
}

// repository 返回当前 handler 绑定的仓储；DBAll 由启动装配注入。
func (s *CommerceWalletService) repository() *walletRepository {
	return newWalletRepository(s.DBAll)
}
