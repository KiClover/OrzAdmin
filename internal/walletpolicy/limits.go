// Package walletpolicy 集中定义钱包服务的内部技术限制常量。
//
// 这些数值属于实现细节而不是部署参数：超时、报文大小、刷新周期、stale 阈值和
// 列表分页上限都不进入 YAML、环境变量或在线设置，避免同一个安全边界被不同环境
// 配置成互相矛盾的值。确实随部署变化的地址、允许 Origin 和可信代理范围放在
// conf 包中，在线业务配置（汇率、开关、渠道 Profile）仍由 Setting 管理。
package walletpolicy

import "time"

const (
	// ProviderConnectTimeout 是连接支付渠道（Creem）时的连接建立超时。
	ProviderConnectTimeout = 5 * time.Second
	// ProviderRequestTimeout 是单次支付渠道请求的总超时，包含连接与读响应。
	ProviderRequestTimeout = 10 * time.Second
	// ProviderMaxResponseBytes 是支付渠道响应的最大读取字节数，超出即拒绝。
	ProviderMaxResponseBytes int64 = 1 << 20 // 1 MiB
	// WebhookMaxBodyBytes 是支付回调请求体的最大字节数，超出即拒绝。
	WebhookMaxBodyBytes int64 = 64 << 10 // 64 KiB
	// SettingRefreshInterval 是 Commerce 刷新 Setting 运行时快照的周期。
	SettingRefreshInterval = 10 * time.Second
	// SettingStaleThreshold 是运行时快照被判定为陈旧（stale）的阈值。
	SettingStaleThreshold = 5 * time.Minute

	// DefaultListPageSize 是列表请求省略分页大小时的默认值。
	DefaultListPageSize = 20
	// MaxListPageSize 是列表请求允许的最大分页大小，超过即返回参数错误。
	MaxListPageSize = 100

	// CallbackTimeout 是用户 HTTP 回调转发到 Commerce 的总超时。
	//
	// 必须明显小于 Creem 的 15 秒回调响应窗口，过期时让渠道重试。
	CallbackTimeout = 12 * time.Second

	// WalletRPCTimeout 是 Finance 调用 Commerce Wallet 的单次 RPC 预算。
	//
	// 必须大于 ProviderRequestTimeout：钱包建单时最多等渠道 10 秒，RPC 先超时
	// 只会让上层拿到 503，而钱包仍可能把订单建成，用户重试就变成第二笔订单。
	WalletRPCTimeout = ProviderRequestTimeout + 5*time.Second
)
