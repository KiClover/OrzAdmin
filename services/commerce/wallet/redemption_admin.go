package wallet

import (
	"context"
	"errors"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/redemptioncode"
	walletpb "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/cloudwego/kitex/pkg/kerrors"
	"github.com/sirupsen/logrus"
)

// adminCreateRedemptionCode 创建兑换码；同一 request_id 的相同请求直接重放。
func (s CommerceWalletService) adminCreateRedemptionCode(
	ctx context.Context,
	req *walletpb.AdminCreateRedemptionCodeReq,
) (*walletpb.AdminCreateRedemptionCodeResp, error) {
	operator, err := parseOperator(req.GetOperatorId())
	if err != nil {
		return nil, err
	}
	requestID := strings.TrimSpace(req.GetRequestId())
	if requestID == "" || len(requestID) > requestIDMaxLen {
		return nil, invalidArgument("request_id is required and must be at most 64 characters")
	}
	codeType, err := redemptionCodeTypeName(req.GetType())
	if err != nil {
		return nil, err
	}
	if req.GetAmount() <= 0 {
		return nil, invalidArgument("amount must be positive")
	}
	now := time.Now()
	var expiresAt *time.Time
	if raw := req.GetExpiresAt(); raw != 0 {
		if raw <= now.UnixMilli() {
			return nil, invalidArgument("expires_at must be in the future")
		}
		value := time.UnixMilli(raw)
		expiresAt = &value
	}

	row, err := createRedemptionCode(ctx, s.DBAll, operator, requestID, codeType, req.GetAmount(), expiresAt)
	if err != nil {
		return nil, wrapInternal("create redemption code", err)
	}
	return &walletpb.AdminCreateRedemptionCodeResp{Code: adminRedemptionCodeView(row, now)}, nil
}

// createRedemptionCode 在事务内处理 request_id 重放与码碰撞重生成。
func createRedemptionCode(
	ctx context.Context,
	client *ent.Client,
	operator, requestID, codeType string,
	amount int64,
	expiresAt *time.Time,
) (*ent.RedemptionCode, error) {
	repository := newWalletRepository(client)
	var created *ent.RedemptionCode

	err := repository.runInTx(ctx, func(tx *ent.Tx) error {
		existing, err := tx.RedemptionCode.Query().
			Where(redemptioncode.RequestID(requestID)).
			Only(ctx)
		if err == nil {
			created = existing
			return replayRedemptionCreate(existing, codeType, amount, expiresAt)
		}
		if !ent.IsNotFound(err) {
			return err
		}

		for attempt := 0; attempt < redemptionCodeAttempts; attempt++ {
			code, err := newRedemptionCode()
			if err != nil {
				return err
			}
			builder := tx.RedemptionCode.Create().
				SetRequestID(requestID).
				SetType(codeType).
				SetCode(code).
				SetAmount(amount).
				SetCurrency(currencyCNY).
				SetStatus(redemptionStatusActive).
				SetRedeemedCount(0).
				SetCreatedBy(operator)
			if expiresAt != nil {
				builder.SetExpiresAt(*expiresAt)
			}
			row, err := builder.Save(ctx)
			if err == nil {
				created = row
				return nil
			}
			if !ent.IsConstraintError(err) {
				return err
			}
			// 可能撞上 request_id（并发重复创建）或 code（极小概率碰撞）。
			if existing, qerr := tx.RedemptionCode.Query().
				Where(redemptioncode.RequestID(requestID)).
				Only(ctx); qerr == nil {
				created = existing
				return replayRedemptionCreate(existing, codeType, amount, expiresAt)
			}
		}
		return walletUnavailable("redemption code could not be generated")
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// replayRedemptionCreate 校验重复请求是否与既有兑换码一致。
func replayRedemptionCreate(
	existing *ent.RedemptionCode,
	codeType string,
	amount int64,
	expiresAt *time.Time,
) error {
	if existing.Type != codeType || existing.Amount != amount {
		return idempotencyConflict()
	}
	if !sameOptionalTime(existing.ExpiresAt, expiresAt) {
		return idempotencyConflict()
	}
	return nil
}

// sameOptionalTime 比较两个可空时间是否指向同一毫秒。
func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.UnixMilli() == right.UnixMilli()
}

// adminListRedemptionCodes 管理员分页查看兑换码（含完整明文）。
func (s CommerceWalletService) adminListRedemptionCodes(
	ctx context.Context,
	req *walletpb.AdminListRedemptionCodesReq,
) (*walletpb.AdminListRedemptionCodesResp, error) {
	pageSize, err := normalizePageSize(req.GetPageSize())
	if err != nil {
		return nil, err
	}
	position, err := decodeCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	now := time.Now()
	query := s.DB.RedemptionCode.Query()
	switch req.GetStatus() {
	case walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE:
		query = query.Where(
			redemptioncode.Status(redemptionStatusActive),
			redemptioncode.Or(
				redemptioncode.ExpiresAtIsNil(),
				redemptioncode.ExpiresAtGT(now),
			),
		)
	case walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_EXPIRED:
		query = query.Where(
			redemptioncode.Status(redemptionStatusActive),
			redemptioncode.ExpiresAtNotNil(),
			redemptioncode.ExpiresAtLTE(now),
		)
	case walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_DISABLED:
		query = query.Where(redemptioncode.Status(redemptionStatusDisabled))
	case walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_CLAIMED:
		query = query.Where(redemptioncode.Status(redemptionStatusClaimed))
	}
	switch req.GetType() {
	case walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE:
		query = query.Where(redemptioncode.Type(redemptionCodeTypeUnique))
	case walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED:
		query = query.Where(redemptioncode.Type(redemptionCodeTypeShared))
	}

	query = applyRedemptionCursor(query, position)
	rows, hasMore, err := fetchRedemptionPage(ctx, query, pageSize)
	if err != nil {
		return nil, wrapInternal("list redemption codes", err)
	}

	items := make([]*walletpb.AdminRedemptionCodeView, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminRedemptionCodeView(row, now))
	}
	next := ""
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return &walletpb.AdminListRedemptionCodesResp{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// applyRedemptionCursor 追加严格的 (created_at, id) 游标过滤。
func applyRedemptionCursor(
	query *ent.RedemptionCodeQuery,
	position cursor,
) *ent.RedemptionCodeQuery {
	if position.empty() {
		return query
	}
	return query.Where(redemptioncode.Or(
		redemptioncode.CreatedAtLT(position.createdAt),
		redemptioncode.And(
			redemptioncode.CreatedAtEQ(position.createdAt),
			redemptioncode.IDLT(position.id),
		),
	))
}

// fetchRedemptionPage 取 pageSize+1 条判断 has_more 并截断。
func fetchRedemptionPage(
	ctx context.Context,
	query *ent.RedemptionCodeQuery,
	pageSize int,
) ([]*ent.RedemptionCode, bool, error) {
	rows, err := query.
		Order(redemptioncode.ByCreatedAt(sql.OrderDesc()), redemptioncode.ByID(sql.OrderDesc())).
		Limit(pageSize + 1).
		All(ctx)
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > pageSize
	if hasMore {
		rows = rows[:pageSize]
	}
	return rows, hasMore, nil
}

// adminSetRedemptionCodeStatus 只允许 ACTIVE 与 DISABLED 之间切换。
func (s CommerceWalletService) adminSetRedemptionCodeStatus(
	ctx context.Context,
	req *walletpb.AdminSetRedemptionCodeStatusReq,
) (*walletpb.AdminSetRedemptionCodeStatusResp, error) {
	operator, err := parseOperator(req.GetOperatorId())
	if err != nil {
		return nil, err
	}
	codeID, err := parseDecimalID("code_id", req.GetCodeId())
	if err != nil {
		return nil, err
	}
	target := req.GetStatus()
	if target != walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE &&
		target != walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_DISABLED {
		return nil, invalidArgument("status must be ACTIVE or DISABLED")
	}
	reason := strings.TrimSpace(req.GetReason())
	if len(reason) > rechargeReviewReasonMaxLen {
		return nil, invalidArgument("reason is too long")
	}

	now := time.Now()
	var updated *ent.RedemptionCode
	err = s.repository().runInTx(ctx, func(tx *ent.Tx) error {
		row, err := tx.RedemptionCode.Query().
			Where(redemptioncode.ID(codeID)).
			ForUpdate().
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return walletNotFound("redemption code was not found")
			}
			return err
		}
		if row.Status == redemptionStatusClaimed {
			return stateConflict("a claimed redemption code cannot change status")
		}
		if redemptionExpired(row, now) && target == walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE {
			return stateConflict("an expired redemption code cannot be enabled")
		}

		current := row.Status
		desired := redemptionStatusDisabled
		if target == walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE {
			desired = redemptionStatusActive
		}
		if current == desired {
			updated = row
			return nil
		}

		affected, err := tx.RedemptionCode.Update().
			Where(
				redemptioncode.ID(row.ID),
				redemptioncode.Status(current),
			).
			SetStatus(desired).
			Save(ctx)
		if err != nil {
			return err
		}
		if affected != 1 {
			return stateConflict("redemption code status changed concurrently")
		}
		refreshed, err := tx.RedemptionCode.Get(ctx, row.ID)
		if err != nil {
			return err
		}
		updated = refreshed
		redemptionStatusAudit(operator, row.ID, current, desired, reason)
		return nil
	})
	if err != nil {
		var biz *kerrors.BizStatusError
		if errors.As(err, &biz) {
			return nil, err
		}
		return nil, wrapInternal("set redemption code status", err)
	}
	return &walletpb.AdminSetRedemptionCodeStatusResp{
		Code: adminRedemptionCodeView(updated, now),
	}, nil
}

// redemptionStatusAudit 记录兑换码启停审计；只写 code id 与操作人，绝不写明文码。
func redemptionStatusAudit(operator string, codeID uint64, from, to, reason string) {
	logger.WithFields(logrus.Fields{
		"audit":    "redemption_code_status",
		"operator": operator,
		"code_id":  formatDecimalID(codeID),
		"from":     from,
		"to":       to,
		"reason":   reason,
	}).Info("redemption code status changed")
}
