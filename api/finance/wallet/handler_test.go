package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	walletapi "github.com/KiClover/orzAdmin/api/model/finance/wallet"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/internal/middleware"
	walletpb "github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet/commercewalletservice"
	"github.com/KiClover/orzAdmin/pkg/jwt"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/kitex/client/callopt"
	"github.com/cloudwego/kitex/pkg/kerrors"
	"github.com/redis/go-redis/v9"
)

// fakeWalletClient 只覆盖本文件用到的 RPC，其余方法由嵌入的接口兜底。
type fakeWalletClient struct {
	commercewalletservice.Client

	getWalletReq  *walletpb.GetWalletReq
	getWalletResp *walletpb.GetWalletResp
	getWalletErr  error

	listTransactionsReq  *walletpb.ListTransactionsReq
	listTransactionsResp *walletpb.ListTransactionsResp
	listTransactionsErr  error

	createRechargeReq  *walletpb.CreateRechargeReq
	createRechargeResp *walletpb.CreateRechargeResp
	createRechargeErr  error

	listRechargesReq  *walletpb.ListRechargesReq
	listRechargesResp *walletpb.ListRechargesResp
	listRechargesErr  error

	getRechargeReq  *walletpb.GetRechargeReq
	getRechargeResp *walletpb.GetRechargeResp
	getRechargeErr  error

	cancelRechargeReq  *walletpb.CancelRechargeReq
	cancelRechargeResp *walletpb.CancelRechargeResp
	cancelRechargeErr  error

	redeemCalls int
	redeemReq   *walletpb.RedeemCodeReq
	redeemResp  *walletpb.RedeemCodeResp
	redeemErr   error

	webhookCalls int
	webhookReq   *walletpb.ProcessPaymentWebhookReq
	webhookResp  *walletpb.ProcessPaymentWebhookResp
	webhookErr   error
}

func (f *fakeWalletClient) GetWallet(ctx context.Context, req *walletpb.GetWalletReq, opts ...callopt.Option) (*walletpb.GetWalletResp, error) {
	f.getWalletReq = req
	return orEmpty(f.getWalletResp, f.getWalletErr)
}

func (f *fakeWalletClient) ListTransactions(ctx context.Context, req *walletpb.ListTransactionsReq, opts ...callopt.Option) (*walletpb.ListTransactionsResp, error) {
	f.listTransactionsReq = req
	return orEmpty(f.listTransactionsResp, f.listTransactionsErr)
}

func (f *fakeWalletClient) CreateRecharge(ctx context.Context, req *walletpb.CreateRechargeReq, opts ...callopt.Option) (*walletpb.CreateRechargeResp, error) {
	f.createRechargeReq = req
	return orEmpty(f.createRechargeResp, f.createRechargeErr)
}

func (f *fakeWalletClient) ListRecharges(ctx context.Context, req *walletpb.ListRechargesReq, opts ...callopt.Option) (*walletpb.ListRechargesResp, error) {
	f.listRechargesReq = req
	return orEmpty(f.listRechargesResp, f.listRechargesErr)
}

func (f *fakeWalletClient) GetRecharge(ctx context.Context, req *walletpb.GetRechargeReq, opts ...callopt.Option) (*walletpb.GetRechargeResp, error) {
	f.getRechargeReq = req
	return orEmpty(f.getRechargeResp, f.getRechargeErr)
}

func (f *fakeWalletClient) CancelRecharge(ctx context.Context, req *walletpb.CancelRechargeReq, opts ...callopt.Option) (*walletpb.CancelRechargeResp, error) {
	f.cancelRechargeReq = req
	return orEmpty(f.cancelRechargeResp, f.cancelRechargeErr)
}

func (f *fakeWalletClient) RedeemCode(ctx context.Context, req *walletpb.RedeemCodeReq, opts ...callopt.Option) (*walletpb.RedeemCodeResp, error) {
	f.redeemCalls++
	f.redeemReq = req
	return orEmpty(f.redeemResp, f.redeemErr)
}

func (f *fakeWalletClient) ProcessPaymentWebhook(ctx context.Context, req *walletpb.ProcessPaymentWebhookReq, opts ...callopt.Option) (*walletpb.ProcessPaymentWebhookResp, error) {
	f.webhookCalls++
	f.webhookReq = req
	return orEmpty(f.webhookResp, f.webhookErr)
}

// orEmpty 在 fake 未配置响应时返回空响应，避免 handler 解引用 nil。
func orEmpty[T any](resp *T, err error) (*T, error) {
	if resp == nil && err == nil {
		return new(T), nil
	}
	return resp, err
}

// testConfig 返回带可用 JWT 密钥的测试配置。
func testConfig() *conf.Config {
	return &conf.Config{
		AuthConf:  conf.AuthConf{AccessSecret: "test-access-secret", AccessExpire: 3600},
		RateLimit: conf.RateLimitConf{Policies: map[string]conf.RateLimitPolicy{"wallet_redemption": {}}},
	}
}

// newTestServer 构造不监听端口的 Hertz 实例，路由与生产一致。
func newTestServer(t *testing.T, cfg *conf.Config, client commercewalletservice.Client, rateMw *middleware.RateMw) *server.Hertz {
	t.Helper()
	return newTestServerWithService(t, cfg, &HzFinanceWalletService{Cfg: cfg, WalletClient: client, RateMw: rateMw})
}

// newTestServerWithService 补全授权依赖后注册路由。
//
// 管理员用例需要自定义 audit 时直接传入已装配的 service。
func newTestServerWithService(t *testing.T, cfg *conf.Config, svc *HzFinanceWalletService) *server.Hertz {
	t.Helper()
	if svc.RateMw == nil {
		rateMw, err := middleware.NewRateMw(cfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		svc.RateMw = rateMw
	}
	if svc.Enforcer == nil {
		svc.Enforcer = newTestEnforcer(t)
	}
	if svc.Audit == nil {
		svc.Audit = newLogAuditSink()
	}
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	NewRegisterRouter(h, svc)
	return h
}

// newTestEnforcer 只加载模型文件，不连接任何适配器。
func newTestEnforcer(t *testing.T) *casbin.Enforcer {
	t.Helper()
	enforcer, err := casbin.NewEnforcer("../../../etc/casbin.conf")
	if err != nil {
		t.Fatalf("load casbin model: %v", err)
	}
	return enforcer
}

// Only the Redis transport is faked. Routing, authentication and the rate middleware run normally.
type fakeRateRedis struct {
	redis.UniversalClient
	counts map[string]int64
	err    error
}

func (r *fakeRateRedis) EvalSha(ctx context.Context, _ string, keys []string, _ ...interface{}) *redis.Cmd {
	cmd := redis.NewCmd(ctx)
	if r.err != nil {
		cmd.SetErr(r.err)
		return cmd
	}
	if r.counts == nil {
		r.counts = map[string]int64{}
	}
	r.counts[keys[0]]++
	cmd.SetVal(r.counts[keys[0]])
	return cmd
}

// bearerToken 签发一个携带 userId claim 的令牌。
func bearerToken(t *testing.T, cfg *conf.Config, userID uint64) string {
	t.Helper()
	token, err := jwt.NewJwtToken(cfg.AuthConf.AccessSecret, time.Now().Unix(), 3600, jwt.WithOption("userId", userID))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return "Bearer " + token
}

// jsonBody 构造 JSON 请求体。
func jsonBody(body string) (*ut.Body, ut.Header) {
	return &ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"}
}

// redeemBody 构造兑换码请求体。
func redeemBody() *ut.Body {
	body, _ := jsonBody(`{"code":"ABCDEFGHJKLMNPQRSTUV"}`)
	return body
}

// rechargeBody 构造创建充值请求体。
func rechargeBody() *ut.Body {
	body, _ := jsonBody(`{"request_id":"req-1","wallet_amount":900,"provider":"CREEM"}`)
	return body
}

// envelope 是统一响应外层。
type envelope struct {
	Code int32           `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func decodeEnvelope(t *testing.T, body string) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("decode response %q: %v", body, err)
	}
	return env
}

// walletView 构造一个用户余额视图。
func walletView(balance int64) *walletpb.WalletView {
	updatedAt := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC).UnixMilli()
	return &walletpb.WalletView{
		Currency:      "CNY",
		BalanceAmount: balance,
		Status:        walletpb.WalletStatus_WALLET_STATUS_ACTIVE,
		Revision:      3,
		UpdatedAt:     updatedAt,
	}
}

func TestGetWalletUsesJWTIdentity(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{getWalletResp: &walletpb.GetWalletResp{Wallet: walletView(12345)}}
	h := newTestServer(t, cfg, fake, nil)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/wallet", nil,
		ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 7001)})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if fake.getWalletReq == nil || fake.getWalletReq.GetUserId() != "7001" {
		t.Fatalf("expected RPC user id 7001, got %+v", fake.getWalletReq)
	}
	env := decodeEnvelope(t, w.Body.String())
	var data walletapi.GetWalletResp
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.GetWallet() == nil || data.GetWallet().GetBalanceAmount() != 12345 {
		t.Fatalf("unexpected wallet payload: %+v", data.GetWallet())
	}
	if data.GetWallet().GetStatus() != "ACTIVE" {
		t.Fatalf("expected ACTIVE status, got %q", data.GetWallet().GetStatus())
	}
}

func TestWalletRoutesRequireToken(t *testing.T) {
	cfg := testConfig()
	h := newTestServer(t, cfg, &fakeWalletClient{}, nil)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/wallet", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateRechargeIgnoresBodyIdentity(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{createRechargeResp: &walletpb.CreateRechargeResp{}}
	h := newTestServer(t, cfg, fake, nil)

	body, header := jsonBody(`{"request_id":"req-1","wallet_amount":900,"provider":"CREEM","user_id":"9999"}`)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/recharges", body, header,
		ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 7001)})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if fake.createRechargeReq == nil {
		t.Fatalf("expected CreateRecharge to be called")
	}
	if fake.createRechargeReq.GetUserId() != "7001" {
		t.Fatalf("expected JWT identity 7001, got %q", fake.createRechargeReq.GetUserId())
	}
	if fake.createRechargeReq.GetWalletAmount() != 900 {
		t.Fatalf("expected amount 900, got %d", fake.createRechargeReq.GetWalletAmount())
	}
	if fake.createRechargeReq.GetProvider() != walletpb.PaymentProvider_PAYMENT_PROVIDER_CREEM {
		t.Fatalf("unexpected provider %v", fake.createRechargeReq.GetProvider())
	}
}

func TestCreateRechargeRejectsInvalidProvider(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{}
	h := newTestServer(t, cfg, fake, nil)

	body, header := jsonBody(`{"request_id":"req-1","wallet_amount":900,"provider":"paypal"}`)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/recharges", body, header,
		ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 7001)})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if fake.createRechargeReq != nil {
		t.Fatalf("expected no RPC call for an invalid provider")
	}
}

func TestListTransactionsRejectsInvalidFilter(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{}
	h := newTestServer(t, cfg, fake, nil)

	body, contentType := jsonBody(`{"type":"bogus"}`)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/transactions/list", body,
		ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 7001)}, contentType)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if fake.listTransactionsReq != nil {
		t.Fatalf("expected no RPC call for an invalid filter")
	}
}

func TestGetRechargeRejectsInvalidPathID(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{}
	h := newTestServer(t, cfg, fake, nil)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/wallet/recharges/abc", nil,
		ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 7001)})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if fake.getRechargeReq != nil {
		t.Fatalf("expected no RPC call for an invalid path id")
	}
}

func TestWalletBizErrorIsMappedWithoutLeaking(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{
		createRechargeErr: kerrors.NewBizStatusError(
			int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_INSUFFICIENT_BALANCE),
			"raw upstream detail",
		),
	}
	h := newTestServer(t, cfg, fake, nil)

	body, header := jsonBody(`{"request_id":"req-1","wallet_amount":900,"provider":"CREEM"}`)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/recharges", body, header,
		ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 7001)})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "raw upstream detail") {
		t.Fatalf("response leaked upstream detail: %s", w.Body.String())
	}
}

func TestWalletUnknownErrorIsUnavailable(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{getWalletErr: errors.New("dial tcp 10.0.0.1: connect: connection refused")}
	h := newTestServer(t, cfg, fake, nil)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/wallet", nil,
		ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 7001)})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "10.0.0.1") {
		t.Fatalf("response leaked connection detail: %s", w.Body.String())
	}
}

func TestRedeemCodeRateLimitedBeforeRPC(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{redeemResp: &walletpb.RedeemCodeResp{Wallet: walletView(500), CreditedAmount: 500}}
	cfg.RateLimit.Policies["wallet_redemption"] = conf.RateLimitPolicy{User: conf.RateLimitRule{Enabled: true, WindowSeconds: 600, MaxRequests: 1}}
	rateMw, err := middleware.NewRateMw(cfg, &fakeRateRedis{})
	if err != nil {
		t.Fatal(err)
	}
	h := newTestServer(t, cfg, fake, rateMw)
	token := bearerToken(t, cfg, 7001)
	contentType := ut.Header{Key: "Content-Type", Value: "application/json"}

	first := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/redemption-codes/redeem",
		redeemBody(), contentType,
		ut.Header{Key: "Authorization", Value: token})
	if first.Code != http.StatusOK {
		t.Fatalf("expected first redemption 200, got %d: %s", first.Code, first.Body.String())
	}

	second := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/redemption-codes/redeem",
		redeemBody(), contentType,
		ut.Header{Key: "Authorization", Value: token})
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", second.Code, second.Body.String())
	}
	if fake.redeemCalls != 1 {
		t.Fatalf("expected the RPC to be called once, got %d", fake.redeemCalls)
	}
}

func TestRedeemCodeFailsClosedWhenLimiterUnavailable(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{}
	cfg.RateLimit.Policies["wallet_redemption"] = conf.RateLimitPolicy{User: conf.RateLimitRule{Enabled: true, WindowSeconds: 600, MaxRequests: 3}}
	rateMw, err := middleware.NewRateMw(cfg, &fakeRateRedis{err: errors.New("redis is down")})
	if err != nil {
		t.Fatal(err)
	}
	h := newTestServer(t, cfg, fake, rateMw)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/redemption-codes/redeem",
		redeemBody(), ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "Authorization", Value: bearerToken(t, cfg, 7001)})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	if fake.redeemCalls != 0 {
		t.Fatalf("expected no RPC call when the limiter is unavailable, got %d", fake.redeemCalls)
	}
}

func TestPaymentCallbackForwardsRawBodyAndHeaders(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{webhookResp: &walletpb.ProcessPaymentWebhookResp{Accepted: true, Message: "received"}}
	h := newTestServer(t, cfg, fake, nil)

	raw := `{"eventType":"transaction.paid","object":{"id":"txn_1"}}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/payment-callbacks/CREEM",
		&ut.Body{Body: bytes.NewReader([]byte(raw)), Len: len(raw)},
		ut.Header{Key: "creem-signature", Value: "first"},
		ut.Header{Key: "creem-signature", Value: "second"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if fake.webhookReq == nil {
		t.Fatalf("expected ProcessPaymentWebhook to be called")
	}
	if string(fake.webhookReq.GetBody()) != raw {
		t.Fatalf("expected raw body to be forwarded unchanged, got %q", string(fake.webhookReq.GetBody()))
	}
	if fake.webhookReq.GetProvider() != walletpb.PaymentProvider_PAYMENT_PROVIDER_CREEM {
		t.Fatalf("unexpected provider %v", fake.webhookReq.GetProvider())
	}

	var signatureValues []string
	for _, header := range fake.webhookReq.GetHeaders() {
		if strings.EqualFold(header.GetName(), "creem-signature") {
			signatureValues = header.GetValues()
		}
	}
	if len(signatureValues) != 2 {
		t.Fatalf("expected duplicate signature headers to be preserved, got %v", fake.webhookReq.GetHeaders())
	}

	env := decodeEnvelope(t, w.Body.String())
	var data walletapi.PaymentCallbackResp
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.GetReceived() != true {
		t.Fatalf("expected received=true, got %v", data.GetReceived())
	}
}

func TestPaymentCallbackRejectsUnknownProvider(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{}
	h := newTestServer(t, cfg, fake, nil)

	w := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/payment-callbacks/paypal",
		&ut.Body{Body: strings.NewReader("{}"), Len: 2})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if fake.webhookCalls != 0 {
		t.Fatalf("expected no RPC call for an unknown provider")
	}
}

func TestPaymentCallbackRejectsEmptyAndOversizeBody(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{}
	h := newTestServer(t, cfg, fake, nil)

	empty := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/payment-callbacks/CREEM", nil)
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty body, got %d: %s", empty.Code, empty.Body.String())
	}

	oversize := strings.Repeat("a", 64*1024+1)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/payment-callbacks/CREEM",
		&ut.Body{Body: strings.NewReader(oversize), Len: len(oversize)})
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
	if fake.webhookCalls != 0 {
		t.Fatalf("expected no RPC call for empty/oversize body, got %d", fake.webhookCalls)
	}
}

func TestPaymentCallbackCommerceFailureReturns503(t *testing.T) {
	cfg := testConfig()
	fake := &fakeWalletClient{webhookErr: kerrors.NewBizStatusError(
		int32(walletpb.WalletErrorCode_WALLET_ERROR_CODE_UNAVAILABLE), "temporary")}
	h := newTestServer(t, cfg, fake, nil)

	raw := `{"eventType":"transaction.paid"}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/wallet/payment-callbacks/CREEM",
		&ut.Body{Body: strings.NewReader(raw), Len: len(raw)})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 so the provider retries, got %d: %s", w.Code, w.Body.String())
	}
}
