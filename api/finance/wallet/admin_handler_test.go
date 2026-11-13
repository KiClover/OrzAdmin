package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/internal/middleware"
	walletpb "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/kitex/client/callopt"
	"github.com/cloudwego/kitex/pkg/kerrors"
)

// fakeAdminClient 覆盖管理员 RPC；其余方法由嵌入的 fakeWalletClient 兜底。
type fakeAdminClient struct {
	*fakeWalletClient

	listWalletsCalls int
	listWalletsReq   *walletpb.AdminListWalletsReq
	listWalletsResp  *walletpb.AdminListWalletsResp
	listWalletsErr   error

	getWalletCalls int
	getWalletReq   *walletpb.AdminGetWalletReq
	getWalletResp  *walletpb.AdminGetWalletResp
	getWalletErr   error

	listTransactionsCalls int
	listTransactionsReq   *walletpb.AdminListTransactionsReq
	listTransactionsResp  *walletpb.AdminListTransactionsResp
	listTransactionsErr   error

	listRechargesCalls int
	listRechargesReq   *walletpb.AdminListRechargesReq
	listRechargesResp  *walletpb.AdminListRechargesResp
	listRechargesErr   error

	getRechargeCalls int
	getRechargeReq   *walletpb.AdminGetRechargeReq
	getRechargeResp  *walletpb.AdminGetRechargeResp
	getRechargeErr   error

	refreshCalls int
	refreshReq   *walletpb.AdminRefreshRechargeReq
	refreshResp  *walletpb.AdminRefreshRechargeResp
	refreshErr   error

	refundCalls int
	refundReq   *walletpb.AdminRefundRechargeReq
	refundResp  *walletpb.AdminRefundRechargeResp
	refundErr   error

	createCodeCalls int
	createCodeReq   *walletpb.AdminCreateRedemptionCodeReq
	createCodeResp  *walletpb.AdminCreateRedemptionCodeResp
	createCodeErr   error

	listCodesCalls int
	listCodesReq   *walletpb.AdminListRedemptionCodesReq
	listCodesResp  *walletpb.AdminListRedemptionCodesResp
	listCodesErr   error

	setStatusCalls int
	setStatusReq   *walletpb.AdminSetRedemptionCodeStatusReq
	setStatusResp  *walletpb.AdminSetRedemptionCodeStatusResp
	setStatusErr   error

	adjustCalls int
	adjustReq   *walletpb.AdminAdjustBalanceReq
	adjustResp  *walletpb.AdminAdjustBalanceResp
	adjustErr   error
}

func newFakeAdminClient() *fakeAdminClient {
	return &fakeAdminClient{fakeWalletClient: &fakeWalletClient{}}
}

func (f *fakeAdminClient) AdminListWallets(ctx context.Context, req *walletpb.AdminListWalletsReq, opts ...callopt.Option) (*walletpb.AdminListWalletsResp, error) {
	f.listWalletsCalls++
	f.listWalletsReq = req
	return orEmpty(f.listWalletsResp, f.listWalletsErr)
}

func (f *fakeAdminClient) AdminGetWallet(ctx context.Context, req *walletpb.AdminGetWalletReq, opts ...callopt.Option) (*walletpb.AdminGetWalletResp, error) {
	f.getWalletCalls++
	f.getWalletReq = req
	return orEmpty(f.getWalletResp, f.getWalletErr)
}

func (f *fakeAdminClient) AdminListTransactions(ctx context.Context, req *walletpb.AdminListTransactionsReq, opts ...callopt.Option) (*walletpb.AdminListTransactionsResp, error) {
	f.listTransactionsCalls++
	f.listTransactionsReq = req
	return orEmpty(f.listTransactionsResp, f.listTransactionsErr)
}

func (f *fakeAdminClient) AdminListRecharges(ctx context.Context, req *walletpb.AdminListRechargesReq, opts ...callopt.Option) (*walletpb.AdminListRechargesResp, error) {
	f.listRechargesCalls++
	f.listRechargesReq = req
	return orEmpty(f.listRechargesResp, f.listRechargesErr)
}

func (f *fakeAdminClient) AdminGetRecharge(ctx context.Context, req *walletpb.AdminGetRechargeReq, opts ...callopt.Option) (*walletpb.AdminGetRechargeResp, error) {
	f.getRechargeCalls++
	f.getRechargeReq = req
	return orEmpty(f.getRechargeResp, f.getRechargeErr)
}

func (f *fakeAdminClient) AdminRefreshRecharge(ctx context.Context, req *walletpb.AdminRefreshRechargeReq, opts ...callopt.Option) (*walletpb.AdminRefreshRechargeResp, error) {
	f.refreshCalls++
	f.refreshReq = req
	return orEmpty(f.refreshResp, f.refreshErr)
}

func (f *fakeAdminClient) AdminRefundRecharge(ctx context.Context, req *walletpb.AdminRefundRechargeReq, opts ...callopt.Option) (*walletpb.AdminRefundRechargeResp, error) {
	f.refundCalls++
	f.refundReq = req
	return orEmpty(f.refundResp, f.refundErr)
}

func (f *fakeAdminClient) AdminCreateRedemptionCode(ctx context.Context, req *walletpb.AdminCreateRedemptionCodeReq, opts ...callopt.Option) (*walletpb.AdminCreateRedemptionCodeResp, error) {
	f.createCodeCalls++
	f.createCodeReq = req
	return orEmpty(f.createCodeResp, f.createCodeErr)
}

func (f *fakeAdminClient) AdminListRedemptionCodes(ctx context.Context, req *walletpb.AdminListRedemptionCodesReq, opts ...callopt.Option) (*walletpb.AdminListRedemptionCodesResp, error) {
	f.listCodesCalls++
	f.listCodesReq = req
	return orEmpty(f.listCodesResp, f.listCodesErr)
}

func (f *fakeAdminClient) AdminSetRedemptionCodeStatus(ctx context.Context, req *walletpb.AdminSetRedemptionCodeStatusReq, opts ...callopt.Option) (*walletpb.AdminSetRedemptionCodeStatusResp, error) {
	f.setStatusCalls++
	f.setStatusReq = req
	return orEmpty(f.setStatusResp, f.setStatusErr)
}

func (f *fakeAdminClient) AdminAdjustBalance(ctx context.Context, req *walletpb.AdminAdjustBalanceReq, opts ...callopt.Option) (*walletpb.AdminAdjustBalanceResp, error) {
	f.adjustCalls++
	f.adjustReq = req
	return orEmpty(f.adjustResp, f.adjustErr)
}

// staticCodeGuard 用固定结果替代 Casbin 明文检查。
type staticCodeGuard struct {
	allowed bool
	err     error
}

func (g staticCodeGuard) Allowed(string) (bool, error) {
	return g.allowed, g.err
}

// adminRoutePolicies 与 services/system/access/seed.go 中的注册条目一致。
func adminRoutePolicies() [][2]string {
	return [][2]string{
		{"/admin/wallets/list", http.MethodPost},
		{"/admin/wallets/get", http.MethodPost},
		{"/admin/transactions/list", http.MethodPost},
		{"/admin/recharges/list", http.MethodPost},
		{"/admin/recharges/:rechargeId", http.MethodGet},
		{"/admin/recharges/:rechargeId/refresh", http.MethodPost},
		{"/admin/recharges/:rechargeId/refund", http.MethodPost},
		{"/admin/redemption-codes", http.MethodPost},
		{"/admin/redemption-codes/list", http.MethodPost},
		{"/admin/redemption-codes/:codeId/status", http.MethodPost},
		{"/admin/wallets/adjustments", http.MethodPost},
	}
}

// newAdminEnforcer 授予管理员路由权限，但不授予兑换码明文权限。
func newAdminEnforcer(t *testing.T) *casbin.Enforcer {
	t.Helper()
	enforcer := newTestEnforcer(t)
	for _, policy := range adminRoutePolicies() {
		if _, err := enforcer.AddPolicy("admin", policy[0], policy[1]); err != nil {
			t.Fatalf("add route policy %v: %v", policy, err)
		}
	}
	if _, err := enforcer.AddRoleForUser(testAdminSubject, "admin"); err != nil {
		t.Fatalf("bind admin role: %v", err)
	}
	return enforcer
}

type adminTestEnv struct {
	server *server.Hertz
	client *fakeAdminClient
	audit  *recordingAuditSink
}

// newAdminTestEnv 构造带管理员路由权限的测试服务。
func newAdminTestEnv(t *testing.T, guard plaintextCodeGuard) *adminTestEnv {
	t.Helper()
	cfg := testConfig()
	client := newFakeAdminClient()
	audit := &recordingAuditSink{}
	svc := &HzFinanceWalletService{
		Cfg:          cfg,
		WalletClient: client,
		Enforcer:     newAdminEnforcer(t),
		Codes:        guard,
		Audit:        audit,
	}
	return &adminTestEnv{server: newTestServerWithService(t, cfg, svc), client: client, audit: audit}
}

// perform 以管理员身份发起请求。
func (e *adminTestEnv) perform(t *testing.T, cfg *conf.Config, method, path string, body *ut.Body, extra ...ut.Header) *ut.ResponseRecorder {
	t.Helper()
	headers := append([]ut.Header{{Key: "Authorization", Value: bearerToken(t, cfg, 9001)}}, extra...)
	return ut.PerformRequest(e.server.Engine, method, path, body, headers...)
}

// decodeData 解析 {code,msg,data} 信封里的 data。
func decodeData(t *testing.T, w *ut.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Code int32           `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v (%s)", err, w.Body.String())
	}
	if envelope.Code != 0 {
		t.Fatalf("expected success envelope, got code=%d msg=%s", envelope.Code, envelope.Msg)
	}
	data := map[string]any{}
	if len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			t.Fatalf("decode data: %v (%s)", err, string(envelope.Data))
		}
	}
	return data
}

// decodeError 解析错误信封。
func decodeError(t *testing.T, w *ut.ResponseRecorder) (int32, string) {
	t.Helper()
	var envelope struct {
		Code int32  `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v (%s)", err, w.Body.String())
	}
	return envelope.Code, envelope.Msg
}

func adminRechargePB() *walletpb.AdminRechargeView {
	return &walletpb.AdminRechargeView{
		Recharge: &walletpb.RechargeView{
			Id:               "42",
			Status:           walletpb.RechargeStatus_RECHARGE_STATUS_SUCCEEDED,
			WalletAmount:     10000,
			WalletCurrency:   "CNY",
			ProviderAmount:   1389,
			ProviderCurrency: "USD",
			CnyPerUsdRate:    "7.200000",
			CheckoutUrl:      "https://checkout.creem.io/chk_1",
			CreatedAt:        1_700_000_000_000,
			PaidAt:           1_700_000_060_000,
			UpdatedAt:        1_700_000_060_000,
		},
		UserId:                 "7001",
		WalletId:               "500",
		RequestId:              "req-1",
		MerchantOrderNo:        "mo-1",
		Provider:               walletpb.PaymentProvider_PAYMENT_PROVIDER_CREEM,
		ProviderEnvironment:    walletpb.ProviderEnvironment_PROVIDER_ENVIRONMENT_TEST,
		PaymentProfileId:       "wp_test_1",
		PaymentProfileRevision: 7,
		ProviderCheckoutId:     "chk_1",
		ProviderTransactionId:  "txn_1",
		FxRateNumerator:        720,
		FxRateDenominator:      100,
		FxSettingRevision:      11,
		FxRoundingMode:         "CEIL_PROVIDER_MINOR_UNIT",
		ReviewBlocksDebit:      false,
		Revision:               2,
	}
}

func adminCodePB(code string) *walletpb.AdminRedemptionCodeView {
	return &walletpb.AdminRedemptionCodeView{
		Id:            "7",
		Code:          code,
		Type:          walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE,
		Amount:        500,
		Currency:      "CNY",
		Status:        walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE,
		RedeemedCount: 0,
		CreatedBy:     "9001",
		CreatedAt:     1_700_000_000_000,
		UpdatedAt:     1_700_000_000_000,
	}
}

func TestAdminListWalletsForwardsFilters(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.listWalletsResp = &walletpb.AdminListWalletsResp{
		Items: []*walletpb.AdminWalletView{{
			Wallet: &walletpb.WalletView{
				Currency:      "CNY",
				BalanceAmount: 1500,
				Status:        walletpb.WalletStatus_WALLET_STATUS_ACTIVE,
				Revision:      3,
				UpdatedAt:     1_700_000_000_000,
			},
			UserId:   "7001",
			WalletId: "500",
		}},
		NextCursor: "cursor-2",
		HasMore:    true,
	}

	body, contentType := jsonBody(`{"user_id":"7001","status":"ACTIVE","page_size":20,"cursor":"c1"}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/wallets/list", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	req := env.client.listWalletsReq
	if req.GetUserId() != "7001" || req.GetStatus() != walletpb.WalletStatus_WALLET_STATUS_ACTIVE {
		t.Fatalf("unexpected filters: user=%q status=%v", req.GetUserId(), req.GetStatus())
	}
	if req.GetPageSize() != 20 || req.GetCursor() != "c1" {
		t.Fatalf("unexpected pagination: %d %q", req.GetPageSize(), req.GetCursor())
	}
	data := decodeData(t, w)
	wallets, _ := data["wallets"].([]any)
	if len(wallets) != 1 {
		t.Fatalf("expected one wallet, got %v", data["wallets"])
	}
	wallet := wallets[0].(map[string]any)
	if wallet["user_id"] != "7001" || wallet["status"] != "ACTIVE" {
		t.Fatalf("unexpected wallet payload: %v", wallet)
	}
	if wallet["wallet_id"] != "500" {
		t.Fatalf("admin wallet view must expose wallet_id, got %v", wallet["wallet_id"])
	}
	if wallet["balance_amount"] != float64(1500) || wallet["revision"] != float64(3) {
		t.Fatalf("unexpected wallet amounts: %v", wallet)
	}
	if data["has_more"] != true || data["next_cursor"] != "cursor-2" {
		t.Fatalf("unexpected pagination payload: %v", data)
	}
	if got := w.Header().Peek("Cache-Control"); len(got) != 0 {
		t.Fatalf("wallet list must not be marked no-store, got %q", got)
	}
}

func TestAdminListWalletsRejectsInvalidFilters(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})

	cases := []string{
		`{"status":"BOGUS"}`,
		`{"user_id":"007"}`,
		`{"page_size":101}`,
		`{"page_size":-1}`,
	}
	for _, raw := range cases {
		body, contentType := jsonBody(raw)
		w := env.perform(t, testConfig(), http.MethodPost, "/admin/wallets/list", body, contentType)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", raw, w.Code, w.Body.String())
		}
	}
	if env.client.listWalletsCalls != 0 {
		t.Fatalf("invalid filters must not reach the RPC, got %d calls", env.client.listWalletsCalls)
	}
}

func TestAdminGetWalletRejectsInvalidPathID(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})

	for _, raw := range []string{`{"wallet_id":"0abc"}`, `{"wallet_id":"007"}`, `{"wallet_id":"-1"}`, `{}`} {
		body, contentType := jsonBody(raw)
		w := env.perform(t, testConfig(), http.MethodPost, "/admin/wallets/get", body, contentType)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", raw, w.Code, w.Body.String())
		}
	}
	if env.client.getWalletCalls != 0 {
		t.Fatalf("invalid path must not reach the RPC, got %d calls", env.client.getWalletCalls)
	}
}

func TestAdminListTransactionsForwardsFilters(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.listTransactionsResp = &walletpb.AdminListTransactionsResp{
		Items: []*walletpb.AdminTransactionView{{
			Transaction: &walletpb.TransactionView{
				Id:           "31",
				Type:         walletpb.TransactionType_TRANSACTION_TYPE_RECHARGE_CREDIT,
				Currency:     "CNY",
				DeltaAmount:  10000,
				BalanceAfter: 10000,
				CreatedAt:    1_700_000_000_000,
			},
			WalletId:       "500",
			UserId:         "7001",
			OwnerNamespace: "commerce",
			BusinessType:   "recharge",
			BusinessId:     "42",
			Actor:          "9001",
		}},
	}

	body, contentType := jsonBody(`{"wallet_id":"500","user_id":"7001","type":"RECHARGE_CREDIT","start_time":100,"end_time":200,"page_size":10}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/transactions/list", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	req := env.client.listTransactionsReq
	if req.GetWalletId() != "500" || req.GetUserId() != "7001" {
		t.Fatalf("unexpected identity filters: %q %q", req.GetWalletId(), req.GetUserId())
	}
	if req.GetType() != walletpb.TransactionType_TRANSACTION_TYPE_RECHARGE_CREDIT {
		t.Fatalf("unexpected type: %v", req.GetType())
	}
	if req.GetStartTime() != 100 || req.GetEndTime() != 200 || req.GetPageSize() != 10 {
		t.Fatalf("unexpected range: %v %v %v", req.GetStartTime(), req.GetEndTime(), req.GetPageSize())
	}

	data := decodeData(t, w)
	items, _ := data["transactions"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one transaction, got %v", data["transactions"])
	}
	item := items[0].(map[string]any)
	if item["wallet_id"] != "500" || item["business_type"] != "recharge" || item["actor"] != "9001" {
		t.Fatalf("unexpected admin transaction payload: %v", item)
	}

	badBody, badType := jsonBody(`{"wallet_id":"500","type":"BOGUS"}`)
	bad := env.perform(t, testConfig(), http.MethodPost, "/admin/transactions/list", badBody, badType)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown type, got %d", bad.Code)
	}
}

func TestAdminListRechargesForwardsFilters(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.listRechargesResp = &walletpb.AdminListRechargesResp{Items: []*walletpb.AdminRechargeView{adminRechargePB()}}

	body, contentType := jsonBody(`{"user_id":"7001","status":"SUCCEEDED","provider":"CREEM","page_size":5}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/recharges/list", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	req := env.client.listRechargesReq
	if req.GetUserId() != "7001" {
		t.Fatalf("unexpected user filter: %q", req.GetUserId())
	}
	if req.GetStatus() != walletpb.RechargeStatus_RECHARGE_STATUS_SUCCEEDED {
		t.Fatalf("unexpected status: %v", req.GetStatus())
	}
	if req.GetProvider() != walletpb.PaymentProvider_PAYMENT_PROVIDER_CREEM {
		t.Fatalf("unexpected provider: %v", req.GetProvider())
	}
	if req.GetPageSize() != 5 {
		t.Fatalf("unexpected page size: %d", req.GetPageSize())
	}

	badBody, badType := jsonBody(`{"provider":"STRIPE"}`)
	bad := env.perform(t, testConfig(), http.MethodPost, "/admin/recharges/list", badBody, badType)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown provider, got %d", bad.Code)
	}
}

func TestAdminGetRechargeMapsProviderFields(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.getRechargeResp = &walletpb.AdminGetRechargeResp{Recharge: adminRechargePB()}

	w := env.perform(t, testConfig(), http.MethodGet, "/admin/recharges/42", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if env.client.getRechargeReq.GetRechargeId() != "42" {
		t.Fatalf("unexpected recharge id: %q", env.client.getRechargeReq.GetRechargeId())
	}

	data := decodeData(t, w)
	recharge, _ := data["recharge"].(map[string]any)
	if recharge["provider"] != "CREEM" || recharge["provider_environment"] != "TEST" {
		t.Fatalf("unexpected provider payload: %v", recharge)
	}
	if recharge["wallet_amount"] != float64(10000) || recharge["provider_amount"] != float64(1389) {
		t.Fatalf("unexpected amounts: %v", recharge)
	}
	if recharge["payment_profile_revision"] != float64(7) || recharge["request_id"] != "req-1" {
		t.Fatalf("unexpected profile payload: %v", recharge)
	}
	if recharge["checkout_url"] != "https://checkout.creem.io/chk_1" {
		t.Fatalf("unexpected checkout url: %v", recharge["checkout_url"])
	}
}

func TestAdminRefreshRechargeUsesTokenIdentity(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.refreshResp = &walletpb.AdminRefreshRechargeResp{Recharge: adminRechargePB()}

	body, contentType := jsonBody(`{"reason":"checked provider dashboard","confirm_provider_not_created":true,"provider_checkout_id":"chk_9","operator_id":"666"}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/recharges/42/refresh", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	req := env.client.refreshReq
	if req.GetOperatorId() != "9001" {
		t.Fatalf("operator must come from the token, got %q", req.GetOperatorId())
	}
	if req.GetReason() != "checked provider dashboard" || !req.GetConfirmProviderNotCreated() {
		t.Fatalf("unexpected refresh payload: %q %v", req.GetReason(), req.GetConfirmProviderNotCreated())
	}
	if req.GetProviderCheckoutId() != "chk_9" {
		t.Fatalf("unexpected provider checkout id: %q", req.GetProviderCheckoutId())
	}

	missingBody, missingType := jsonBody(`{"reason":"   "}`)
	missing := env.perform(t, testConfig(), http.MethodPost, "/admin/recharges/42/refresh", missingBody, missingType)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without reason, got %d: %s", missing.Code, missing.Body.String())
	}
	if env.client.refreshCalls != 1 {
		t.Fatalf("invalid refresh must not reach the RPC, got %d calls", env.client.refreshCalls)
	}
}

func TestAdminRefundRechargeRequiresReason(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.refundResp = &walletpb.AdminRefundRechargeResp{Recharge: adminRechargePB()}

	body, contentType := jsonBody(`{"reason":"customer reported duplicate charge"}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/recharges/42/refund", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	req := env.client.refundReq
	if req.GetOperatorId() != "9001" || req.GetRechargeId() != "42" {
		t.Fatalf("unexpected refund identity: %q %q", req.GetOperatorId(), req.GetRechargeId())
	}
	if req.GetReason() != "customer reported duplicate charge" {
		t.Fatalf("unexpected refund reason: %q", req.GetReason())
	}

	missingBody, missingType := jsonBody(`{}`)
	missing := env.perform(t, testConfig(), http.MethodPost, "/admin/recharges/42/refund", missingBody, missingType)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without reason, got %d", missing.Code)
	}
	if env.client.refundCalls != 1 {
		t.Fatalf("invalid refund must not reach the RPC, got %d calls", env.client.refundCalls)
	}
}

func TestAdminCreateRedemptionCodeRequiresPlaintextPermission(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: false})
	env.client.createCodeResp = &walletpb.AdminCreateRedemptionCodeResp{Code: adminCodePB("SECRET-CODE-001")}

	body, contentType := jsonBody(`{"request_id":"req-1","type":"UNIQUE","amount":500}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes", body, contentType)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if env.client.createCodeCalls != 0 {
		t.Fatalf("denied request must not reach the RPC, got %d calls", env.client.createCodeCalls)
	}
	if strings.Contains(w.Body.String(), "SECRET-CODE-001") {
		t.Fatal("denied response must not leak the plaintext code")
	}
	event := env.audit.last()
	if event == nil || !event.Denied || event.Action != walletCodeActionCreate {
		t.Fatalf("unexpected denial audit: %+v", event)
	}
	if event.Reason != "plaintext permission missing" {
		t.Fatalf("unexpected denial reason: %q", event.Reason)
	}
}

func TestAdminCreateRedemptionCodeReturnsPlaintextWithNoStore(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.createCodeResp = &walletpb.AdminCreateRedemptionCodeResp{Code: adminCodePB("SECRET-CODE-001")}

	body, contentType := jsonBody(`{"request_id":"req-1","type":"UNIQUE","amount":500}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := string(w.Header().Peek("Cache-Control")); got != "no-store" {
		t.Fatalf("plaintext responses must set no-store, got %q", got)
	}
	if !strings.Contains(w.Body.String(), "SECRET-CODE-001") {
		t.Fatalf("expected plaintext code in response: %s", w.Body.String())
	}
	req := env.client.createCodeReq
	if req.GetOperatorId() != "9001" || req.GetRequestId() != "req-1" {
		t.Fatalf("unexpected create payload: %q %q", req.GetOperatorId(), req.GetRequestId())
	}
	if req.GetType() != walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_UNIQUE || req.GetAmount() != 500 {
		t.Fatalf("unexpected create type/amount: %v %d", req.GetType(), req.GetAmount())
	}
	if got := req.GetCode(); got != "" {
		t.Fatalf("missing code must request automatic generation, got %q", got)
	}
	event := env.audit.last()
	if event == nil || event.Denied || event.Action != walletCodeActionCreate {
		t.Fatalf("unexpected success audit: %+v", event)
	}
	if event.RequestID != "req-1" || event.Count != 1 {
		t.Fatalf("unexpected audit metadata: %+v", event)
	}

	// 缺少必填字段时为 400，且不再产生兑换码。
	badBody, badType := jsonBody(`{"request_id":"req-2","type":"BOGUS","amount":500}`)
	bad := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes", badBody, badType)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown type, got %d", bad.Code)
	}
	if env.client.createCodeCalls != 1 {
		t.Fatalf("invalid create must not reach the RPC, got %d calls", env.client.createCodeCalls)
	}
}

func TestAdminCreateRedemptionCodeForwardsExactCustomCode(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.createCodeResp = &walletpb.AdminCreateRedemptionCodeResp{Code: adminCodePB("Vip-2026")}

	body, contentType := jsonBody(`{"request_id":"req-custom","type":"UNIQUE","amount":500,"code":"Vip-2026"}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if env.client.createCodeCalls != 1 {
		t.Fatalf("expected one RPC call, got %d", env.client.createCodeCalls)
	}
	if got := env.client.createCodeReq.GetCode(); got != "Vip-2026" {
		t.Fatalf("custom code changed in transit: %q", got)
	}
	if !strings.Contains(w.Body.String(), "Vip-2026") {
		t.Fatalf("response must echo the exact plaintext code: %s", w.Body.String())
	}
}

func TestAdminCreateRedemptionCodeForwardsMaxRedemptions(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.createCodeResp = &walletpb.AdminCreateRedemptionCodeResp{Code: adminCodePB("SECRET-CODE-001")}

	body, contentType := jsonBody(`{"request_id":"req-cap","type":"SHARED","amount":100,"max_redemptions":3}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if env.client.createCodeCalls != 1 {
		t.Fatalf("expected one RPC call, got %d", env.client.createCodeCalls)
	}
	if got := env.client.createCodeReq.GetMaxRedemptions(); got != 3 {
		t.Fatalf("max_redemptions changed in transit: %d", got)
	}

	body, contentType = jsonBody(`{"request_id":"req-no-cap","type":"SHARED","amount":100}`)
	w = env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := env.client.createCodeReq.GetMaxRedemptions(); got != 0 {
		t.Fatalf("missing max_redemptions must default to unlimited, got %d", got)
	}
}

func TestAdminCreateRedemptionCodeRejectsNegativeMaxRedemptions(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.createCodeResp = &walletpb.AdminCreateRedemptionCodeResp{Code: adminCodePB("SECRET-CODE-001")}

	body, contentType := jsonBody(`{"request_id":"req-cap-neg","type":"SHARED","amount":100,"max_redemptions":-1}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes", body, contentType)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if env.client.createCodeCalls != 0 {
		t.Fatalf("invalid max_redemptions must not reach the RPC, got %d calls", env.client.createCodeCalls)
	}
}

func TestAdminCreateRedemptionCodeAcceptsVisibleASCIIBoundaries(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{name: "single_byte", code: "!"},
		{name: "max_length", code: strings.Repeat("~", 128)},
		{name: "short_code", code: "a-b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
			env.client.createCodeResp = &walletpb.AdminCreateRedemptionCodeResp{Code: adminCodePB(tc.code)}

			raw, err := json.Marshal(map[string]any{
				"request_id": "req-boundary",
				"type":       "UNIQUE",
				"amount":     500,
				"code":       tc.code,
			})
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes",
				&ut.Body{Body: strings.NewReader(string(raw)), Len: len(raw)},
				ut.Header{Key: "Content-Type", Value: "application/json"})
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			if got := env.client.createCodeReq.GetCode(); got != tc.code {
				t.Fatalf("code changed in transit: got %q want %q", got, tc.code)
			}
		})
	}
}

func TestAdminCreateRedemptionCodeTreatsEmptyCodeAsAutomatic(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.createCodeResp = &walletpb.AdminCreateRedemptionCodeResp{Code: adminCodePB("SECRET-CODE-001")}

	body, contentType := jsonBody(`{"request_id":"req-empty","type":"UNIQUE","amount":500,"code":""}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if env.client.createCodeCalls != 1 {
		t.Fatalf("expected one RPC call, got %d", env.client.createCodeCalls)
	}
	if got := env.client.createCodeReq.GetCode(); got != "" {
		t.Fatalf("empty code must mean automatic generation, got %q", got)
	}
}

func TestAdminCreateRedemptionCodeRejectsInvalidCustomCode(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{name: "inner_space", code: "has space"},
		{name: "leading_space", code: " VIP"},
		{name: "trailing_space", code: "VIP "},
		{name: "line_break", code: "line\nbreak"},
		{name: "tab", code: "tab\tcode"},
		{name: "non_ascii", code: "兑换码"},
		{name: "too_long", code: strings.Repeat("A", 129)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
			raw, err := json.Marshal(map[string]any{
				"request_id": "req-invalid",
				"type":       "UNIQUE",
				"amount":     500,
				"code":       tc.code,
			})
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes",
				&ut.Body{Body: strings.NewReader(string(raw)), Len: len(raw)},
				ut.Header{Key: "Content-Type", Value: "application/json"})
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %q, got %d: %s", tc.code, w.Code, w.Body.String())
			}
			if env.client.createCodeCalls != 0 {
				t.Fatalf("invalid code %q reached Commerce", tc.code)
			}
		})
	}
}

func TestAdminListRedemptionCodesAuditsFilters(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.listCodesResp = &walletpb.AdminListRedemptionCodesResp{
		Items: []*walletpb.AdminRedemptionCodeView{adminCodePB("SECRET-CODE-002")},
	}

	body, contentType := jsonBody(`{"type":"SHARED","status":"ACTIVE","page_size":10}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes/list", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := string(w.Header().Peek("Cache-Control")); got != "no-store" {
		t.Fatalf("plaintext list must set no-store, got %q", got)
	}
	req := env.client.listCodesReq
	if req.GetType() != walletpb.RedemptionCodeType_REDEMPTION_CODE_TYPE_SHARED ||
		req.GetStatus() != walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_ACTIVE {
		t.Fatalf("unexpected filters: %v %v", req.GetType(), req.GetStatus())
	}
	event := env.audit.last()
	if event == nil || event.Action != walletCodeActionList || event.Count != 1 {
		t.Fatalf("unexpected audit event: %+v", event)
	}
	if event.Filters["type"] != "SHARED" || event.Filters["status"] != "ACTIVE" {
		t.Fatalf("unexpected audit filters: %v", event.Filters)
	}
	data := decodeData(t, w)
	codes, _ := data["codes"].([]any)
	if len(codes) != 1 || codes[0].(map[string]any)["code"] != "SECRET-CODE-002" {
		t.Fatalf("unexpected code payload: %v", data["codes"])
	}
}

func TestAdminListRedemptionCodesReturnsMaxRedemptions(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.listCodesResp = &walletpb.AdminListRedemptionCodesResp{
		Items: []*walletpb.AdminRedemptionCodeView{{
			Id:             "1",
			Code:           "CAP-CODE",
			MaxRedemptions: 3,
		}},
	}

	body, contentType := jsonBody(`{"page_size":10}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes/list", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	data := decodeData(t, w)
	codes, _ := data["codes"].([]any)
	if len(codes) != 1 {
		t.Fatalf("unexpected codes payload: %v", data["codes"])
	}
	if got := codes[0].(map[string]any)["max_redemptions"]; got != float64(3) {
		t.Fatalf("max_redemptions = %v, want 3", got)
	}
}

func TestAdminSetRedemptionCodeStatusRejectsUnsupportedStatus(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.setStatusResp = &walletpb.AdminSetRedemptionCodeStatusResp{Code: adminCodePB("SECRET-CODE-003")}

	body, contentType := jsonBody(`{"status":"CLAIMED"}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes/7/status", body, contentType)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for CLAIMED, got %d: %s", w.Code, w.Body.String())
	}
	if env.client.setStatusCalls != 0 {
		t.Fatalf("unsupported status must not reach the RPC")
	}

	body, contentType = jsonBody(`{"status":"DISABLED","reason":"fraud"}`)
	w = env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes/7/status", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	req := env.client.setStatusReq
	if req.GetStatus() != walletpb.RedemptionCodeStatus_REDEMPTION_CODE_STATUS_DISABLED {
		t.Fatalf("unexpected status: %v", req.GetStatus())
	}
	if req.GetCodeId() != "7" || req.GetOperatorId() != "9001" || req.GetReason() != "fraud" {
		t.Fatalf("unexpected status payload: %+v", req)
	}
	event := env.audit.last()
	if event == nil || event.Action != walletCodeActionStatus || event.Filters["code_id"] != "7" {
		t.Fatalf("unexpected audit event: %+v", event)
	}
	if got := string(w.Header().Peek("Cache-Control")); got != "no-store" {
		t.Fatalf("status response must set no-store, got %q", got)
	}
}

func TestAdminPlaintextGuardErrorReturnsUnavailable(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{err: errors.New("casbin is down")})
	body, contentType := jsonBody(`{"request_id":"req-1","type":"UNIQUE","amount":500}`)

	w := env.perform(t, testConfig(), http.MethodPost, "/admin/redemption-codes", body, contentType)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	if env.client.createCodeCalls != 0 {
		t.Fatalf("guard failure must not reach the RPC")
	}
	event := env.audit.last()
	if event == nil || !event.Denied || event.Reason != "authorization check failed" {
		t.Fatalf("unexpected audit event: %+v", event)
	}
}

func TestAdminMissingGuardFailsClosed(t *testing.T) {
	// 直接装配而不经过 newTestServerWithService：保留 Codes 为 nil，
	// 验证服务在缺少明文检查组件时同样拒绝。
	cfg := testConfig()
	client := newFakeAdminClient()
	audit := &recordingAuditSink{}
	rateMw, err := middleware.NewRateMw(cfg, nil)
	if err != nil {
		t.Fatalf("build rate middleware: %v", err)
	}
	svc := &HzFinanceWalletService{
		Cfg:          cfg,
		WalletClient: client,
		Enforcer:     newAdminEnforcer(t),
		RateMw:       rateMw,
		Audit:        audit,
	}
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	NewRegisterRouter(h, svc)

	body, contentType := jsonBody(`{"request_id":"req-1","type":"UNIQUE","amount":500}`)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/admin/redemption-codes", body,
		ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 9001)}, contentType)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if client.createCodeCalls != 0 {
		t.Fatalf("missing guard must not reach the RPC")
	}
	event := audit.last()
	if event == nil || event.Reason != "plaintext guard is not configured" {
		t.Fatalf("unexpected audit event: %+v", event)
	}
}

func TestAdminRPCMapsBusinessErrorsToHTTP(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.getRechargeErr = kerrors.NewBizStatusError(
		int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND), "recharge 42 does not exist")

	w := env.perform(t, testConfig(), http.MethodGet, "/admin/recharges/42", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "recharge 42 does not exist") {
		t.Fatalf("business error detail must not leak: %s", w.Body.String())
	}
	code, message := decodeError(t, w)
	if code != int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_NOT_FOUND) {
		t.Fatalf("unexpected error code: %d", code)
	}
	if message != "wallet resource not found" {
		t.Fatalf("unexpected error message: %q", message)
	}
}

func TestAdminRequiresRoutePolicy(t *testing.T) {
	cfg := testConfig()
	client := newFakeAdminClient()
	svc := &HzFinanceWalletService{
		Cfg:          cfg,
		WalletClient: client,
		Enforcer:     newTestEnforcer(t), // 没有任何路由策略
		Codes:        staticCodeGuard{allowed: true},
		Audit:        &recordingAuditSink{},
	}
	h := newTestServerWithService(t, cfg, svc)

	body, contentType := jsonBody(`{"status":"ACTIVE"}`)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/admin/wallets/list", body,
		ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 9001)}, contentType)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without route policy, got %d: %s", w.Code, w.Body.String())
	}
	if client.listWalletsCalls != 0 {
		t.Fatalf("unauthorized request must not reach the RPC")
	}
}

// 保证测试辅助使用的时间值与 handler 的毫秒格式一致。
func TestAdminTimeRenderingIsRFC3339Millis(t *testing.T) {
	if got := time.UnixMilli(1_700_000_000_000).UTC().Format(time.RFC3339); got == "" {
		t.Fatal("sanity check failed")
	}
}

// TestAdminAdjustmentTypeRoundTripsThroughFilters 保证流水筛选与展示都能识别新类型：
// 查询参数 ADMIN_ADJUSTMENT 映射到枚举 6，展示时再还原成同名字符串。
func TestAdminAdjustmentTypeRoundTripsThroughFilters(t *testing.T) {
	parsed, ok := parseTransactionType("ADMIN_ADJUSTMENT")
	if !ok || parsed == nil {
		t.Fatal("ADMIN_ADJUSTMENT must be a valid transaction type filter")
	}
	if *parsed != walletpb.TransactionType_TRANSACTION_TYPE_ADMIN_ADJUSTMENT {
		t.Fatalf("unexpected enum: %v", *parsed)
	}
	if got := transactionTypeName(*parsed); got != "ADMIN_ADJUSTMENT" {
		t.Fatalf("transactionTypeName must render ADMIN_ADJUSTMENT, got %q", got)
	}
	if _, ok := parseTransactionType("BOGUS"); ok {
		t.Fatal("unknown type must be rejected")
	}
}

// adminAdjustViewPB 构造一条管理员调账流水视图。
func adminAdjustViewPB(userID, actor string, delta int64) *walletpb.AdminTransactionView {
	return &walletpb.AdminTransactionView{
		Transaction: &walletpb.TransactionView{
			Id:           "1001",
			Type:         walletpb.TransactionType_TRANSACTION_TYPE_ADMIN_ADJUSTMENT,
			Currency:     "CNY",
			DeltaAmount:  delta,
			BalanceAfter: delta,
			Description:  "manual correction",
			CreatedAt:    1_700_000_000_000,
		},
		WalletId:     "7",
		UserId:       userID,
		BusinessType: "admin_adjustment",
		BusinessId:   "req-1",
		Actor:        actor,
	}
}

// TestAdminAdjustBalanceForwardsSignedDelta 覆盖成功路径：
// 请求侧去掉首尾空白后原样下发，操作人来自 JWT，响应只含原始流水。
func TestAdminAdjustBalanceForwardsSignedDelta(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.adjustResp = &walletpb.AdminAdjustBalanceResp{Transaction: adminAdjustViewPB("42", "admin:9001", -250)}

	body, contentType := jsonBody(`{"user_id":" 42 ","request_id":" req-1 ","delta_amount":-250,"reason":" manual correction "}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/wallets/adjustments", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if env.client.adjustCalls != 1 {
		t.Fatalf("expected exactly one RPC call, got %d", env.client.adjustCalls)
	}
	req := env.client.adjustReq
	if req.GetOperatorId() != "9001" || req.GetUserId() != "42" ||
		req.GetRequestId() != "req-1" || req.GetDeltaAmount() != -250 || req.GetReason() != "manual correction" {
		t.Fatalf("unexpected adjustment payload: %+v", req)
	}

	data := decodeData(t, w)
	transaction, ok := data["transaction"].(map[string]any)
	if !ok {
		t.Fatalf("missing transaction in response: %v", data)
	}
	if transaction["id"] != "1001" || transaction["type"] != "ADMIN_ADJUSTMENT" ||
		transaction["delta_amount"] != float64(-250) || transaction["actor"] != "admin:9001" ||
		transaction["user_id"] != "42" || transaction["business_id"] != "req-1" ||
		transaction["business_type"] != "admin_adjustment" {
		t.Fatalf("unexpected transaction payload: %v", transaction)
	}
}

// TestAdminAdjustBalanceIgnoresBodyIdentity 覆盖 §12.1：
// 请求体里伪造的 operator_id/actor 不生效，操作人只能是 JWT 主体。
func TestAdminAdjustBalanceIgnoresBodyIdentity(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.adjustResp = &walletpb.AdminAdjustBalanceResp{Transaction: adminAdjustViewPB("42", "admin:9001", 500)}

	body, contentType := jsonBody(`{"user_id":"42","request_id":"req-1","delta_amount":500,"reason":"top up","operator_id":"6666","actor":"admin:6666"}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/wallets/adjustments", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := env.client.adjustReq.GetOperatorId(); got != "9001" {
		t.Fatalf("operator must come from the token, got %q", got)
	}
}

// TestAdminAdjustBalanceRejectsInvalidInput 冻结请求校验：
// 非法请求一律 400，且不得触达 Commerce。
func TestAdminAdjustBalanceRejectsInvalidInput(t *testing.T) {
	longRequestID := strings.Repeat("a", 129)
	longReason := strings.Repeat("b", 513)
	bodies := []string{
		`{}`,
		`{"user_id":"abc","request_id":"req-1","delta_amount":100,"reason":"r"}`,
		`{"user_id":"","request_id":"req-1","delta_amount":100,"reason":"r"}`,
		`{"user_id":"42","request_id":"","delta_amount":100,"reason":"r"}`,
		`{"user_id":"42","request_id":"` + longRequestID + `","delta_amount":100,"reason":"r"}`,
		`{"user_id":"42","request_id":"req-1","delta_amount":100,"reason":""}`,
		`{"user_id":"42","request_id":"req-1","delta_amount":100,"reason":"` + longReason + `"}`,
		`{"user_id":"42","request_id":"req-1","reason":"r"}`,
		`{"user_id":"42","request_id":"req-1","delta_amount":0,"reason":"r"}`,
		`{"user_id":"42","request_id":"req-1","delta_amount":-9223372036854775808,"reason":"r"}`,
		`not-json`,
	}

	for _, payload := range bodies {
		env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
		body, contentType := jsonBody(payload)
		w := env.perform(t, testConfig(), http.MethodPost, "/admin/wallets/adjustments", body, contentType)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d: %s", payload, w.Code, w.Body.String())
		}
		if env.client.adjustCalls != 0 {
			t.Fatalf("invalid request %s must not reach the RPC, got %d calls", payload, env.client.adjustCalls)
		}
	}
}

// TestAdminAdjustBalanceMapsFeatureDisabledToForbidden 冻结本接口的错误映射：
// 钱包功能关闭返回 403（其余旧接口仍保持 503），且不泄漏上游细节。
func TestAdminAdjustBalanceMapsFeatureDisabledToForbidden(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.adjustErr = kerrors.NewBizStatusError(
		int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_FEATURE_DISABLED), "wallet is disabled by runtime settings")

	body, contentType := jsonBody(`{"user_id":"42","request_id":"req-1","delta_amount":100,"reason":"r"}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/wallets/adjustments", body, contentType)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	code, message := decodeError(t, w)
	if code != int32(financeCodeFeatureDisabled) {
		t.Fatalf("unexpected error code: %d", code)
	}
	if message != "wallet feature is disabled" {
		t.Fatalf("unexpected error message: %q", message)
	}
	if strings.Contains(w.Body.String(), "runtime settings") {
		t.Fatalf("upstream detail must not leak: %s", w.Body.String())
	}
}

// TestAdminAdjustBalanceMapsConflictToHTTP 保证幂等/状态冲突仍是 409。
func TestAdminAdjustBalanceMapsConflictToHTTP(t *testing.T) {
	env := newAdminTestEnv(t, staticCodeGuard{allowed: true})
	env.client.adjustErr = kerrors.NewBizStatusError(
		int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_IDEMPOTENCY_CONFLICT), "request_id already used with other fields")

	body, contentType := jsonBody(`{"user_id":"42","request_id":"req-1","delta_amount":100,"reason":"r"}`)
	w := env.perform(t, testConfig(), http.MethodPost, "/admin/wallets/adjustments", body, contentType)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}
