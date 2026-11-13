package wallet

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/sirupsen/logrus"
)

// 这组测试冻结设计 §10.1 / §10.5 的运行时快照与动作策略：
// 错误刷新必须保留 last-known-good、stale 与关闭语义一致、无快照一律 fail closed。

// testNow 是注入的固定时钟基准，测试不依赖真实时间。
var testNow = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

// fakeSettingsSource 是可控制返回值的运行时快照来源；并发安全。
type fakeSettingsSource struct {
	mu    sync.Mutex
	snap  *setting.WalletRuntimeSettings
	err   error
	calls int
}

func (f *fakeSettingsSource) GetWalletRuntimeSettings(_ context.Context, _ *base.Empty) (*setting.WalletRuntimeSettings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.snap == nil {
		// 真实实现不会返回 (nil, nil)；这里保留该输入用于覆盖“返回 nil”的非法快照路径。
		return nil, nil
	}
	clone := *f.snap
	return &clone, nil
}

func (f *fakeSettingsSource) set(snap *setting.WalletRuntimeSettings, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snap, f.err = snap, err
}

func (f *fakeSettingsSource) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakePresence 模拟钱包业务数据存在性检查。
type fakePresence struct {
	has bool
	err error
}

func (f fakePresence) HasBusinessData(context.Context) (bool, error) { return f.has, f.err }

// validSnapshot 返回一份合法快照；secret 字段故意带可检索的特征串，供脱敏断言使用。
func validSnapshot(revision int64) *setting.WalletRuntimeSettings {
	return &setting.WalletRuntimeSettings{
		Revision:                    revision,
		Enabled:                     true,
		RechargeMode:                rechargeModeTest,
		RedemptionEnabled:           true,
		RedemptionRateLimit:         5,
		RedemptionRateWindowSeconds: 60,
		WalletCurrency:              currencyCNY,
		ProviderCurrency:            currencyUSD,
		CnyPerUsdRate:               "7.200000",
		CnyPerUsdNumerator:          36,
		CnyPerUsdDenominator:        5,
		TestProfile: &setting.WalletProviderProfileRuntime{
			Environment:     "TEST",
			ProfileId:       "profile-test",
			ProfileRevision: 1,
			ApiKey:          "sk_test_super_secret_key",
			ProductId:       "prod_test",
			WebhookId:       "wh_test",
			WebhookSecrets: []*setting.WalletWebhookSecretRuntime{
				{EndpointId: "ep_test", Fingerprint: "fp_test", Secret: "whsec_super_secret"},
			},
		},
		UpdatedBy: "admin",
		UpdatedAt: testNow.Unix(),
	}
}

func quietLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return logger
}

// newTestService 构造注入了可控时钟的 Service；返回的 *time.Time 可被测试推进。
func newTestService(t *testing.T, source SettingsSource, presence DataPresence, logger *logrus.Logger) (*Service, *time.Time) {
	t.Helper()

	svc, err := NewService(Options{Source: source, Presence: presence, Logger: logger})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	now := testNow
	svc.now = func() time.Time { return now }
	return svc, &now
}

// newLoadedService 构造已成功加载 snapshot 的 Service；stale=true 时将时钟推进到阈值之外。
func newLoadedService(t *testing.T, snap *setting.WalletRuntimeSettings, stale bool) *Service {
	t.Helper()

	svc, now := newTestService(t, &fakeSettingsSource{snap: snap}, nil, quietLogger())
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if stale {
		*now = now.Add(walletpolicy.SettingStaleThreshold + time.Minute)
	}
	return svc
}

// ============================ 动作策略表 ============================

var (
	readActions = []Action{
		ActionQuery,
		ActionListTransactions,
		ActionGetRecharge,
		ActionListRecharges,
		ActionAdminGetWallet,
		ActionAdminListWallets,
		ActionAdminListTransactions,
		ActionAdminGetRecharge,
		ActionAdminListRecharges,
		ActionAdminListRedemptionCodes,
	}
	recoveryActions = []Action{
		ActionCancelRecharge,
		ActionProcessPaymentWebhook,
		ActionRefundBalance,
		ActionAdminRefreshRecharge,
		ActionAdminRefundRecharge,
		ActionAdminSetCodeStatusDisable,
	}
	newValueActions = []Action{
		ActionCreateRecharge,
		ActionDebitBalance,
		ActionRedeemCode,
		ActionAdminCreateRedemptionCode,
		ActionAdminSetCodeStatusActive,
		ActionAdminAdjustBalance,
	}
)

func unionActions(groups ...[]Action) []Action {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	merged := make([]Action, 0, total)
	for _, group := range groups {
		merged = append(merged, group...)
	}
	return merged
}

func allWalletActions() []Action {
	return unionActions(readActions, recoveryActions, newValueActions)
}

func actionSet(actions []Action) map[Action]struct{} {
	set := make(map[Action]struct{}, len(actions))
	for _, a := range actions {
		set[a] = struct{}{}
	}
	return set
}

// TestActionTableCoversEveryAction 防止新增 Action 常量却忘记登记策略类别。
func TestActionTableCoversEveryAction(t *testing.T) {
	for _, a := range allWalletActions() {
		if _, ok := actionCategories[a]; !ok {
			t.Fatalf("动作 %q 未登记策略类别", a)
		}
	}
	if len(actionCategories) != len(allWalletActions()) {
		t.Fatalf("策略表条目 = %d, want %d（每个动作恰好一条）", len(actionCategories), len(allWalletActions()))
	}
}

// TestDecidePolicyTable 是设计 §10.1 的表驱动测试：
// 覆盖 enabled/disabled × stale/不 stale × recharge_mode × redemption_enabled 的组合。
func TestDecidePolicyTable(t *testing.T) {
	cases := []struct {
		name string
		// withSnapshot=false 表示从未成功加载过快照。
		withSnapshot bool
		stale        bool
		mutate       func(*setting.WalletRuntimeSettings)
		allowed      []Action
		denyReason   string
	}{
		{
			name:         "enabled 允许全部动作",
			withSnapshot: true,
			allowed:      unionActions(readActions, recoveryActions, newValueActions),
		},
		{
			name:         "disabled 只拒绝新价值意图",
			withSnapshot: true,
			mutate:       func(s *setting.WalletRuntimeSettings) { s.Enabled = false },
			allowed:      unionActions(readActions, recoveryActions),
			denyReason:   reasonWalletDisabled,
		},
		{
			name:         "stale 与 disabled 采用同一策略",
			withSnapshot: true,
			stale:        true,
			allowed:      unionActions(readActions, recoveryActions),
			denyReason:   reasonSettingsStale,
		},
		{
			name:         "recharge_mode=DISABLED 只额外拒绝新建充值",
			withSnapshot: true,
			mutate:       func(s *setting.WalletRuntimeSettings) { s.RechargeMode = rechargeModeDisabled },
			allowed: unionActions(readActions, recoveryActions,
				[]Action{ActionDebitBalance, ActionRedeemCode, ActionAdminCreateRedemptionCode, ActionAdminSetCodeStatusActive, ActionAdminAdjustBalance}),
			denyReason: reasonRechargeDisabled,
		},
		{
			name:         "redemption_enabled=false 拒绝兑换与兑换码启用",
			withSnapshot: true,
			mutate:       func(s *setting.WalletRuntimeSettings) { s.RedemptionEnabled = false },
			allowed: unionActions(readActions, recoveryActions,
				[]Action{ActionCreateRecharge, ActionDebitBalance, ActionAdminAdjustBalance}),
			denyReason: reasonRedemptionDisabled,
		},
		{
			name:         "disabled 且 redemption 关闭时仍按 disabled 拒绝",
			withSnapshot: true,
			mutate: func(s *setting.WalletRuntimeSettings) {
				s.Enabled = false
				s.RedemptionEnabled = false
			},
			allowed:    unionActions(readActions, recoveryActions),
			denyReason: reasonWalletDisabled,
		},
		{
			name:         "stale 且 recharge_mode=DISABLED 时按 stale 拒绝",
			withSnapshot: true,
			stale:        true,
			mutate:       func(s *setting.WalletRuntimeSettings) { s.RechargeMode = rechargeModeDisabled },
			allowed:      unionActions(readActions, recoveryActions),
			denyReason:   reasonSettingsStale,
		},
		{
			name:         "无快照一律拒绝",
			withSnapshot: false,
			allowed:      nil,
			denyReason:   reasonSettingsUnavailable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var svc *Service
			if tc.withSnapshot {
				snap := validSnapshot(1)
				if tc.mutate != nil {
					tc.mutate(snap)
				}
				svc = newLoadedService(t, snap, tc.stale)
			} else {
				svc, _ = newTestService(t, &fakeSettingsSource{}, nil, quietLogger())
			}

			allowed := actionSet(tc.allowed)
			for _, action := range allWalletActions() {
				_, wantAllowed := allowed[action]
				got := svc.Decide(action)

				if got.Allowed != wantAllowed {
					t.Fatalf("Decide(%q).Allowed = %v, want %v (reason=%q)",
						action, got.Allowed, wantAllowed, got.Reason)
				}
				if wantAllowed {
					if got.Reason != "" {
						t.Fatalf("允许的动作 %q 不应带拒绝原因，实际 %q", action, got.Reason)
					}
					continue
				}
				if got.Reason != tc.denyReason {
					t.Fatalf("Decide(%q).Reason = %q, want %q", action, got.Reason, tc.denyReason)
				}
			}
		})
	}
}

// TestDecideUnknownActionFailsClosed 保证新增 RPC 忘记登记策略时不会默认放行。
func TestDecideUnknownActionFailsClosed(t *testing.T) {
	svc := newLoadedService(t, validSnapshot(1), false)

	got := svc.Decide(Action("not_registered"))
	if got.Allowed {
		t.Fatal("未登记的动作必须拒绝")
	}
	if got.Reason != reasonUnknownAction {
		t.Fatalf("Reason = %q, want %q", got.Reason, reasonUnknownAction)
	}
}

// TestDecideWithoutSnapshotUsesStableReason 冻结无快照时的固定安全文案。
func TestDecideWithoutSnapshotUsesStableReason(t *testing.T) {
	svc, _ := newTestService(t, &fakeSettingsSource{}, nil, quietLogger())

	got := svc.Decide(ActionQuery)
	if got.Allowed {
		t.Fatal("无快照时即使查询也必须拒绝，由上层 Ready 决定是否可用")
	}
	if got.Reason != "wallet settings unavailable" {
		t.Fatalf("Reason = %q, want %q", got.Reason, "wallet settings unavailable")
	}
}

// ============================ 刷新与快照 ============================

func TestRefreshReplacesSnapshot(t *testing.T) {
	src := &fakeSettingsSource{snap: validSnapshot(1)}
	svc, now := newTestService(t, src, nil, quietLogger())

	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	snap, ok := svc.Snapshot()
	if !ok || snap.GetRevision() != 1 {
		t.Fatalf("Snapshot() = (%v, %v), want revision 1", snap.GetRevision(), ok)
	}
	if svc.Stale() {
		t.Fatal("刚加载的快照不应 stale")
	}

	// 新一轮 revision 快照必须替换旧快照。
	*now = now.Add(2 * time.Minute)
	src.set(validSnapshot(2), nil)
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	snap, ok = svc.Snapshot()
	if !ok || snap.GetRevision() != 2 {
		t.Fatalf("替换后 revision = %d, want 2", snap.GetRevision())
	}
	// 成功刷新会重置 loadedAt。
	*now = now.Add(4 * time.Minute)
	if svc.Stale() {
		t.Fatal("刷新后 loadedAt 未更新")
	}
}

func TestRefreshFailureKeepsLastKnownGood(t *testing.T) {
	src := &fakeSettingsSource{snap: validSnapshot(3)}
	svc, _ := newTestService(t, src, nil, quietLogger())
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	src.set(nil, errors.New("setting service unavailable"))
	if err := svc.Refresh(context.Background()); err == nil {
		t.Fatal("RPC 失败时 Refresh 必须返回错误")
	}

	snap, ok := svc.Snapshot()
	if !ok || snap.GetRevision() != 3 {
		t.Fatalf("RPC 失败后快照被破坏: ok=%v revision=%d", ok, snap.GetRevision())
	}
}

func TestRefreshRejectsInvalidSnapshotAndKeepsLastKnownGood(t *testing.T) {
	cases := []struct {
		name   string
		snap   *setting.WalletRuntimeSettings
		mutate func(*setting.WalletRuntimeSettings)
	}{
		{name: "返回 nil", snap: nil},
		{name: "revision 为 0", snap: validSnapshot(0)},
		{name: "revision 为负", snap: validSnapshot(-1)},
		{name: "wallet_currency 非 CNY", snap: validSnapshot(9), mutate: func(s *setting.WalletRuntimeSettings) { s.WalletCurrency = "EUR" }},
		{name: "provider_currency 非 USD", snap: validSnapshot(9), mutate: func(s *setting.WalletRuntimeSettings) { s.ProviderCurrency = currencyCNY }},
		{name: "recharge_mode 非法", snap: validSnapshot(9), mutate: func(s *setting.WalletRuntimeSettings) { s.RechargeMode = "PAUSED" }},
		{name: "recharge_mode 为空", snap: validSnapshot(9), mutate: func(s *setting.WalletRuntimeSettings) { s.RechargeMode = "" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := &fakeSettingsSource{snap: validSnapshot(5)}
			svc, _ := newTestService(t, src, nil, quietLogger())
			if err := svc.Refresh(context.Background()); err != nil {
				t.Fatalf("初始 Refresh() error = %v", err)
			}

			if tc.mutate != nil {
				tc.mutate(tc.snap)
			}
			src.set(tc.snap, nil)

			err := svc.Refresh(context.Background())
			if err == nil {
				t.Fatal("非法快照必须返回错误")
			}
			if tc.snap != nil && !errors.Is(err, errSettingsInvalid) {
				t.Fatalf("error = %v, want errors.Is(errSettingsInvalid)", err)
			}

			snap, ok := svc.Snapshot()
			if !ok || snap.GetRevision() != 5 {
				t.Fatalf("非法刷新破坏了 last-known-good: ok=%v revision=%d", ok, snap.GetRevision())
			}
			if svc.Stale() {
				t.Fatal("非法刷新不应重置或破坏 loadedAt")
			}
		})
	}
}

func TestRefreshFirstFailureLeavesNoSnapshot(t *testing.T) {
	src := &fakeSettingsSource{err: errors.New("setting service unavailable")}
	svc, _ := newTestService(t, src, nil, quietLogger())

	if err := svc.Refresh(context.Background()); err == nil {
		t.Fatal("首次刷新失败必须返回错误")
	}

	if _, ok := svc.Snapshot(); ok {
		t.Fatal("首次刷新失败后不得存在快照")
	}
	if !svc.Stale() {
		t.Fatal("从未加载过必须判为 stale")
	}
}

func TestStaleThreshold(t *testing.T) {
	src := &fakeSettingsSource{snap: validSnapshot(1)}
	svc, now := newTestService(t, src, nil, quietLogger())
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	// 恰好等于阈值不算 stale（判定为“超过”阈值）。
	*now = now.Add(walletpolicy.SettingStaleThreshold)
	if svc.Stale() {
		t.Fatal("恰好等于阈值时不应判为 stale")
	}
	*now = now.Add(time.Second)
	if !svc.Stale() {
		t.Fatal("超过阈值必须判为 stale")
	}
}

// ============================ Readiness ============================

func TestReady(t *testing.T) {
	cases := []struct {
		name     string
		snap     *setting.WalletRuntimeSettings
		stale    bool
		presence DataPresence
		wantErr  bool
	}{
		{
			name:     "有非 stale 快照时可用",
			snap:     validSnapshot(1),
			presence: nil, // 有快照时不需要再查业务数据
		},
		{
			name:     "stale 的 last-known-good 仍可用",
			snap:     validSnapshot(1),
			stale:    true,
			presence: fakePresence{has: true},
		},
		{
			name:     "无快照且无数据时允许首次初始化",
			presence: fakePresence{has: false},
		},
		{
			name:     "无快照但有数据时 readiness 失败",
			presence: fakePresence{has: true},
			wantErr:  true,
		},
		{
			name:     "无快照且检查失败时 readiness 失败",
			presence: fakePresence{err: errors.New("db down")},
			wantErr:  true,
		},
		{
			name:    "无快照且没有 Presence 时 readiness 失败",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var svc *Service
			var now *time.Time
			if tc.snap != nil {
				svc, now = newTestService(t, &fakeSettingsSource{snap: tc.snap}, tc.presence, quietLogger())
				if err := svc.Refresh(context.Background()); err != nil {
					t.Fatalf("Refresh() error = %v", err)
				}
				if tc.stale {
					*now = now.Add(walletpolicy.SettingStaleThreshold + time.Minute)
				}
			} else {
				svc, _ = newTestService(t, &fakeSettingsSource{}, tc.presence, quietLogger())
			}

			err := svc.Ready(context.Background())
			if tc.wantErr && err == nil {
				t.Fatal("Ready() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Ready() error = %v", err)
			}
		})
	}
}

func TestReadyErrorReasons(t *testing.T) {
	noDataSvc, _ := newTestService(t, &fakeSettingsSource{}, fakePresence{has: true}, quietLogger())
	if err := noDataSvc.Ready(context.Background()); !errors.Is(err, errDataPresentWithoutSettings) {
		t.Fatalf("error = %v, want errDataPresentWithoutSettings", err)
	}

	checkFailedSvc, _ := newTestService(t, &fakeSettingsSource{}, fakePresence{err: errors.New("db down")}, quietLogger())
	if err := checkFailedSvc.Ready(context.Background()); !errors.Is(err, errDataPresenceCheckFailed) {
		t.Fatalf("error = %v, want errDataPresenceCheckFailed", err)
	}
}

// ============================ 后台刷新 ============================

func TestStartRefreshesUntilContextCanceled(t *testing.T) {
	src := &fakeSettingsSource{snap: validSnapshot(1)}
	svc, err := NewService(Options{Source: src, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.refreshInterval = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx)

	// Start 会立刻加载一次，不需要等待第一个 tick。
	waitFor(t, "首次加载", func() bool {
		snap, ok := svc.Snapshot()
		return ok && snap.GetRevision() == 1
	})

	// 周期刷新必须跟随 revision 变化。
	src.set(validSnapshot(2), nil)
	waitFor(t, "周期刷新到 revision 2", func() bool {
		snap, ok := svc.Snapshot()
		return ok && snap.GetRevision() == 2
	})

	cancel()
	select {
	case <-svc.done:
	case <-time.After(3 * time.Second):
		t.Fatal("ctx 取消后后台刷新未停止")
	}

	// 循环退出后不应再产生 RPC 调用。
	before := src.callCount()
	time.Sleep(4 * svc.refreshInterval)
	if after := src.callCount(); after != before {
		t.Fatalf("取消后仍在刷新: calls %d -> %d", before, after)
	}
}

func TestStartSurvivesRefreshFailure(t *testing.T) {
	src := &fakeSettingsSource{err: errors.New("setting service unavailable")}
	svc, err := NewService(Options{Source: src, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.refreshInterval = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Start(ctx)

	waitFor(t, "持续失败仍在刷新", func() bool { return src.callCount() >= 2 })

	if _, ok := svc.Snapshot(); ok {
		t.Fatal("持续失败时不应产生快照")
	}
}

// ============================ 并发安全 ============================

func TestConcurrentRefreshAndSnapshot(t *testing.T) {
	src := &fakeSettingsSource{snap: validSnapshot(1)}
	svc, err := NewService(Options{Source: src, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if j%5 == 0 {
					src.set(validSnapshot(int64(worker*100+j+1)), nil)
				}
				_ = svc.Refresh(context.Background())
				if snap, ok := svc.Snapshot(); ok {
					_ = snap.GetRevision()
				}
				_ = svc.Stale()
				_ = svc.Decide(ActionCreateRecharge)
			}
		}(i)
	}
	wg.Wait()

	if _, ok := svc.Snapshot(); !ok {
		t.Fatal("并发刷新后应存在快照")
	}
}

// ============================ 日志脱敏 ============================

func TestRefreshLogsRevisionWithoutSecrets(t *testing.T) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

	svc, err := NewService(Options{Source: &fakeSettingsSource{snap: validSnapshot(7)}, Logger: logger})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "revision=7") {
		t.Fatalf("日志缺少 revision: %q", out)
	}
	for _, leaked := range []string{"sk_test_super_secret_key", "whsec_super_secret", "profile-test"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("日志泄露了敏感值 %q: %q", leaked, out)
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("等待超时: %s", what)
}
