package wallet

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/redemptioncode"
	walletpb "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
)

// 兑换码类型与状态取值，与数据库中的明文取值保持一致。
const (
	redemptionCodeTypeUnique = "UNIQUE"
	redemptionCodeTypeShared = "SHARED"

	redemptionStatusActive   = "ACTIVE"
	redemptionStatusDisabled = "DISABLED"
	redemptionStatusClaimed  = "CLAIMED"

	// businessTypeRedemption 是兑换入账的固定业务类型。
	businessTypeRedemption = "redemption"

	// redemptionCodeAlphabet 去掉了 0/O、1/I/L 等易混字符，避免人工输入歧义。
	redemptionCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

	// redemptionCodeLength 是规范化后的码长度（20 个字符，约 100 位熵）。
	redemptionCodeLength = 20

	// redemptionCodeAttempts 是生成码时遇到唯一碰撞的最大重试次数。
	redemptionCodeAttempts = 5

	// redemptionCodeMaxInputLen 限制兑换请求里可接受的原始字符串长度。
	redemptionCodeMaxInputLen = 128

	// redemptionUnavailableMessage 是兑换失败的统一文案：
	// 不存在、禁用、已领取、已兑换、过期都返回它，避免枚举有效码。
	redemptionUnavailableMessage = "redemption code is not available"
)

// redeemCode 在单个事务内锁定兑换码与钱包，校验可用性后记入余额。
func (s CommerceWalletService) redeemCode(ctx context.Context, req *walletpb.RedeemCodeReq) (*walletpb.RedeemCodeResp, error) {
	userID, err := parseDecimalID("user_id", req.GetUserId())
	if err != nil {
		return nil, err
	}
	// 明文码原样查询：大小写与连字符都参与精确匹配。
	code := req.GetCode()
	if !validRedemptionCode(code) {
		return nil, walletNotFound(redemptionUnavailableMessage)
	}

	now := time.Now()
	repository := s.repository()
	var walletAccount *ent.WalletAccount
	var credited int64

	err = repository.runInTx(ctx, func(tx *ent.Tx) error {
		row, err := tx.RedemptionCode.Query().
			Where(redemptioncode.Code(code)).
			ForUpdate().
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return walletNotFound(redemptionUnavailableMessage)
			}
			return err
		}
		if row.Status != redemptionStatusActive || redemptionExpired(row, now) {
			return walletNotFound(redemptionUnavailableMessage)
		}

		account, err := repository.lockWallet(ctx, tx, userID)
		if err != nil {
			return err
		}

		// 同一用户对同一兑换码只入账一次：业务键固定为 "<code_id>:<user_id>"。
		command := balanceCommand{
			UserID:          userID,
			Currency:        currencyCNY,
			TransactionType: txTypeRedemptionCredit,
			Amount:          row.Amount,
			OwnerNamespace:  s.Cfg.Wallet.OwnerNamespace,
			BusinessType:    businessTypeRedemption,
			BusinessID:      fmt.Sprintf("%d:%d", row.ID, userID),
			Description:     "redemption code credit",
			Actor:           actorUser,
		}
		existing, err := repository.findTransaction(ctx, tx, command)
		if err != nil {
			return err
		}
		if existing != nil {
			return walletNotFound(redemptionUnavailableMessage)
		}

		updated, err := claimRedemptionCode(ctx, tx, row, userID, now)
		if err != nil {
			return err
		}
		if !updated {
			return walletNotFound(redemptionUnavailableMessage)
		}

		balance, ok := addAmount(account.BalanceAmount, row.Amount)
		if !ok {
			return walletInternal("wallet balance overflow")
		}
		if err := repository.applyBalance(ctx, tx, account, balance); err != nil {
			return err
		}
		if _, err := repository.insertTransaction(ctx, tx, command, account.ID, row.Amount, balance); err != nil {
			return err
		}

		refreshed, err := tx.WalletAccount.Get(ctx, account.ID)
		if err != nil {
			return err
		}
		walletAccount = refreshed
		credited = row.Amount
		return nil
	})
	if err != nil {
		if ent.IsConstraintError(err) {
			// 并发兑换同一用户/同一码时由唯一索引兜底，对外仍是统一失败文案。
			return nil, walletNotFound(redemptionUnavailableMessage)
		}
		return nil, wrapInternal("redeem code", err)
	}

	return &walletpb.RedeemCodeResp{
		Wallet:         walletViewOf(walletAccount),
		CreditedAmount: credited,
	}, nil
}

// claimRedemptionCode 按类型推进兑换码状态：UNIQUE 领取后变 CLAIMED，SHARED 只累加次数。
func claimRedemptionCode(
	ctx context.Context,
	tx *ent.Tx,
	row *ent.RedemptionCode,
	userID uint64,
	now time.Time,
) (bool, error) {
	if row.Type == redemptionCodeTypeUnique {
		affected, err := tx.RedemptionCode.Update().
			Where(
				redemptioncode.ID(row.ID),
				redemptioncode.Status(redemptionStatusActive),
			).
			SetStatus(redemptionStatusClaimed).
			SetClaimedByUserID(userID).
			SetClaimedAt(now).
			AddRedeemedCount(1).
			Save(ctx)
		if err != nil {
			return false, err
		}
		return affected == 1, nil
	}

	// SHARED 码保持 ACTIVE，仅当管理员仍未禁用时累加兑换次数。
	affected, err := tx.RedemptionCode.Update().
		Where(
			redemptioncode.ID(row.ID),
			redemptioncode.Status(redemptionStatusActive),
		).
		AddRedeemedCount(1).
		Save(ctx)
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// validRedemptionCode 校验兑换码明文。
//
// 只接受 1~128 字节的可见 ASCII（0x21~0x7E）：不做去空白、大小写折叠
// 或连字符移除，兑换码按调用方给出的原值精确认领。
func validRedemptionCode(raw string) bool {
	if len(raw) == 0 || len(raw) > redemptionCodeMaxInputLen {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x21 || raw[i] > 0x7e {
			return false
		}
	}
	return true
}

// newRedemptionCode 用密码学安全随机数生成规范明文码。
func newRedemptionCode() (string, error) {
	buffer := make([]byte, redemptionCodeLength)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	code := make([]byte, redemptionCodeLength)
	for i, value := range buffer {
		code[i] = redemptionCodeAlphabet[int(value)%len(redemptionCodeAlphabet)]
	}
	return string(code), nil
}

// redemptionExpired 判断兑换码是否已过期；未设置过期时间为永久有效。
func redemptionExpired(row *ent.RedemptionCode, now time.Time) bool {
	return row.ExpiresAt != nil && !now.Before(*row.ExpiresAt)
}

// redemptionStatusOf 返回对外展示状态：ACTIVE 且已过期时派生为 EXPIRED。
func redemptionStatusOf(row *ent.RedemptionCode, now time.Time) walletpb.RedemptionCodeStatus {
	switch row.Status {
	case redemptionStatusDisabled:
		return walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_DISABLED
	case redemptionStatusClaimed:
		return walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_CLAIMED
	}
	if redemptionExpired(row, now) {
		return walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_EXPIRED
	}
	return walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE
}

// redemptionCodeTypeProto 把数据库取值映射为 proto 枚举。
func redemptionCodeTypeProto(raw string) walletpb.RedemptionCodeType {
	if raw == redemptionCodeTypeShared {
		return walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED
	}
	return walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE
}

// redemptionCodeTypeName 把 proto 枚举映射为数据库取值。
func redemptionCodeTypeName(value walletpb.RedemptionCodeType) (string, error) {
	switch value {
	case walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE:
		return redemptionCodeTypeUnique, nil
	case walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED:
		return redemptionCodeTypeShared, nil
	default:
		return "", invalidArgument("type must be UNIQUE or SHARED")
	}
}

// adminRedemptionCodeView 构造管理员视图（唯一允许出现完整明文码的响应）。
func adminRedemptionCodeView(row *ent.RedemptionCode, now time.Time) *walletpb.AdminRedemptionCodeView {
	view := &walletpb.AdminRedemptionCodeView{
		Id:            formatDecimalID(row.ID),
		Code:          row.Code,
		Type:          redemptionCodeTypeProto(row.Type),
		Amount:        row.Amount,
		Currency:      row.Currency,
		Status:        redemptionStatusOf(row, now),
		RedeemedCount: int64(row.RedeemedCount),
		CreatedBy:     row.CreatedBy,
		CreatedAt:     row.CreatedAt.UnixMilli(),
		UpdatedAt:     row.UpdatedAt.UnixMilli(),
	}
	if row.ExpiresAt != nil {
		view.ExpiresAt = row.ExpiresAt.UnixMilli()
	}
	if row.ClaimedByUserID != nil {
		view.ClaimedByUserId = formatDecimalID(*row.ClaimedByUserID)
	}
	if row.ClaimedAt != nil {
		view.ClaimedAt = row.ClaimedAt.UnixMilli()
	}
	return view
}
