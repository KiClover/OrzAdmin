package wallet

import (
	"math"
	"testing"

	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// 这组测试冻结金额与业务键的纯函数语义：ID/币种解析、有符号增量、
// 溢出保护、幂等重放判定与命令校验。数据库行为在初始化/余额集成测试中覆盖。

func TestParseDecimalID(t *testing.T) {
	valid := map[string]uint64{"1": 1, "7001": 7001, "18446744073709551615": math.MaxUint64}
	for raw, want := range valid {
		got, err := parseDecimalID("user_id", raw)
		if err != nil {
			t.Fatalf("合法 ID %q 被拒绝: %v", raw, err)
		}
		if got != want {
			t.Fatalf("ID %q 解析为 %d，期望 %d", raw, got, want)
		}
	}

	invalid := []string{"", " ", " 12 ", "12 ", " 12", "0", "0001", "-1", "+1", "1.0", "1e3", "18446744073709551616", "abc"}
	for _, raw := range invalid {
		if _, err := parseDecimalID("user_id", raw); err == nil {
			t.Fatalf("非法 ID %q 应被拒绝", raw)
		}
	}
}

func TestFormatDecimalID(t *testing.T) {
	if got := formatDecimalID(7001); got != "7001" {
		t.Fatalf("ID 格式化为 %q，期望 7001", got)
	}
}

func TestNormalizeWalletCurrency(t *testing.T) {
	for _, raw := range []string{"", "CNY"} {
		got, err := normalizeWalletCurrency(raw)
		if err != nil || got != currencyCNY {
			t.Fatalf("币种 %q 应归一化为 CNY，实际 %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"cny", " cny ", "USD", "JPY", "cny1"} {
		if _, err := normalizeWalletCurrency(raw); err == nil {
			t.Fatalf("非 CNY 币种 %q 应被拒绝", raw)
		}
	}
}

func TestSignedDelta(t *testing.T) {
	cases := []struct {
		name      string
		txType    string
		amount    int64
		wantDelta int64
	}{
		{"充值入账", txTypeRechargeCredit, 1500, 1500},
		{"余额消费", txTypeBalanceDebit, 900, -900},
		{"余额补偿", txTypeBalanceRefund, 900, 900},
		{"充值退款", txTypeRechargeRefund, 1389, -1389},
		{"兑换码入账", txTypeRedemptionCredit, 2000, 2000},
	}
	for _, testCase := range cases {
		got, err := signedDelta(testCase.txType, testCase.amount)
		if err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}
		if got != testCase.wantDelta {
			t.Fatalf("%s: 增量 %d，期望 %d", testCase.name, got, testCase.wantDelta)
		}
	}

	if _, err := signedDelta(txTypeBalanceDebit, 0); err == nil {
		t.Fatal("零金额应被拒绝")
	}
	if _, err := signedDelta("UNKNOWN", 1); err == nil {
		t.Fatal("未知交易类型应被拒绝")
	}
}

func TestAddAmountRejectsOverflow(t *testing.T) {
	if got, ok := addAmount(math.MaxInt64-1, 1); !ok || got != math.MaxInt64 {
		t.Fatalf("正常加法失败: %d %v", got, ok)
	}
	if _, ok := addAmount(math.MaxInt64, 1); ok {
		t.Fatal("正溢出必须被拒绝")
	}
	if _, ok := addAmount(math.MinInt64, -1); ok {
		t.Fatal("负溢出必须被拒绝")
	}
	if got, ok := addAmount(-100, 100); !ok || got != 0 {
		t.Fatalf("归零加法失败: %d %v", got, ok)
	}
}

func TestWalletStatusFor(t *testing.T) {
	if got := walletStatusFor(0); got != wallet.WalletStatus_WALLET_STATUS_ACTIVE {
		t.Fatalf("零余额应为 ACTIVE，实际 %v", got)
	}
	if got := walletStatusFor(-1); got != wallet.WalletStatus_WALLET_STATUS_DEBT {
		t.Fatalf("负余额应为 DEBT，实际 %v", got)
	}
}

func TestTransactionTypeRoundTrip(t *testing.T) {
	for _, enum := range []wallet.TransactionType{
		wallet.TransactionType_TRANSACTION_TYPE_RECHARGE_CREDIT,
		wallet.TransactionType_TRANSACTION_TYPE_BALANCE_DEBIT,
		wallet.TransactionType_TRANSACTION_TYPE_BALANCE_REFUND,
		wallet.TransactionType_TRANSACTION_TYPE_RECHARGE_REFUND,
		wallet.TransactionType_TRANSACTION_TYPE_REDEMPTION_CREDIT,
		wallet.TransactionType_TRANSACTION_TYPE_ADMIN_ADJUSTMENT,
	} {
		name, err := transactionTypeName(enum)
		if err != nil {
			t.Fatalf("枚举 %v 应有名字: %v", enum, err)
		}
		if got := transactionTypeFromName(name); got != enum {
			t.Fatalf("%q 还原为 %v，期望 %v", name, got, enum)
		}
	}
	if _, err := transactionTypeName(wallet.TransactionType_TRANSACTION_TYPE_UNSPECIFIED); err == nil {
		t.Fatal("UNSPECIFIED 不应被当成查询条件")
	}
	if got := transactionTypeFromName("UNKNOWN"); got != wallet.TransactionType_TRANSACTION_TYPE_UNSPECIFIED {
		t.Fatalf("未知文本应还原为 UNSPECIFIED，实际 %v", got)
	}
}

func uint64Ptr(v uint64) *uint64 { return &v }

func TestReplayMatches(t *testing.T) {
	cmd := balanceCommand{
		UserID:               7001,
		Currency:             currencyCNY,
		TransactionType:      txTypeBalanceDebit,
		Amount:               900,
		OwnerNamespace:       testOwnerNamespace,
		BusinessType:         "order",
		BusinessID:           "A-1",
		RelatedTransactionID: nil,
	}

	existing := transactionRecord{
		WalletID:    11,
		Currency:    currencyCNY,
		Type:        txTypeBalanceDebit,
		DeltaAmount: -900,
	}
	if !replayMatches(existing, 11, cmd) {
		t.Fatal("完全一致的交易应判定为重放")
	}
	if replayMatches(existing, 12, cmd) {
		t.Fatal("钱包不一致不应判定为重放")
	}
	if replayMatches(existing, 11, balanceCommand{
		UserID: 7001, Currency: currencyCNY, TransactionType: txTypeBalanceDebit,
		Amount: 901, OwnerNamespace: testOwnerNamespace, BusinessType: "order", BusinessID: "A-1",
	}) {
		t.Fatal("金额不一致不应判定为重放")
	}

	refund := balanceCommand{
		UserID: 7001, Currency: currencyCNY, TransactionType: txTypeBalanceRefund,
		Amount: 900, OwnerNamespace: testOwnerNamespace, BusinessType: businessTypeBalanceRefund,
		BusinessID: "42", RelatedTransactionID: uint64Ptr(42),
	}
	refundRecord := transactionRecord{
		WalletID: 11, Currency: currencyCNY, Type: txTypeBalanceRefund,
		DeltaAmount: 900, RelatedTransactionID: uint64Ptr(42),
	}
	if !replayMatches(refundRecord, 11, refund) {
		t.Fatal("关联交易一致应判定为重放")
	}
	if replayMatches(refundRecord, 11, balanceCommand{
		UserID: 7001, Currency: currencyCNY, TransactionType: txTypeBalanceRefund,
		Amount: 900, OwnerNamespace: testOwnerNamespace, BusinessType: businessTypeBalanceRefund,
		BusinessID: "43", RelatedTransactionID: uint64Ptr(43),
	}) {
		t.Fatal("关联交易不一致不应判定为重放")
	}
}

func TestValidateBalanceCommand(t *testing.T) {
	base := balanceCommand{
		UserID:          7001,
		Currency:        currencyCNY,
		TransactionType: txTypeBalanceDebit,
		Amount:          100,
		OwnerNamespace:  testOwnerNamespace,
		BusinessType:    "order",
		BusinessID:      "A-1",
		Actor:           actorUser,
	}
	if err := validateBalanceCommand(base); err != nil {
		t.Fatalf("合法命令被拒绝: %v", err)
	}

	mutations := []func(cmd *balanceCommand){
		func(cmd *balanceCommand) { cmd.UserID = 0 },
		func(cmd *balanceCommand) { cmd.Currency = currencyUSD },
		func(cmd *balanceCommand) { cmd.Amount = 0 },
		func(cmd *balanceCommand) { cmd.TransactionType = "" },
		func(cmd *balanceCommand) { cmd.TransactionType = txTypeRechargeCredit },
		func(cmd *balanceCommand) { cmd.OwnerNamespace = "" },
		func(cmd *balanceCommand) { cmd.BusinessType = "" },
		func(cmd *balanceCommand) { cmd.BusinessID = "" },
		func(cmd *balanceCommand) { cmd.RelatedTransactionID = uint64Ptr(1) },
	}
	for index, mutate := range mutations {
		cmd := base
		mutate(&cmd)
		if err := validateBalanceCommand(cmd); err == nil {
			t.Fatalf("第 %d 个非法变体应被拒绝", index)
		}
	}

	refund := base
	refund.TransactionType = txTypeBalanceRefund
	if err := validateBalanceCommand(refund); err == nil {
		t.Fatal("余额补偿缺少关联交易应被拒绝")
	}
	refund.RelatedTransactionID = uint64Ptr(42)
	if err := validateBalanceCommand(refund); err != nil {
		t.Fatalf("合法补偿命令被拒绝: %v", err)
	}
}
