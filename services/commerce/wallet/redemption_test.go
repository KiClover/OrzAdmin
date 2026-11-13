package wallet

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/ent"
	walletpb "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting"
)

// redemptionSnapshot 在默认快照上打开兑换功能，供下单/兑换用例复用。
func redemptionSnapshot() *setting.WalletRuntimeSettings {
	snapshot := validSnapshot(11)
	snapshot.RedemptionEnabled = true
	snapshot.RedemptionRateLimit = 10
	snapshot.RedemptionRateWindowSeconds = 60
	return snapshot
}

func TestRedemptionValidCode(t *testing.T) {
	valid := []string{
		"A",
		"Vip-2026",
		"a-b",
		"ABCDE-FGHJK-LMNPQ-RSTUV",
		"!!~~",
		strings.Repeat("~", redemptionCodeMaxInputLen),
	}
	for _, raw := range valid {
		if !validRedemptionCode(raw) {
			t.Fatalf("validRedemptionCode(%q) 应接受", raw)
		}
	}

	invalid := []string{
		"",
		"SHORT CODE",
		" Vip",
		"Vip ",
		"line\nbreak",
		"tab\tcode",
		"兑换码",
		strings.Repeat("A", redemptionCodeMaxInputLen+1),
	}
	for _, raw := range invalid {
		if validRedemptionCode(raw) {
			t.Fatalf("validRedemptionCode(%q) 应拒绝", raw)
		}
	}
}

func TestRedemptionGenerateCode(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 200; i++ {
		code, err := newRedemptionCode()
		if err != nil {
			t.Fatalf("生成兑换码失败: %v", err)
		}
		if len(code) != redemptionCodeLength {
			t.Fatalf("长度 = %d，期望 %d", len(code), redemptionCodeLength)
		}
		for _, r := range code {
			if !strings.ContainsRune(redemptionCodeAlphabet, r) {
				t.Fatalf("码 %q 含非法字符 %q", code, r)
			}
		}
		if _, ok := seen[code]; ok {
			t.Fatalf("生成的兑换码重复: %q", code)
		}
		seen[code] = struct{}{}
		// 生成结果必须是兑换接口接受的合法自定义码。
		if !validRedemptionCode(code) {
			t.Fatalf("生成的码未通过校验: %q", code)
		}
	}
}

func TestRedemptionStatusDerivation(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name   string
		row    *ent.RedemptionCode
		expect walletpb.RedemptionCodeStatus
	}{
		{
			name:   "active without expiry",
			row:    &ent.RedemptionCode{Status: redemptionStatusActive},
			expect: walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE,
		},
		{
			name:   "active with future expiry",
			row:    &ent.RedemptionCode{Status: redemptionStatusActive, ExpiresAt: &future},
			expect: walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE,
		},
		{
			name:   "active but expired",
			row:    &ent.RedemptionCode{Status: redemptionStatusActive, ExpiresAt: &past},
			expect: walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_EXPIRED,
		},
		{
			name:   "disabled wins over expiry",
			row:    &ent.RedemptionCode{Status: redemptionStatusDisabled, ExpiresAt: &past},
			expect: walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_DISABLED,
		},
		{
			name:   "claimed wins over expiry",
			row:    &ent.RedemptionCode{Status: redemptionStatusClaimed, ExpiresAt: &past},
			expect: walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_CLAIMED,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redemptionStatusOf(tc.row, now); got != tc.expect {
				t.Fatalf("状态 = %v，期望 %v", got, tc.expect)
			}
		})
	}

	if !redemptionExpired(&ent.RedemptionCode{ExpiresAt: &past}, now) {
		t.Fatalf("过去时间应判定过期")
	}
	if redemptionExpired(&ent.RedemptionCode{ExpiresAt: &future}, now) {
		t.Fatalf("未来时间不应判定过期")
	}
	if redemptionExpired(&ent.RedemptionCode{}, now) {
		t.Fatalf("未设置过期时间不应判定过期")
	}
}

func TestRedemptionTypeMapping(t *testing.T) {
	unique, err := redemptionCodeTypeName(walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE)
	if err != nil || unique != redemptionCodeTypeUnique {
		t.Fatalf("UNIQUE 映射失败: %v %q", err, unique)
	}
	shared, err := redemptionCodeTypeName(walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED)
	if err != nil || shared != redemptionCodeTypeShared {
		t.Fatalf("SHARED 映射失败: %v %q", err, shared)
	}
	if _, err := redemptionCodeTypeName(walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNSPECIFIED); err == nil {
		t.Fatalf("UNSPECIFIED 应返回参数错误")
	}

	if got := redemptionCodeTypeProto(redemptionCodeTypeUnique); got != walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE {
		t.Fatalf("UNIQUE 反查失败: %v", got)
	}
	if got := redemptionCodeTypeProto(redemptionCodeTypeShared); got != walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED {
		t.Fatalf("SHARED 反查失败: %v", got)
	}
}

func TestRedemptionAdminView(t *testing.T) {
	now := time.Now()
	expiry := now.Add(time.Hour)
	claimedAt := now.Add(-time.Minute)
	claimedBy := uint64(7001)

	row := &ent.RedemptionCode{
		ID:              42,
		Code:            "ABCDEFGHJKLMNPQRST",
		Type:            redemptionCodeTypeUnique,
		Amount:          500,
		Currency:        currencyCNY,
		Status:          redemptionStatusClaimed,
		ExpiresAt:       &expiry,
		RedeemedCount:   1,
		ClaimedByUserID: &claimedBy,
		ClaimedAt:       &claimedAt,
		CreatedBy:       "9001",
		CreatedAt:       now.Add(-time.Hour),
		UpdatedAt:       now,
	}

	view := adminRedemptionCodeView(row, now)
	if view.GetId() != "42" {
		t.Fatalf("id = %q", view.GetId())
	}
	if view.GetCode() != row.Code {
		t.Fatalf("管理员视图必须返回完整明文码")
	}
	if view.GetType() != walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE {
		t.Fatalf("type = %v", view.GetType())
	}
	if view.GetStatus() != walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_CLAIMED {
		t.Fatalf("status = %v", view.GetStatus())
	}
	if view.GetAmount() != 500 || view.GetCurrency() != currencyCNY {
		t.Fatalf("金额/币种映射错误: %d %q", view.GetAmount(), view.GetCurrency())
	}
	if view.GetExpiresAt() != expiry.UnixMilli() {
		t.Fatalf("expires_at = %d", view.GetExpiresAt())
	}
	if view.GetClaimedByUserId() != "7001" || view.GetClaimedAt() != claimedAt.UnixMilli() {
		t.Fatalf("领取信息映射错误: %q %d", view.GetClaimedByUserId(), view.GetClaimedAt())
	}
	if view.GetRedeemedCount() != 1 {
		t.Fatalf("redeemed_count = %d", view.GetRedeemedCount())
	}
	if view.GetCreatedBy() != "9001" {
		t.Fatalf("created_by = %q", view.GetCreatedBy())
	}
}

func TestRedemptionReplayRules(t *testing.T) {
	expiry := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	existing := &ent.RedemptionCode{
		Type:      redemptionCodeTypeUnique,
		Code:      "Vip-2026",
		Amount:    500,
		ExpiresAt: &expiry,
	}

	if err := replayRedemptionCreate(existing, redemptionCodeTypeUnique, "Vip-2026", 500, &expiry); err != nil {
		t.Fatalf("完全相同的重放应成功: %v", err)
	}
	if err := replayRedemptionCreate(existing, redemptionCodeTypeUnique, "Vip-2026", 501, &expiry); err == nil {
		t.Fatalf("金额不同应冲突")
	}
	if err := replayRedemptionCreate(existing, redemptionCodeTypeShared, "Vip-2026", 500, &expiry); err == nil {
		t.Fatalf("类型不同应冲突")
	}
	if err := replayRedemptionCreate(existing, redemptionCodeTypeUnique, "Vip-2026", 500, nil); err == nil {
		t.Fatalf("过期时间不同应冲突")
	}

	// 自动生成请求不比较已生成的明文码：重试必须拿回第一次创建的兑换码。
	if err := replayRedemptionCreate(existing, redemptionCodeTypeUnique, "", 500, &expiry); err != nil {
		t.Fatalf("空码重放应返回首次记录: %v", err)
	}
	// 非空自定义码必须与既有记录逐字节一致。
	for _, requested := range []string{"vip-2026", "Vip2026", "Vip-2026 "} {
		if err := replayRedemptionCreate(existing, redemptionCodeTypeUnique, requested, 500, &expiry); err == nil {
			t.Fatalf("自定义码 %q 与既有码不同，应冲突", requested)
		}
	}

	noExpiry := &ent.RedemptionCode{Type: redemptionCodeTypeShared, Amount: 100}
	if err := replayRedemptionCreate(noExpiry, redemptionCodeTypeShared, "", 100, nil); err != nil {
		t.Fatalf("都无过期时间的重放应成功: %v", err)
	}
}

func TestRedemptionAdminCreateRejectsInvalidCustomCode(t *testing.T) {
	// Commerce 边界独立维护字符集不变量：非 HTTP 调用也必须被拦住。
	handler := testHandler(newPolicyRuntime(t, validSnapshot(7), nil))

	for _, testCase := range []struct {
		name string
		code string
	}{
		{"inner space", "has space"},
		{"leading space", " Vip"},
		{"trailing space", "Vip "},
		{"line break", "line\nbreak"},
		{"tab", "tab\tcode"},
		{"non ascii", "兑换码"},
		{"too long", strings.Repeat("A", redemptionCodeMaxInputLen+1)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			requested := testCase.code
			_, err := handler.AdminCreateRedemptionCode(context.Background(), &walletpb.AdminCreateRedemptionCodeReq{
				OperatorId: "9001",
				RequestId:  "req-invalid-code",
				Type:       walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE,
				Amount:     100,
				Code:       &requested,
			})
			if code := bizCodeOf(t, err); code != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_INVALID_ARGUMENT) {
				t.Fatalf("非法自定义码 %q 应 INVALID_ARGUMENT，实际 code=%d", testCase.code, code)
			}
		})
	}
}

func TestRedemptionUnavailableMessageIsGeneric(t *testing.T) {
	// 不存在、禁用、已领取、过期必须共用同一文案，避免枚举有效码。
	if strings.Contains(redemptionUnavailableMessage, "expired") ||
		strings.Contains(redemptionUnavailableMessage, "claimed") ||
		strings.Contains(redemptionUnavailableMessage, "disabled") {
		t.Fatalf("统一文案不应暴露具体原因: %q", redemptionUnavailableMessage)
	}
}

// 保证测试上下文可用（避免个别构建标签组合下 unused）。
var _ = context.Background
