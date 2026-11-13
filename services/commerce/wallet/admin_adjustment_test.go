package wallet

import (
	"math"
	"testing"
)

// TestAdminAdjustmentSignedDelta 冻结管理员调账的有符号金额规则：
// 正数加款、负数扣款，0 与 MinInt64 属于非法输入。
func TestAdminAdjustmentSignedDelta(t *testing.T) {
	for _, amount := range []int64{1, -1, math.MaxInt64, math.MinInt64 + 1} {
		got, err := signedDelta(txTypeAdminAdjustment, amount)
		if err != nil || got != amount {
			t.Fatalf("amount=%d got=%d err=%v", amount, got, err)
		}
	}
	for _, amount := range []int64{0, math.MinInt64} {
		if _, err := signedDelta(txTypeAdminAdjustment, amount); err == nil {
			t.Fatalf("accepted invalid amount %d", amount)
		}
	}
}

// TestAdminAdjustmentReplayPayload 冻结管理员调账的幂等重放必须逐字段一致：
// 金额、描述、操作人任一变化都是 IDEMPOTENCY_CONFLICT，而不是被当成重放吞掉。
func TestAdminAdjustmentReplayPayload(t *testing.T) {
	cmd := balanceCommand{
		UserID:          7001,
		Currency:        currencyCNY,
		TransactionType: txTypeAdminAdjustment,
		Amount:          -500,
		OwnerNamespace:  "test",
		BusinessType:    businessTypeAdminAdjustment,
		BusinessID:      "adjust-1",
		Description:     "correction",
		Actor:           "admin:42",
	}
	old := transactionRecord{
		WalletID:    10,
		Currency:    currencyCNY,
		Type:        txTypeAdminAdjustment,
		DeltaAmount: -500,
		Description: "correction",
		Actor:       "admin:42",
	}
	if !replayMatches(old, 10, cmd) {
		t.Fatal("exact replay rejected")
	}
	for _, change := range []func(*balanceCommand){
		func(c *balanceCommand) { c.Amount = 500 },
		func(c *balanceCommand) { c.Description = "other" },
		func(c *balanceCommand) { c.Actor = "admin:43" },
	} {
		changed := cmd
		change(&changed)
		if replayMatches(old, 10, changed) {
			t.Fatal("conflicting replay accepted")
		}
	}
	if replayMatches(old, 11, cmd) {
		t.Fatal("different wallet accepted")
	}
}

// TestAdminAdjustmentCommandValidation 覆盖新类型的业务键、原因与操作人规则，
// 同时确认旧类型的正金额约束没有被放宽。
func TestAdminAdjustmentCommandValidation(t *testing.T) {
	valid := func() balanceCommand {
		return balanceCommand{
			UserID:          7001,
			Currency:        currencyCNY,
			TransactionType: txTypeAdminAdjustment,
			Amount:          -500,
			OwnerNamespace:  "test",
			BusinessType:    businessTypeAdminAdjustment,
			BusinessID:      "adjust-1",
			Description:     "correction",
			Actor:           "admin:42",
		}
	}
	if err := validateBalanceCommand(valid()); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*balanceCommand)
	}{
		{"wrong business type", func(c *balanceCommand) { c.BusinessType = "manual" }},
		{"empty description", func(c *balanceCommand) { c.Description = "" }},
		{"too long description", func(c *balanceCommand) { c.Description = string(make([]byte, maxDescriptionLen+1)) }},
		{"actor without prefix", func(c *balanceCommand) { c.Actor = "42" }},
		{"actor with invalid id", func(c *balanceCommand) { c.Actor = "admin:0" }},
		{"empty actor", func(c *balanceCommand) { c.Actor = "" }},
		{"related transaction set", func(c *balanceCommand) { id := uint64(9); c.RelatedTransactionID = &id }},
		{"zero amount", func(c *balanceCommand) { c.Amount = 0 }},
		{"min int64 amount", func(c *balanceCommand) { c.Amount = math.MinInt64 }},
		{"non CNY currency", func(c *balanceCommand) { c.Currency = "USD" }},
	}
	for _, tc := range cases {
		cmd := valid()
		tc.mutate(&cmd)
		if err := validateBalanceCommand(cmd); err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
	}

	legacy := valid()
	legacy.TransactionType = txTypeBalanceDebit
	legacy.BusinessType = "recharge"
	legacy.Actor = actorUser
	legacy.Amount = 500
	if err := validateBalanceCommand(legacy); err != nil {
		t.Fatalf("legacy debit rejected: %v", err)
	}
	legacy.Amount = -1
	if err := validateBalanceCommand(legacy); err == nil {
		t.Fatal("legacy debit accepted a negative amount")
	}
}
