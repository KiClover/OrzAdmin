package wallet

import (
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// 这组测试冻结查询层的稳定分页与视图隔离：游标编解码、分页上限、
// 时间窗校验，以及用户视图不暴露内部归属字段。

func TestCursorRoundTrip(t *testing.T) {
	createdAt := time.Date(2026, 11, 12, 9, 30, 15, 0, time.UTC)
	encoded := encodeCursor(createdAt, 42)
	if encoded == "" {
		t.Fatal("游标不应为空")
	}

	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if decoded.id != 42 {
		t.Fatalf("id 解码为 %d，期望 42", decoded.id)
	}
	if !decoded.createdAt.Equal(createdAt) {
		t.Fatalf("created_at 解码为 %v，期望 %v", decoded.createdAt, createdAt)
	}

	if c, err := decodeCursor(""); err != nil || !c.empty() {
		t.Fatalf("空游标应表示第一页: %v, %v", c, err)
	}
}

func TestDecodeCursorRejectsInvalid(t *testing.T) {
	invalid := []string{
		"!!!",
		"bm90LWEtY3Vyc29y", // "not-a-cursor"
		"MTIz",             // "123" 只有一段
		"OjEyMw",           // ":123" createdAt 非法
		"MTIzOjA",          // "123:0" id 非法
		"LTE6MTIz",         // "-1:123" 负数
		"MTIzOjEyMzo0NTY",  // "123:123:456" 三段
	}
	for _, raw := range invalid {
		if _, err := decodeCursor(raw); err == nil {
			t.Fatalf("非法游标 %q 应被拒绝", raw)
		}
	}
}

func TestNormalizePageSize(t *testing.T) {
	if got, err := normalizePageSize(0); err != nil || got != walletpolicy.DefaultListPageSize {
		t.Fatalf("默认分页失败: %d %v", got, err)
	}
	if got, err := normalizePageSize(int32(walletpolicy.MaxListPageSize)); err != nil || got != walletpolicy.MaxListPageSize {
		t.Fatalf("上限分页失败: %d %v", got, err)
	}
	if _, err := normalizePageSize(-1); err == nil {
		t.Fatal("负数分页应被拒绝")
	}
	if _, err := normalizePageSize(int32(walletpolicy.MaxListPageSize) + 1); err == nil {
		t.Fatal("超过上限的分页应被拒绝")
	}
}

func TestTimeRangeValidation(t *testing.T) {
	start := int64(1_700_000_000_000)
	end := start + 1_000

	gotStart, gotEnd, err := timeRange(&start, &end)
	if err != nil {
		t.Fatalf("合法时间窗被拒绝: %v", err)
	}
	if !gotStart.Equal(time.UnixMilli(start)) || !gotEnd.Equal(time.UnixMilli(end)) {
		t.Fatalf("时间窗换算不一致: %v %v", gotStart, gotEnd)
	}

	reversedStart, reversedEnd := end, start
	if _, _, err := timeRange(&reversedStart, &reversedEnd); err == nil {
		t.Fatal("start 晚于 end 应被拒绝")
	}

	negative := int64(-1)
	if _, _, err := timeRange(&negative, nil); err == nil {
		t.Fatal("负 start_time 应被拒绝")
	}
	if _, _, err := timeRange(nil, &negative); err == nil {
		t.Fatal("负 end_time 应被拒绝")
	}

	if gotStart, gotEnd, err := timeRange(nil, nil); err != nil || gotStart != nil || gotEnd != nil {
		t.Fatalf("无时间窗应返回 nil: %v %v %v", gotStart, gotEnd, err)
	}
}

func TestWalletViewHidesIdentity(t *testing.T) {
	account := &ent.WalletAccount{
		ID:            11,
		UserID:        7001,
		Currency:      currencyCNY,
		BalanceAmount: -250,
		Revision:      7,
		UpdatedAt:     time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC),
	}

	view := walletViewOf(account)
	if view.GetStatus() != wallet.WalletStatus_WALLET_STATUS_DEBT {
		t.Fatalf("负余额应派生 DEBT，实际 %v", view.GetStatus())
	}
	if view.GetBalanceAmount() != -250 || view.GetRevision() != 7 {
		t.Fatalf("余额/版本映射错误: %+v", view)
	}
	if view.GetCurrency() != currencyCNY {
		t.Fatalf("币种映射错误: %q", view.GetCurrency())
	}

	admin := adminWalletViewOf(account)
	if admin.GetUserId() != "7001" || admin.GetWallet() == nil {
		t.Fatalf("管理员视图缺少归属: %+v", admin)
	}

	empty := emptyWalletView()
	if empty.GetStatus() != wallet.WalletStatus_WALLET_STATUS_ACTIVE ||
		empty.GetBalanceAmount() != 0 ||
		empty.GetRevision() != 0 ||
		empty.GetCurrency() != currencyCNY {
		t.Fatalf("空钱包视图错误: %+v", empty)
	}
}

func TestTransactionViewsExposeExpectedFields(t *testing.T) {
	related := uint64(42)
	transaction := &ent.WalletTransaction{
		ID:                   99,
		WalletID:             11,
		Type:                 txTypeBalanceRefund,
		Currency:             currencyCNY,
		DeltaAmount:          900,
		BalanceAfter:         1000,
		OwnerNamespace:       testOwnerNamespace,
		BusinessType:         businessTypeBalanceRefund,
		BusinessID:           "42",
		RelatedTransactionID: &related,
		Description:          "balance refund",
		Actor:                actorUser,
		CreatedAt:            time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC),
	}

	userView := transactionViewOf(transaction)
	if userView.GetId() != "99" ||
		userView.GetType() != wallet.TransactionType_TRANSACTION_TYPE_BALANCE_REFUND ||
		userView.GetDeltaAmount() != 900 ||
		userView.GetBalanceAfter() != 1000 {
		t.Fatalf("用户交易视图错误: %+v", userView)
	}

	adminView := adminTransactionViewOf(transaction, 7001)
	if adminView.GetWalletId() != "11" ||
		adminView.GetUserId() != "7001" ||
		adminView.GetOwnerNamespace() != testOwnerNamespace ||
		adminView.GetBusinessType() != businessTypeBalanceRefund ||
		adminView.GetBusinessId() != "42" ||
		adminView.GetRelatedTransactionId() != "42" ||
		adminView.GetActor() != actorUser {
		t.Fatalf("管理员交易视图错误: %+v", adminView)
	}
	if adminView.GetTransaction() == nil {
		t.Fatal("管理员交易视图必须内嵌用户视图")
	}
}
