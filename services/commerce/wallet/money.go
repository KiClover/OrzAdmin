package wallet

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/cloudwego/kitex/pkg/kerrors"
)

// 本文件收拢金额、ID、币种与错误码的纯函数，供余额变更和查询共用。
// 这里不访问数据库，也不读取运行时快照，便于单元测试覆盖边界。

const (
	// 交易类型在数据库中的稳定取值，与设计 §6.2 的枚举文本一致。
	txTypeRechargeCredit   = "RECHARGE_CREDIT"
	txTypeBalanceDebit     = "BALANCE_DEBIT"
	txTypeBalanceRefund    = "BALANCE_REFUND"
	txTypeRechargeRefund   = "RECHARGE_REFUND"
	txTypeRedemptionCredit = "REDEMPTION_CREDIT"

	// 业务键与描述的长度上限，与 ent/schema 中的 varchar 长度保持一致。
	maxBusinessTypeLen  = 64
	maxBusinessIDLen    = 128
	maxDescriptionLen   = 512
	maxOwnerNamespaceLn = 64
	maxDecimalIDLength  = 20

	// actorUser 表示由终端用户经 Finance API 发起的资金动作。
	actorUser = "user"
)

// walletBizError 把 WalletErrorCode 映射为 Kitex 业务错误，HTTP/API 层据此稳定映射状态码。
func walletBizError(code wallet.WalletErrorCode, message string) error {
	return kerrors.NewBizStatusError(int32(code), message)
}

func invalidArgument(message string) error {
	return walletBizError(wallet.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT, message)
}

func walletNotFound(message string) error {
	return walletBizError(wallet.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND, message)
}

func idempotencyConflict() error {
	return walletBizError(wallet.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT, "business key was already used with a different payload")
}

func stateConflict(message string) error {
	return walletBizError(wallet.WalletErrorCode_WALLET_ERROR_CODE_STATE_CONFLICT, message)
}

func insufficientBalance() error {
	return walletBizError(wallet.WalletErrorCode_WALLET_ERROR_CODE_INSUFFICIENT_BALANCE, "wallet balance is insufficient")
}

func balanceBlocked() error {
	return walletBizError(wallet.WalletErrorCode_WALLET_ERROR_CODE_BALANCE_BLOCKED, "wallet balance is blocked by a provider reversal")
}

func walletUnavailable(message string) error {
	return walletBizError(wallet.WalletErrorCode_WALLET_ERROR_CODE_UNAVAILABLE, message)
}

func walletInternal(message string) error {
	return walletBizError(wallet.WalletErrorCode_WALLET_ERROR_CODE_INTERNAL, message)
}

func featureDisabled(reason string) error {
	return walletBizError(wallet.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED, reason)
}

// parseDecimalID 解析对外暴露的十进制正整数 ID：不接受空值、空白、符号、前导零与超长输入。
func parseDecimalID(field, raw string) (uint64, error) {
	value := raw
	if value == "" {
		return 0, invalidArgument(field + " is required")
	}
	if len(value) > maxDecimalIDLength {
		return 0, invalidArgument(field + " is invalid")
	}
	if value[0] == '0' {
		return 0, invalidArgument(field + " is invalid")
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, invalidArgument(field + " is invalid")
		}
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 {
		return 0, invalidArgument(field + " is invalid")
	}
	return id, nil
}

// formatDecimalID 是对外 ID 的唯一格式：无符号十进制字符串。
func formatDecimalID(id uint64) string {
	return strconv.FormatUint(id, 10)
}

// normalizeWalletCurrency 只接受 CNY；省略币种按 CNY 处理（钱包币种由设计冻结）。
func normalizeWalletCurrency(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", currencyCNY:
		return currencyCNY, nil
	default:
		return "", invalidArgument("currency must be " + currencyCNY)
	}
}

// transactionTypeName 把 Proto 枚举映射为数据库中的稳定文本。
func transactionTypeName(t wallet.TransactionType) (string, error) {
	switch t {
	case wallet.TransactionType_TRANSACTION_TYPE_RECHARGE_CREDIT:
		return txTypeRechargeCredit, nil
	case wallet.TransactionType_TRANSACTION_TYPE_BALANCE_DEBIT:
		return txTypeBalanceDebit, nil
	case wallet.TransactionType_TRANSACTION_TYPE_BALANCE_REFUND:
		return txTypeBalanceRefund, nil
	case wallet.TransactionType_TRANSACTION_TYPE_RECHARGE_REFUND:
		return txTypeRechargeRefund, nil
	case wallet.TransactionType_TRANSACTION_TYPE_REDEMPTION_CREDIT:
		return txTypeRedemptionCredit, nil
	default:
		return "", invalidArgument("transaction type is not supported")
	}
}

// transactionTypeFromName 把数据库文本还原为 Proto 枚举；未知文本保留为 UNSPECIFIED，
// 只用于视图展示，不参与资金判断。
func transactionTypeFromName(name string) wallet.TransactionType {
	switch name {
	case txTypeRechargeCredit:
		return wallet.TransactionType_TRANSACTION_TYPE_RECHARGE_CREDIT
	case txTypeBalanceDebit:
		return wallet.TransactionType_TRANSACTION_TYPE_BALANCE_DEBIT
	case txTypeBalanceRefund:
		return wallet.TransactionType_TRANSACTION_TYPE_BALANCE_REFUND
	case txTypeRechargeRefund:
		return wallet.TransactionType_TRANSACTION_TYPE_RECHARGE_REFUND
	case txTypeRedemptionCredit:
		return wallet.TransactionType_TRANSACTION_TYPE_REDEMPTION_CREDIT
	default:
		return wallet.TransactionType_TRANSACTION_TYPE_UNSPECIFIED
	}
}

// walletStatusFor 由余额符号派生状态：负数即欠款，其余为正常。
func walletStatusFor(balance int64) wallet.WalletStatus {
	if balance < 0 {
		return wallet.WalletStatus_WALLET_STATUS_DEBT
	}
	return wallet.WalletStatus_WALLET_STATUS_ACTIVE
}

// signedDelta 计算一笔正向金额对应的有符号流水增量。
// BALANCE_DEBIT 与 RECHARGE_REFUND 减少余额，其余类型增加余额。
func signedDelta(transactionType string, amount int64) (int64, error) {
	if amount <= 0 {
		return 0, invalidArgument("amount must be positive")
	}
	switch transactionType {
	case txTypeBalanceDebit, txTypeRechargeRefund:
		return -amount, nil
	case txTypeRechargeCredit, txTypeBalanceRefund, txTypeRedemptionCredit:
		return amount, nil
	default:
		return 0, invalidArgument("transaction type is not supported")
	}
}

// addAmount 对余额做带溢出检查的加减，溢出返回 false 而不是回绕。
func addAmount(balance, delta int64) (int64, bool) {
	sum := balance + delta
	if (delta > 0 && sum < balance) || (delta < 0 && sum > balance) {
		return 0, false
	}
	return sum, true
}

// replayMatches 判断已存在的交易是否与本次命令完全等价（幂等重放）。
// 同一业务键只允许完全一致的重放；钱包、金额、币种或关联交易任一不同都视为冲突。
func replayMatches(existing transactionRecord, walletID uint64, cmd balanceCommand) bool {
	return existing.WalletID == walletID &&
		existing.Currency == cmd.Currency &&
		existing.Type == cmd.TransactionType &&
		absAmount(existing.DeltaAmount) == cmd.Amount &&
		sameOptionalID(existing.RelatedTransactionID, cmd.RelatedTransactionID)
}

// sameOptionalID 比较可空 ID 的值语义（两个独立指针指向同一数值也算一致）。
func sameOptionalID(a, b *uint64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// transactionRecord 是幂等比较需要的最小不可变字段集合。
type transactionRecord struct {
	WalletID             uint64
	Currency             string
	Type                 string
	DeltaAmount          int64
	RelatedTransactionID *uint64
}

func absAmount(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// validateBalanceCommand 校验资金命令的不可变输入，任何非法值都在开事务之前拒绝。
func validateBalanceCommand(cmd balanceCommand) error {
	if cmd.UserID == 0 {
		return invalidArgument("user_id is invalid")
	}
	if cmd.Currency != currencyCNY {
		return invalidArgument("currency must be " + currencyCNY)
	}
	if cmd.Amount <= 0 {
		return invalidArgument("amount must be positive")
	}
	if cmd.TransactionType == "" {
		return invalidArgument("transaction type is required")
	}
	switch cmd.TransactionType {
	case txTypeBalanceDebit, txTypeBalanceRefund, txTypeRechargeRefund:
	default:
		return invalidArgument("transaction type is not supported for balance changes")
	}
	if cmd.OwnerNamespace == "" || len(cmd.OwnerNamespace) > maxOwnerNamespaceLn {
		return invalidArgument("owner namespace is invalid")
	}
	if cmd.BusinessType == "" || len(cmd.BusinessType) > maxBusinessTypeLen {
		return invalidArgument("business_type is invalid")
	}
	if cmd.BusinessID == "" || len(cmd.BusinessID) > maxBusinessIDLen {
		return invalidArgument("business_id is invalid")
	}
	if len(cmd.Description) > maxDescriptionLen {
		return invalidArgument("description is too long")
	}
	requiresRelated := cmd.TransactionType == txTypeBalanceRefund || cmd.TransactionType == txTypeRechargeRefund
	if requiresRelated && cmd.RelatedTransactionID == nil {
		return invalidArgument("related_transaction_id is required")
	}
	if !requiresRelated && cmd.RelatedTransactionID != nil {
		return invalidArgument("related_transaction_id is not allowed")
	}
	return nil
}

// errNilTransaction 是内部防御：事务成功提交却没有任何交易结果时视为缺陷。
var errNilTransaction = errors.New("wallet: balance change produced no transaction")

// wrapInternal 隐藏数据库细节，只保留可安全返回的文案。
func wrapInternal(operation string, err error) error {
	var biz kerrors.BizStatusErrorIface
	if errors.As(err, &biz) {
		return err
	}
	return walletInternal(fmt.Sprintf("%s failed", operation))
}
