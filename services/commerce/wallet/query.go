package wallet

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/walletaccount"
	"github.com/KiClover/orzAdmin/ent/wallettransaction"
	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// 查询类 RPC（GetWallet/ListTransactions/AdminGetWallet/AdminListWallets/AdminListTransactions）。
//
// 列表一律使用 (created_at, id) 的稳定游标，不使用大 offset：按时间倒序返回最新记录，
// 游标只编码上一页最后一条的唯一位置，并在下一页用严格小于过滤，避免并发写入导致重复或遗漏。

// cursor 是列表的稳定分页位置。
type cursor struct {
	createdAt time.Time
	id        uint64
}

func (c cursor) empty() bool {
	return c.id == 0
}

// encodeCursor 把 (created_at, id) 编码成对客户端不透明的短字符串。
func encodeCursor(createdAt time.Time, id uint64) string {
	raw := strconv.FormatInt(createdAt.UTC().UnixMilli(), 10) + ":" + strconv.FormatUint(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor 解析游标；空字符串表示第一页，非法游标一律拒绝。
func decodeCursor(raw string) (cursor, error) {
	if raw == "" {
		return cursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursor{}, invalidArgument("cursor is invalid")
	}
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 2 {
		return cursor{}, invalidArgument("cursor is invalid")
	}
	millis, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || millis < 0 {
		return cursor{}, invalidArgument("cursor is invalid")
	}
	id, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || id == 0 {
		return cursor{}, invalidArgument("cursor is invalid")
	}
	return cursor{createdAt: time.UnixMilli(millis).UTC(), id: id}, nil
}

// normalizePageSize 应用分页默认值与上限；负数与超大值都拒绝。
func normalizePageSize(size int32) (int, error) {
	if size < 0 {
		return 0, invalidArgument("page_size must not be negative")
	}
	if size == 0 {
		return walletpolicy.DefaultListPageSize, nil
	}
	if int(size) > walletpolicy.MaxListPageSize {
		return 0, invalidArgument("page_size is too large")
	}
	return int(size), nil
}

// walletViewOf 把账户实体映射成用户可见的钱包视图（不含 wallet_id/user_id）。
func walletViewOf(account *ent.WalletAccount) *wallet.WalletView {
	return &wallet.WalletView{
		Currency:      account.Currency,
		BalanceAmount: account.BalanceAmount,
		Status:        walletStatusFor(account.BalanceAmount),
		Revision:      int64(account.Revision),
		UpdatedAt:     account.UpdatedAt.UTC().UnixMilli(),
	}
}

// emptyWalletView 是尚无钱包行时的零余额视图：按需建行发生在首次资金变更。
func emptyWalletView() *wallet.WalletView {
	return &wallet.WalletView{
		Currency:      currencyCNY,
		BalanceAmount: 0,
		Status:        wallet.WalletStatus_WALLET_STATUS_ACTIVE,
		Revision:      0,
		UpdatedAt:     0,
	}
}

func adminWalletViewOf(account *ent.WalletAccount) *wallet.AdminWalletView {
	return &wallet.AdminWalletView{
		Wallet:   walletViewOf(account),
		UserId:   formatDecimalID(account.UserID),
		WalletId: formatDecimalID(account.ID),
	}
}

// transactionViewOf 把流水实体映射成用户视图（不含归属与业务键）。
func transactionViewOf(transaction *ent.WalletTransaction) *wallet.TransactionView {
	return &wallet.TransactionView{
		Id:           formatDecimalID(transaction.ID),
		Type:         transactionTypeFromName(transaction.Type),
		Currency:     transaction.Currency,
		DeltaAmount:  transaction.DeltaAmount,
		BalanceAfter: transaction.BalanceAfter,
		Description:  transaction.Description,
		CreatedAt:    transaction.CreatedAt.UTC().UnixMilli(),
	}
}

// adminTransactionViewOf 追加管理员可见的归属、业务键与 actor。
func adminTransactionViewOf(transaction *ent.WalletTransaction, userID uint64) *wallet.AdminTransactionView {
	view := &wallet.AdminTransactionView{
		Transaction:    transactionViewOf(transaction),
		WalletId:       formatDecimalID(transaction.WalletID),
		UserId:         formatDecimalID(userID),
		OwnerNamespace: transaction.OwnerNamespace,
		BusinessType:   transaction.BusinessType,
		BusinessId:     transaction.BusinessID,
		Actor:          transaction.Actor,
	}
	if transaction.RelatedTransactionID != nil {
		view.RelatedTransactionId = formatDecimalID(*transaction.RelatedTransactionID)
	}
	return view
}

// walletByUser 查询用户的 CNY 钱包，未找到返回 ent.NotFound。
func (s *CommerceWalletService) walletByUser(ctx context.Context, userID uint64) (*ent.WalletAccount, error) {
	return s.DB.Account.Query().
		Where(walletaccount.UserID(userID), walletaccount.Currency(currencyCNY)).
		Only(ctx)
}

// getWallet 实现用户侧 GetWallet；尚无钱包行时返回零余额视图而不是创建行。
func (s *CommerceWalletService) getWallet(ctx context.Context, req *wallet.GetWalletReq) (*wallet.GetWalletResp, error) {
	userID, err := parseDecimalID("user_id", req.GetUserId())
	if err != nil {
		return nil, err
	}

	account, err := s.walletByUser(ctx, userID)
	if ent.IsNotFound(err) {
		return &wallet.GetWalletResp{Wallet: emptyWalletView()}, nil
	}
	if err != nil {
		return nil, wrapInternal("get wallet", err)
	}
	return &wallet.GetWalletResp{Wallet: walletViewOf(account)}, nil
}

// listTransactions 实现用户侧 ListTransactions；钱包不存在时返回空列表。
func (s *CommerceWalletService) listTransactions(ctx context.Context, req *wallet.ListTransactionsReq) (*wallet.ListTransactionsResp, error) {
	userID, err := parseDecimalID("user_id", req.GetUserId())
	if err != nil {
		return nil, err
	}
	pageSize, err := normalizePageSize(req.GetPageSize())
	if err != nil {
		return nil, err
	}
	position, err := decodeCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}
	startTime, endTime, err := timeRange(req.StartTime, req.EndTime)
	if err != nil {
		return nil, err
	}

	account, err := s.walletByUser(ctx, userID)
	if ent.IsNotFound(err) {
		return &wallet.ListTransactionsResp{Items: []*wallet.TransactionView{}, HasMore: false}, nil
	}
	if err != nil {
		return nil, wrapInternal("list transactions", err)
	}

	query := s.DB.Transaction.Query().
		Where(
			wallettransaction.WalletID(account.ID),
			wallettransaction.Currency(currencyCNY),
		)
	query, err = applyTransactionFilters(query, req.Type, startTime, endTime, position)
	if err != nil {
		return nil, err
	}

	transactions, hasMore, err := fetchTransactionPage(ctx, query, pageSize)
	if err != nil {
		return nil, wrapInternal("list transactions", err)
	}

	items := make([]*wallet.TransactionView, 0, len(transactions))
	for _, transaction := range transactions {
		items = append(items, transactionViewOf(transaction))
	}

	next := ""
	if hasMore && len(transactions) > 0 {
		last := transactions[len(transactions)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return &wallet.ListTransactionsResp{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

// adminGetWallet 实现管理员钱包详情：wallet_id 与 user_id 至少提供一个，同时提供时必须一致。
func (s *CommerceWalletService) adminGetWallet(ctx context.Context, req *wallet.AdminGetWalletReq) (*wallet.AdminGetWalletResp, error) {
	hasWalletID := req.WalletId != nil && strings.TrimSpace(*req.WalletId) != ""
	hasUserID := req.UserId != nil && strings.TrimSpace(*req.UserId) != ""
	if !hasWalletID && !hasUserID {
		return nil, invalidArgument("wallet_id or user_id is required")
	}

	var (
		account *ent.WalletAccount
		err     error
	)
	if hasWalletID {
		walletID, parseErr := parseDecimalID("wallet_id", *req.WalletId)
		if parseErr != nil {
			return nil, parseErr
		}
		account, err = s.DB.Account.Get(ctx, walletID)
		if ent.IsNotFound(err) {
			return nil, walletNotFound("wallet was not found")
		}
		if err != nil {
			return nil, wrapInternal("get wallet", err)
		}
		if hasUserID {
			userID, parseErr := parseDecimalID("user_id", *req.UserId)
			if parseErr != nil {
				return nil, parseErr
			}
			if account.UserID != userID {
				return nil, invalidArgument("wallet_id and user_id do not match")
			}
		}
		return &wallet.AdminGetWalletResp{Wallet: adminWalletViewOf(account)}, nil
	}

	userID, err := parseDecimalID("user_id", *req.UserId)
	if err != nil {
		return nil, err
	}
	account, err = s.walletByUser(ctx, userID)
	if ent.IsNotFound(err) {
		return nil, walletNotFound("wallet was not found")
	}
	if err != nil {
		return nil, wrapInternal("get wallet", err)
	}
	return &wallet.AdminGetWalletResp{Wallet: adminWalletViewOf(account)}, nil
}

// adminListWallets 实现管理员钱包列表：可按用户与派生状态过滤。
func (s *CommerceWalletService) adminListWallets(ctx context.Context, req *wallet.AdminListWalletsReq) (*wallet.AdminListWalletsResp, error) {
	pageSize, err := normalizePageSize(req.GetPageSize())
	if err != nil {
		return nil, err
	}
	position, err := decodeCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	query := s.DB.Account.Query().Where(walletaccount.Currency(currencyCNY))
	if req.UserId != nil && strings.TrimSpace(*req.UserId) != "" {
		userID, parseErr := parseDecimalID("user_id", *req.UserId)
		if parseErr != nil {
			return nil, parseErr
		}
		query = query.Where(walletaccount.UserID(userID))
	}
	if req.Status != nil {
		switch req.GetStatus() {
		case wallet.WalletStatus_WALLET_STATUS_ACTIVE:
			query = query.Where(walletaccount.BalanceAmountGTE(0))
		case wallet.WalletStatus_WALLET_STATUS_DEBT:
			query = query.Where(walletaccount.BalanceAmountLT(0))
		default:
			return nil, invalidArgument("status is not supported")
		}
	}
	if !position.empty() {
		query = query.Where(walletaccount.Or(
			walletaccount.CreatedAtLT(position.createdAt),
			walletaccount.And(
				walletaccount.CreatedAtEQ(position.createdAt),
				walletaccount.IDLT(position.id),
			),
		))
	}

	accounts, err := query.
		Order(walletaccount.ByCreatedAt(entsql.OrderDesc()), walletaccount.ByID(entsql.OrderDesc())).
		Limit(pageSize + 1).
		All(ctx)
	if err != nil {
		return nil, wrapInternal("list wallets", err)
	}

	hasMore := len(accounts) > pageSize
	if hasMore {
		accounts = accounts[:pageSize]
	}

	items := make([]*wallet.AdminWalletView, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, adminWalletViewOf(account))
	}
	next := ""
	if hasMore && len(accounts) > 0 {
		last := accounts[len(accounts)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return &wallet.AdminListWalletsResp{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

// adminListTransactions 实现管理员流水列表：可按 wallet_id 或 user_id 过滤。
func (s *CommerceWalletService) adminListTransactions(ctx context.Context, req *wallet.AdminListTransactionsReq) (*wallet.AdminListTransactionsResp, error) {
	pageSize, err := normalizePageSize(req.GetPageSize())
	if err != nil {
		return nil, err
	}
	position, err := decodeCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}
	startTime, endTime, err := timeRange(req.StartTime, req.EndTime)
	if err != nil {
		return nil, err
	}

	query := s.DB.Transaction.Query().Where(wallettransaction.Currency(currencyCNY))

	walletIDs, err := s.adminTransactionWalletIDs(ctx, req)
	if err != nil {
		return nil, err
	}
	if walletIDs != nil {
		if len(walletIDs) == 0 {
			return &wallet.AdminListTransactionsResp{Items: []*wallet.AdminTransactionView{}, HasMore: false}, nil
		}
		query = query.Where(wallettransaction.WalletIDIn(walletIDs...))
	}

	query, err = applyTransactionFilters(query, req.Type, startTime, endTime, position)
	if err != nil {
		return nil, err
	}

	transactions, hasMore, err := fetchTransactionPage(ctx, query, pageSize)
	if err != nil {
		return nil, wrapInternal("list transactions", err)
	}

	userIDByWallet, err := s.userIDsForWallets(ctx, transactions)
	if err != nil {
		return nil, wrapInternal("list transactions", err)
	}

	items := make([]*wallet.AdminTransactionView, 0, len(transactions))
	for _, transaction := range transactions {
		items = append(items, adminTransactionViewOf(transaction, userIDByWallet[transaction.WalletID]))
	}
	next := ""
	if hasMore && len(transactions) > 0 {
		last := transactions[len(transactions)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return &wallet.AdminListTransactionsResp{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

// adminTransactionWalletIDs 把管理员过滤条件收敛成钱包 ID 集合：
//   - 都没提供：nil，表示不过滤；
//   - wallet_id：直接使用；
//   - user_id：解析成该用户的 CNY 钱包，未建钱包时返回空集合。
func (s *CommerceWalletService) adminTransactionWalletIDs(ctx context.Context, req *wallet.AdminListTransactionsReq) ([]uint64, error) {
	hasWalletID := req.WalletId != nil && strings.TrimSpace(*req.WalletId) != ""
	hasUserID := req.UserId != nil && strings.TrimSpace(*req.UserId) != ""
	if !hasWalletID && !hasUserID {
		return nil, nil
	}

	var walletIDs []uint64
	if hasWalletID {
		walletID, err := parseDecimalID("wallet_id", *req.WalletId)
		if err != nil {
			return nil, err
		}
		walletIDs = append(walletIDs, walletID)
	}
	if hasUserID {
		userID, err := parseDecimalID("user_id", *req.UserId)
		if err != nil {
			return nil, err
		}
		account, err := s.walletByUser(ctx, userID)
		if ent.IsNotFound(err) {
			return []uint64{}, nil
		}
		if err != nil {
			return nil, wrapInternal("list transactions", err)
		}
		if hasWalletID && account.ID != walletIDs[0] {
			return nil, invalidArgument("wallet_id and user_id do not match")
		}
		if !hasWalletID {
			walletIDs = append(walletIDs, account.ID)
		}
	}
	return walletIDs, nil
}

// userIDsForWallets 批量补齐管理员流水的 user_id，避免逐条查询。
func (s *CommerceWalletService) userIDsForWallets(ctx context.Context, transactions []*ent.WalletTransaction) (map[uint64]uint64, error) {
	walletIDs := make([]uint64, 0, len(transactions))
	seen := make(map[uint64]struct{}, len(transactions))
	for _, transaction := range transactions {
		if _, ok := seen[transaction.WalletID]; ok {
			continue
		}
		seen[transaction.WalletID] = struct{}{}
		walletIDs = append(walletIDs, transaction.WalletID)
	}

	result := make(map[uint64]uint64, len(walletIDs))
	if len(walletIDs) == 0 {
		return result, nil
	}

	accounts, err := s.DB.Account.Query().Where(walletaccount.IDIn(walletIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, account := range accounts {
		result[account.ID] = account.UserID
	}
	return result, nil
}

// applyTransactionFilters 统一收敛用户/管理员列表的过滤条件（类型、时间窗、游标）。
func applyTransactionFilters(
	query *ent.WalletTransactionQuery,
	transactionType *wallet.TransactionType,
	startTime, endTime *time.Time,
	position cursor,
) (*ent.WalletTransactionQuery, error) {
	if transactionType != nil {
		name, err := transactionTypeName(*transactionType)
		if err != nil {
			return nil, err
		}
		query = query.Where(wallettransaction.Type(name))
	}
	if startTime != nil {
		query = query.Where(wallettransaction.CreatedAtGTE(*startTime))
	}
	if endTime != nil {
		query = query.Where(wallettransaction.CreatedAtLTE(*endTime))
	}
	if !position.empty() {
		query = query.Where(wallettransaction.Or(
			wallettransaction.CreatedAtLT(position.createdAt),
			wallettransaction.And(
				wallettransaction.CreatedAtEQ(position.createdAt),
				wallettransaction.IDLT(position.id),
			),
		))
	}
	return query, nil
}

// fetchTransactionPage 取 pageSize+1 条判断 has_more，并截断到 pageSize。
func fetchTransactionPage(
	ctx context.Context,
	query *ent.WalletTransactionQuery,
	pageSize int,
) ([]*ent.WalletTransaction, bool, error) {
	transactions, err := query.
		Order(wallettransaction.ByCreatedAt(entsql.OrderDesc()), wallettransaction.ByID(entsql.OrderDesc())).
		Limit(pageSize + 1).
		All(ctx)
	if err != nil {
		return nil, false, err
	}

	hasMore := len(transactions) > pageSize
	if hasMore {
		transactions = transactions[:pageSize]
	}
	return transactions, hasMore, nil
}

// timeRange 校验并归一化 Unix 毫秒时间窗；start 必须早于或等于 end。
func timeRange(startMillis, endMillis *int64) (*time.Time, *time.Time, error) {
	var start, end *time.Time
	if startMillis != nil {
		if *startMillis < 0 {
			return nil, nil, invalidArgument("start_time is invalid")
		}
		value := time.UnixMilli(*startMillis).UTC()
		start = &value
	}
	if endMillis != nil {
		if *endMillis < 0 {
			return nil, nil, invalidArgument("end_time is invalid")
		}
		value := time.UnixMilli(*endMillis).UTC()
		end = &value
	}
	if start != nil && end != nil && start.After(*end) {
		return nil, nil, invalidArgument("start_time must not be after end_time")
	}
	return start, end, nil
}
