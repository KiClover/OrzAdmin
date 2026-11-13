package wallet

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/KiClover/orzAdmin/internal/walletpolicy"
	"github.com/KiClover/orzAdmin/kitex_gen/base"
	"github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/sirupsen/logrus"
)

// 本文件实现 Commerce Wallet 的运行时 Setting 快照（设计 §10.5）与统一动作策略表（设计 §10.1）。
//
// 关键约束：
//   - 快照只在 Refresh 成功时原子替换；RPC 错误、返回 nil 或配置非法一律保留 last-known-good；
//   - 请求路径只读取一次快照引用，且把快照视为不可变对象；
//   - 动作策略只有本文件这一份表，Handler 不得各自解释；
//   - 日志只记录 revision 与错误原因，绝不输出 API Key / Webhook Secret / 完整 Profile。

const (
	currencyCNY = "CNY"
	currencyUSD = "USD"

	rechargeModeDisabled   = "DISABLED"
	rechargeModeTest       = "TEST"
	rechargeModeProduction = "PRODUCTION"
)

// 拒绝原因是面向用户的稳定安全文案，不得拼接任何配置内容或秘密。
const (
	reasonSettingsUnavailable = "wallet settings unavailable"
	reasonWalletDisabled      = "wallet disabled"
	reasonSettingsStale       = "wallet settings stale"
	reasonRechargeDisabled    = "recharge disabled"
	reasonRedemptionDisabled  = "redemption disabled"
	reasonUnknownAction       = "unknown wallet action"
)

var (
	// errSettingsUnavailable 表示从未成功加载过快照，也没有可以安全使用的内置默认值。
	errSettingsUnavailable = errors.New("wallet runtime settings unavailable")
	// errSettingsInvalid 表示 Setting 返回的快照不合法，调用方必须继续使用 last-known-good。
	errSettingsInvalid = errors.New("wallet runtime settings invalid")
	// errDataPresenceCheckFailed 表示无法确认是否已有钱包业务数据，readiness 必须失败。
	errDataPresenceCheckFailed = errors.New("wallet data presence check failed")
	// errDataPresentWithoutSettings 表示已有钱包业务数据却没有可信快照，无法承担回调验签与恢复。
	errDataPresentWithoutSettings = errors.New("wallet data exists but runtime settings unavailable")
)

// SettingsSource 是 Commerce Wallet 读取完整运行时快照的窄接口。
// 生产实现由 system-setting 的内部 RPC 客户端适配，测试使用 fake，不连真实服务。
type SettingsSource interface {
	GetWalletRuntimeSettings(ctx context.Context, req *base.Empty) (*setting.WalletRuntimeSettings, error)
}

// DataPresence 判断数据库里是否已存在钱包业务数据（账户/充值/流水/兑换码）。
// 生产实现由 db.WalletServiceDB 提供；查询失败必须返回错误，不能被当成「没有数据」。
type DataPresence interface {
	HasBusinessData(ctx context.Context) (bool, error)
}

// Action 是钱包 RPC 在策略表中的动作名。
type Action string

const (
	ActionQuery                     Action = "query"
	ActionListTransactions          Action = "list_transactions"
	ActionCreateRecharge            Action = "create_recharge"
	ActionGetRecharge               Action = "get_recharge"
	ActionListRecharges             Action = "list_recharges"
	ActionCancelRecharge            Action = "cancel_recharge"
	ActionRedeemCode                Action = "redeem_code"
	ActionDebitBalance              Action = "debit_balance"
	ActionRefundBalance             Action = "refund_balance"
	ActionProcessPaymentWebhook     Action = "process_payment_webhook"
	ActionAdminGetWallet            Action = "admin_get_wallet"
	ActionAdminListWallets          Action = "admin_list_wallets"
	ActionAdminListTransactions     Action = "admin_list_transactions"
	ActionAdminGetRecharge          Action = "admin_get_recharge"
	ActionAdminListRecharges        Action = "admin_list_recharges"
	ActionAdminRefreshRecharge      Action = "admin_refresh_recharge"
	ActionAdminRefundRecharge       Action = "admin_refund_recharge"
	ActionAdminCreateRedemptionCode Action = "admin_create_redemption_code"
	ActionAdminListRedemptionCodes  Action = "admin_list_redemption_codes"
	ActionAdminSetCodeStatusActive  Action = "admin_set_code_status_active"
	ActionAdminSetCodeStatusDisable Action = "admin_set_code_status_disable"
	ActionAdminAdjustBalance        Action = "admin_adjust_balance"
	// 重建支付链接会再次向渠道申请一个 Checkout（产生新的 Provider 对象），
	// 因此与创建充值同属"新资金意图"，关闭或 stale 时必须 fail closed。
	ActionRetryRechargeCheckout      Action = "retry_recharge_checkout"
	ActionAdminRetryRechargeCheckout Action = "admin_retry_recharge_checkout"
)

// Decision 是策略判断结果。Allowed=false 时 Reason 是可直接返回给用户的稳定文案。
type Decision struct {
	Allowed bool
	Reason  string
}

// actionCategory 是设计 §10.1 动作策略表的三个动作类别。
type actionCategory uint8

const (
	// categoryRead 只读动作：不产生新价值，因此任何快照状态都允许。
	categoryRead actionCategory = iota + 1
	// categoryRecovery 收尾/恢复动作：只收缩风险或收敛已发生的 Provider 事实，始终允许。
	categoryRecovery
	// categoryNewValue 新资金/新价值意图：快照关闭或 stale 时一律 fail closed。
	categoryNewValue
)

// actionCategories 是设计 §10.1 策略表的唯一权威映射。
var actionCategories = map[Action]actionCategory{
	// 查询：Get/List Wallet、Transaction、Recharge、RedemptionCode。
	ActionQuery:                    categoryRead,
	ActionListTransactions:         categoryRead,
	ActionGetRecharge:              categoryRead,
	ActionListRecharges:            categoryRead,
	ActionAdminGetWallet:           categoryRead,
	ActionAdminListWallets:         categoryRead,
	ActionAdminListTransactions:    categoryRead,
	ActionAdminGetRecharge:         categoryRead,
	ActionAdminListRecharges:       categoryRead,
	ActionAdminListRedemptionCodes: categoryRead,

	// 收尾/恢复：取消、签名回调、同 owner 补偿、管理员刷新/全额退款、禁用兑换码。
	ActionCancelRecharge:            categoryRecovery,
	ActionProcessPaymentWebhook:     categoryRecovery,
	ActionRefundBalance:             categoryRecovery,
	ActionAdminRefreshRecharge:      categoryRecovery,
	ActionAdminRefundRecharge:       categoryRecovery,
	ActionAdminSetCodeStatusDisable: categoryRecovery,

	// 新资金/新价值意图：关闭或 stale 时拒绝。
	ActionCreateRecharge:            categoryNewValue,
	ActionDebitBalance:              categoryNewValue,
	ActionRedeemCode:                categoryNewValue,
	ActionAdminCreateRedemptionCode: categoryNewValue,
	ActionAdminSetCodeStatusActive:  categoryNewValue,
	// 管理员人工调账会直接改动余额，属于新资金意图：快照关闭或 stale 时同样 fail closed。
	ActionAdminAdjustBalance: categoryNewValue,
	// 重建支付链接会新建 Checkout，同样按新价值意图处理。
	ActionRetryRechargeCheckout:      categoryNewValue,
	ActionAdminRetryRechargeCheckout: categoryNewValue,
}

// snapshot 是不可变快照加上它的成功加载时间。
type snapshot struct {
	settings *setting.WalletRuntimeSettings
	loadedAt time.Time
}

// Service 持有 Setting 快照缓存，并对外提供唯一的动作策略判断。
//
// 快照读取走 atomic.Pointer，刷新失败不会影响进行中的读；字段均不导出，
// 外部只能通过 NewService 构造。
type Service struct {
	source   SettingsSource
	presence DataPresence
	logger   *logrus.Logger

	state atomic.Pointer[snapshot]

	// now 与 refreshInterval 只为测试注入保留，生产取真实时钟与 walletpolicy 常量。
	now             func() time.Time
	refreshInterval time.Duration

	startOnce sync.Once
	done      chan struct{}

	// loggedRevision 用于避免每 10 秒重复输出同一 revision 的加载日志。
	loggedRevision atomic.Int64
}

// Options 是 NewService 的依赖集合。
type Options struct {
	Source   SettingsSource
	Presence DataPresence   // 可为 nil：任务 3 之前没有真实表。
	Logger   *logrus.Logger // 可为 nil，回落到 logrus 标准 Logger。
}

// NewService 构造快照缓存服务。Source 必填，其余依赖可缺省。
func NewService(opts Options) (*Service, error) {
	if opts.Source == nil {
		return nil, errors.New("wallet: SettingsSource is required")
	}

	logger := opts.Logger
	if logger == nil {
		logger = logrus.StandardLogger()
	}

	return &Service{
		source:          opts.Source,
		presence:        opts.Presence,
		logger:          logger,
		now:             time.Now,
		refreshInterval: walletpolicy.SettingRefreshInterval,
		done:            make(chan struct{}),
	}, nil
}

// Start 启动后台刷新：先立刻加载一次，之后每个 walletpolicy.SettingRefreshInterval 拉一次，
// ctx 取消即停止。本方法非阻塞，重复调用只有第一次生效。
func (s *Service) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		go s.loop(ctx)
	})
}

// loop 是后台刷新循环：成功/失败都不退出，失败时保留 last-known-good 并记录低基数告警。
func (s *Service) loop(ctx context.Context) {
	defer close(s.done)

	s.refreshOnce(ctx)

	ticker := time.NewTicker(s.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshOnce(ctx)
		}
	}
}

func (s *Service) refreshOnce(ctx context.Context) {
	if err := s.Refresh(ctx); err != nil {
		// 只记录错误原因，快照内容与任何秘密都不进日志。
		s.logger.WithError(err).Warn("wallet runtime settings refresh failed")
	}
}

// Refresh 同步刷新一次；成功时原子替换快照，失败时保留 last-known-good 并返回错误。
func (s *Service) Refresh(ctx context.Context) error {
	candidate, err := s.source.GetWalletRuntimeSettings(ctx, &base.Empty{})
	if err != nil {
		return fmt.Errorf("fetch wallet runtime settings: %w", err)
	}
	if err := validateRuntimeSettings(candidate); err != nil {
		return err
	}

	// 校验通过后才替换；快照本身视为不可变，后续请求只读不改。
	s.state.Store(&snapshot{settings: candidate, loadedAt: s.now()})
	s.logLoaded(candidate.GetRevision())
	return nil
}

func (s *Service) logLoaded(revision int64) {
	if previous := s.loggedRevision.Swap(revision); previous == revision {
		return
	}
	s.logger.WithField("revision", revision).Info("wallet runtime settings loaded")
}

// Snapshot 返回当前快照；ok=false 表示尚未成功加载过。
func (s *Service) Snapshot() (*setting.WalletRuntimeSettings, bool) {
	st := s.state.Load()
	if st == nil || st.settings == nil {
		return nil, false
	}
	return st.settings, true
}

// Stale 判断当前快照是否过期（超过 walletpolicy.SettingStaleThreshold 未成功刷新）。
// 从未成功加载过也算 stale。
func (s *Service) Stale() bool {
	st := s.state.Load()
	if st == nil || st.settings == nil {
		return true
	}
	return s.isStaleAt(st)
}

func (s *Service) isStaleAt(st *snapshot) bool {
	return s.now().Sub(st.loadedAt) > walletpolicy.SettingStaleThreshold
}

// Decide 按设计 §10.1 的统一动作策略表判断某个动作是否允许。
//
// 判定顺序：无快照一律拒绝 -> 关闭/陈旧拒绝新价值意图 -> 回收站级开关（recharge_mode、
// redemption_enabled）再收窄 -> 其余允许。
func (s *Service) Decide(a Action) Decision {
	st := s.state.Load()
	if st == nil || st.settings == nil {
		return Decision{Allowed: false, Reason: reasonSettingsUnavailable}
	}

	category, known := actionCategories[a]
	if !known {
		// 未知动作 fail closed，避免新增 RPC 忘记登记策略。
		return Decision{Allowed: false, Reason: reasonUnknownAction}
	}

	snap := st.settings
	if category == categoryNewValue {
		if !snap.GetEnabled() {
			return Decision{Allowed: false, Reason: reasonWalletDisabled}
		}
		if s.isStaleAt(st) {
			return Decision{Allowed: false, Reason: reasonSettingsStale}
		}
		// recharge_mode=DISABLED 只额外拒绝新建充值；旧订单回调/取消/查询/退款继续工作。
		// 重建支付链接同样会新建 Provider Checkout，因此一并拒绝。
		if (a == ActionCreateRecharge || a == ActionRetryRechargeCheckout || a == ActionAdminRetryRechargeCheckout) &&
			snap.GetRechargeMode() == rechargeModeDisabled {
			return Decision{Allowed: false, Reason: reasonRechargeDisabled}
		}
		// redemption_enabled=false 拒绝用户兑换与新建/重新启用兑换码，管理员仍可禁用现有码。
		if requiresRedemptionEnabled(a) && !snap.GetRedemptionEnabled() {
			return Decision{Allowed: false, Reason: reasonRedemptionDisabled}
		}
	}

	return Decision{Allowed: true}
}

// requiresRedemptionEnabled 标记受 redemption_enabled 开关约束的动作。
func requiresRedemptionEnabled(a Action) bool {
	switch a {
	case ActionRedeemCode, ActionAdminCreateRedemptionCode, ActionAdminSetCodeStatusActive:
		return true
	default:
		return false
	}
}

// Ready 判断服务是否可用：有可信快照（含 stale 的 last-known-good）即可用；
// 无快照时仅当 Presence 明确返回“无业务数据”才可用内置关闭默认值。
func (s *Service) Ready(ctx context.Context) error {
	if st := s.state.Load(); st != nil && st.settings != nil {
		// stale 只让策略拒绝新价值意图，业务仍可按 last-known-good 处理已发生的事实。
		return nil
	}

	if s.presence == nil {
		return fmt.Errorf("%w: no last-known-good snapshot and no data presence checker", errSettingsUnavailable)
	}

	hasData, err := s.presence.HasBusinessData(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", errDataPresenceCheckFailed, err)
	}
	if hasData {
		return errDataPresentWithoutSettings
	}

	// 全新数据库且没有任何钱包业务记录：允许使用内置 enabled=false 默认值完成首次初始化。
	return nil
}

// validateRuntimeSettings 校验 Setting 返回的快照是否可用于业务。
// 币种与充值模式是设计冻结值，任何偏差都视为快照损坏而不是“另一种合法配置”。
func validateRuntimeSettings(snap *setting.WalletRuntimeSettings) error {
	if snap == nil {
		return fmt.Errorf("%w: snapshot is nil", errSettingsInvalid)
	}
	if snap.GetRevision() <= 0 {
		return fmt.Errorf("%w: revision %d is not positive", errSettingsInvalid, snap.GetRevision())
	}
	if snap.GetWalletCurrency() != currencyCNY {
		return fmt.Errorf("%w: wallet_currency must be %s", errSettingsInvalid, currencyCNY)
	}
	if snap.GetProviderCurrency() != currencyUSD {
		return fmt.Errorf("%w: provider_currency must be %s", errSettingsInvalid, currencyUSD)
	}
	switch snap.GetRechargeMode() {
	case rechargeModeDisabled, rechargeModeTest, rechargeModeProduction:
	default:
		return fmt.Errorf("%w: recharge_mode %q is not supported", errSettingsInvalid, snap.GetRechargeMode())
	}
	return nil
}
