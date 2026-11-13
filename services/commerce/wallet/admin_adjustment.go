package wallet

import (
	"context"
	"math"
	"strings"

	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// AdminAdjustBalance 实现管理员人工调账（设计 §7、§8）：
// delta_amount 为正表示加款、为负表示扣款，允许把余额扣成负数。
//
// 不可变载荷全部由服务端派生：owner_namespace 来自配置，actor 固定为
// admin:<operator_id>，业务键固定为 admin_adjustment + request_id。
// 同一 request_id 的重复提交只有载荷完全一致才精确重放，否则返回冲突。
func (s CommerceWalletService) AdminAdjustBalance(ctx context.Context, req *wallet.AdminAdjustBalanceReq) (res *wallet.AdminAdjustBalanceResp, err error) {
	if err := s.guard(ctx, ActionAdminAdjustBalance); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, invalidArgument("request is required")
	}

	operator, err := parseDecimalID("operator_id", req.GetOperatorId())
	if err != nil {
		return nil, err
	}
	userID, err := parseDecimalID("user_id", req.GetUserId())
	if err != nil {
		return nil, err
	}

	requestID := strings.TrimSpace(req.GetRequestId())
	reason := strings.TrimSpace(req.GetReason())
	delta := req.GetDeltaAmount()
	if requestID == "" || len(requestID) > maxBusinessIDLen ||
		reason == "" || len(reason) > maxDescriptionLen ||
		delta == 0 || delta == math.MinInt64 {
		return nil, invalidArgument("adjustment parameters are invalid")
	}

	transaction, err := s.applyBalanceChange(ctx, balanceCommand{
		UserID:          userID,
		Currency:        currencyCNY,
		TransactionType: txTypeAdminAdjustment,
		Amount:          delta,
		OwnerNamespace:  s.Cfg.Wallet.OwnerNamespace,
		BusinessType:    businessTypeAdminAdjustment,
		BusinessID:      requestID,
		Description:     reason,
		Actor:           actorAdminPrefix + formatDecimalID(operator),
	})
	if err != nil {
		return nil, err
	}
	return &wallet.AdminAdjustBalanceResp{
		Transaction: adminTransactionViewOf(transaction, userID),
	}, nil
}
