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
