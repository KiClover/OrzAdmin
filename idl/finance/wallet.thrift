namespace go finance.wallet

// Finance Wallet HTTP 契约（对外 DTO）。
//
// 约束（见 doc/plan/WalletDesign.MD §11.2–§11.4、§11.6）：
//   1. 用户 DTO 不得包含 user_id / wallet_id / operator_id / request_id / Provider 内部 ID /
//      审核与退款审计字段 / Secret。
//   2. 管理员传入的操作人身份一律来自 JWT + Casbin，不作为 body 字段接收。
//   3. 完整明文兑换码只出现在管理员兑换码响应（AdminRedemptionCodeView）中。
//   4. 金额一律 int64 最小货币单位：钱包为 CNY 分，Provider 为 USD 美分。
//   5. 对外 ID 一律十进制字符串；时间一律 RFC3339 毫秒字符串。
//   6. 成功 2xx；参数/认证/授权/不存在/状态冲突稳定 4xx；Provider/DB 暂时失败可重试 5xx。

// ============================ 用户视图 ============================

// 钱包视图：GET /wallet
// 用户视图隔离（§11.6）：用户 DTO 不暴露 wallet_id，钱包由 JWT 身份唯一确定。
struct WalletView {
  1: optional string currency (api.body = "currency")
  2: optional i64 balance_amount (api.body = "balance_amount")
  3: optional string status (api.body = "status")
  4: optional i64 revision (api.body = "revision")
  5: optional string updated_at (api.body = "updated_at")
}

// 流水视图：GET /wallet/transactions，不含 wallet_id / owner_namespace / 业务键 / actor。
struct TransactionView {
  1: optional string id (api.body = "id")
  2: optional string type (api.body = "type")
  3: optional string currency (api.body = "currency")
  4: optional i64 delta_amount (api.body = "delta_amount")
  5: optional i64 balance_after (api.body = "balance_after")
  6: optional string description (api.body = "description")
  7: optional string created_at (api.body = "created_at")
}

// 用户充值视图：不含 wallet_id / user_id / request_id / Provider 内部 ID / 审核与退款审计。
struct RechargeView {
  1: optional string id (api.body = "id")
  2: optional string status (api.body = "status")
  3: optional i64 wallet_amount (api.body = "wallet_amount")
  4: optional string wallet_currency (api.body = "wallet_currency")
  5: optional i64 provider_amount (api.body = "provider_amount")
  6: optional string provider_currency (api.body = "provider_currency")
  7: optional string cny_per_usd_rate (api.body = "cny_per_usd_rate")
  8: optional string checkout_url (api.body = "checkout_url")
  9: optional string created_at (api.body = "created_at")
  10: optional string paid_at (api.body = "paid_at")
  11: optional string updated_at (api.body = "updated_at")
}

// ============================ 管理员视图 ============================

// 管理员钱包视图：POST /admin/wallets/list、POST /admin/wallets/get
struct AdminWalletView {
  1: optional string wallet_id (api.body = "wallet_id")
  2: optional string user_id (api.body = "user_id")
  3: optional string currency (api.body = "currency")
  4: optional i64 balance_amount (api.body = "balance_amount")
  5: optional string status (api.body = "status")
  6: optional i64 revision (api.body = "revision")
  7: optional string created_at (api.body = "created_at")
  8: optional string updated_at (api.body = "updated_at")
}

// 管理员流水视图：含归属与审计字段。
struct AdminTransactionView {
  1: optional string id (api.body = "id")
  2: optional string wallet_id (api.body = "wallet_id")
  3: optional string user_id (api.body = "user_id")
  4: optional string type (api.body = "type")
  5: optional string currency (api.body = "currency")
  6: optional i64 delta_amount (api.body = "delta_amount")
  7: optional i64 balance_after (api.body = "balance_after")
  8: optional string owner_namespace (api.body = "owner_namespace")
  9: optional string business_type (api.body = "business_type")
  10: optional string business_id (api.body = "business_id")
  11: optional string related_transaction_id (api.body = "related_transaction_id")
  12: optional string description (api.body = "description")
  13: optional string actor (api.body = "actor")
  14: optional string created_at (api.body = "created_at")
}

// 管理员充值视图：含归属、冻结报价快照与人工处置字段。
struct AdminRechargeView {
  1: optional string id (api.body = "id")
  2: optional string user_id (api.body = "user_id")
  3: optional string wallet_id (api.body = "wallet_id")
  4: optional string request_id (api.body = "request_id")
  5: optional string merchant_order_no (api.body = "merchant_order_no")
  6: optional string status (api.body = "status")
  7: optional string provider (api.body = "provider")
  8: optional string provider_environment (api.body = "provider_environment")
  9: optional string payment_profile_id (api.body = "payment_profile_id")
  10: optional i64 payment_profile_revision (api.body = "payment_profile_revision")
  11: optional string provider_product_id (api.body = "provider_product_id")
  12: optional string provider_checkout_id (api.body = "provider_checkout_id")
  13: optional string provider_order_id (api.body = "provider_order_id")
  14: optional string provider_transaction_id (api.body = "provider_transaction_id")
  15: optional i64 wallet_amount (api.body = "wallet_amount")
  16: optional string wallet_currency (api.body = "wallet_currency")
  17: optional i64 provider_amount (api.body = "provider_amount")
  18: optional string provider_currency (api.body = "provider_currency")
  19: optional i64 fx_rate_numerator (api.body = "fx_rate_numerator")
  20: optional i64 fx_rate_denominator (api.body = "fx_rate_denominator")
  21: optional i64 fx_setting_revision (api.body = "fx_setting_revision")
  22: optional string fx_quoted_at (api.body = "fx_quoted_at")
  23: optional string fx_rounding_mode (api.body = "fx_rounding_mode")
  24: optional string checkout_url (api.body = "checkout_url")
  25: optional string review_reason (api.body = "review_reason")
  26: optional bool review_blocks_debit (api.body = "review_blocks_debit")
  27: optional string reviewed_by (api.body = "reviewed_by")
  28: optional string reviewed_at (api.body = "reviewed_at")
  29: optional string refund_reason (api.body = "refund_reason")
  30: optional string refund_requested_by (api.body = "refund_requested_by")
  31: optional string refund_requested_at (api.body = "refund_requested_at")
  32: optional string paid_at (api.body = "paid_at")
  33: optional string canceled_at (api.body = "canceled_at")
  34: optional string refunded_at (api.body = "refunded_at")
  35: optional i64 revision (api.body = "revision")
  36: optional string created_at (api.body = "created_at")
  37: optional string updated_at (api.body = "updated_at")
}

// 管理员兑换码视图：唯一允许返回完整明文 code 的 DTO。
struct AdminRedemptionCodeView {
  1: optional string id (api.body = "id")
  2: optional string code (api.body = "code")
  3: optional string type (api.body = "type")
  4: optional i64 amount (api.body = "amount")
  5: optional string currency (api.body = "currency")
  6: optional string status (api.body = "status")
  7: optional string expires_at (api.body = "expires_at")
  8: optional i64 redeemed_count (api.body = "redeemed_count")
  9: optional string claimed_by_user_id (api.body = "claimed_by_user_id")
  10: optional string claimed_at (api.body = "claimed_at")
  11: optional string created_by (api.body = "created_by")
  12: optional string created_at (api.body = "created_at")
  13: optional string updated_at (api.body = "updated_at")
  14: optional i64 max_redemptions (api.body = "max_redemptions")
}

// ============================ 用户接口 DTO ============================

// GET /wallet
struct GetWalletResp {
  1: optional WalletView wallet (api.body = "wallet")
}

// POST /wallet/transactions/list
// 列表与筛选统一使用 POST + JSON body（仓库既有 API 同样以 POST 承载列表），
// 只有带路径参数的详情查询保留 GET。
struct ListTransactionsReq {
  1: optional string type (api.body = "type")
  2: optional i64 start_time (api.body = "start_time")
  3: optional i64 end_time (api.body = "end_time")
  4: optional i32 page_size (api.body = "page_size")
  5: optional string cursor (api.body = "cursor")
}

struct ListTransactionsResp {
  1: optional list<TransactionView> transactions (api.body = "transactions")
  2: optional string next_cursor (api.body = "next_cursor")
  3: optional bool has_more (api.body = "has_more")
}

// POST /wallet/recharges
// 用户只提供稳定 request_id 与 CNY 意图；USD 金额与汇率由服务端按 Setting 快照换算并冻结。
struct CreateRechargeReq {
  1: optional string request_id (api.body = "request_id")
  2: optional i64 wallet_amount (api.body = "wallet_amount")
  3: optional string provider (api.body = "provider")
}

struct CreateRechargeResp {
  1: optional RechargeView recharge (api.body = "recharge")
}

// GET /wallet/recharges/:rechargeId
struct GetRechargeReq {
  1: optional string recharge_id (api.path = "rechargeId")
}

struct GetRechargeResp {
  1: optional RechargeView recharge (api.body = "recharge")
}

// POST /wallet/recharges/list
struct ListRechargesReq {
  1: optional string status (api.body = "status")
  2: optional i32 page_size (api.body = "page_size")
  3: optional string cursor (api.body = "cursor")
}

struct ListRechargesResp {
  1: optional list<RechargeView> recharges (api.body = "recharges")
  2: optional string next_cursor (api.body = "next_cursor")
  3: optional bool has_more (api.body = "has_more")
}

// POST /wallet/recharges/:rechargeId/cancel
struct CancelRechargeReq {
  1: optional string recharge_id (api.path = "rechargeId")
}

struct CancelRechargeResp {
  1: optional RechargeView recharge (api.body = "recharge")
}

// POST /wallet/redemption-codes/redeem
struct RedeemCodeReq {
  1: optional string code (api.body = "code")
}

struct RedeemCodeResp {
  1: optional WalletView wallet (api.body = "wallet")
  2: optional i64 credited_amount (api.body = "credited_amount")
}

// POST /wallet/payment-callbacks/:provider
// 请求体（原始 JSON）由 Finance 原样转发给 Commerce，验签在 Commerce 内完成；
// 此处只声明响应体，不对此路由做 body 绑定。
struct PaymentCallbackResp {
  1: optional bool received (api.body = "received")
  2: optional string message (api.body = "message")
}

// ============================ 管理员接口 DTO ============================

// POST /admin/wallets/list
struct AdminListWalletsReq {
  1: optional string user_id (api.body = "user_id")
  2: optional string status (api.body = "status")
  3: optional i32 page_size (api.body = "page_size")
  4: optional string cursor (api.body = "cursor")
}

struct AdminListWalletsResp {
  1: optional list<AdminWalletView> wallets (api.body = "wallets")
  2: optional string next_cursor (api.body = "next_cursor")
  3: optional bool has_more (api.body = "has_more")
}

// POST /admin/wallets/get
// wallet_id 与 user_id 都是可选的定位条件，由 body 传入；至少提供一个。
struct AdminGetWalletReq {
  1: optional string wallet_id (api.body = "wallet_id")
  2: optional string user_id (api.body = "user_id")
}

struct AdminGetWalletResp {
  1: optional AdminWalletView wallet (api.body = "wallet")
}

// POST /admin/transactions/list
// wallet_id / user_id 为可选的归属过滤条件，由 body 传入。
struct AdminListTransactionsReq {
  1: optional string wallet_id (api.body = "wallet_id")
  2: optional string user_id (api.body = "user_id")
  3: optional string type (api.body = "type")
  4: optional i64 start_time (api.body = "start_time")
  5: optional i64 end_time (api.body = "end_time")
  6: optional i32 page_size (api.body = "page_size")
  7: optional string cursor (api.body = "cursor")
}

struct AdminListTransactionsResp {
  1: optional list<AdminTransactionView> transactions (api.body = "transactions")
  2: optional string next_cursor (api.body = "next_cursor")
  3: optional bool has_more (api.body = "has_more")
}

// GET /admin/recharges/:rechargeId
struct AdminGetRechargeReq {
  1: optional string recharge_id (api.path = "rechargeId")
}

struct AdminGetRechargeResp {
  1: optional AdminRechargeView recharge (api.body = "recharge")
}

// POST /admin/recharges/list
struct AdminListRechargesReq {
  1: optional string user_id (api.body = "user_id")
  2: optional string status (api.body = "status")
  3: optional string provider (api.body = "provider")
  4: optional i32 page_size (api.body = "page_size")
  5: optional string cursor (api.body = "cursor")
}

struct AdminListRechargesResp {
  1: optional list<AdminRechargeView> recharges (api.body = "recharges")
  2: optional string next_cursor (api.body = "next_cursor")
  3: optional bool has_more (api.body = "has_more")
}

// POST /admin/recharges/:rechargeId/refresh
// 只查询并收敛已有订单；不接受管理员指定成功状态、到账金额或新汇率。
struct AdminRefreshRechargeReq {
  1: optional string recharge_id (api.path = "rechargeId")
  2: optional string provider_checkout_id (api.body = "provider_checkout_id")
  3: optional string provider_order_id (api.body = "provider_order_id")
  4: optional string provider_transaction_id (api.body = "provider_transaction_id")
  5: optional string reason (api.body = "reason")
  6: optional bool confirm_provider_not_created (api.body = "confirm_provider_not_created")
}

struct AdminRefreshRechargeResp {
  1: optional AdminRechargeView recharge (api.body = "recharge")
}

// POST /admin/recharges/:rechargeId/refund
// 全额退款；金额从订单冻结快照派生，body 只含必填原因。
struct AdminRefundRechargeReq {
  1: optional string recharge_id (api.path = "rechargeId")
  2: optional string reason (api.body = "reason")
}

struct AdminRefundRechargeResp {
  1: optional AdminRechargeView recharge (api.body = "recharge")
}

// POST /admin/redemption-codes
struct AdminCreateRedemptionCodeReq {
  1: optional string request_id (api.body = "request_id")
  2: optional string type (api.body = "type")
  3: optional i64 amount (api.body = "amount")
  4: optional i64 expires_at (api.body = "expires_at")
  5: optional string code (api.body = "code")
  6: optional i64 max_redemptions (api.body = "max_redemptions")
}

struct AdminCreateRedemptionCodeResp {
  1: optional AdminRedemptionCodeView code (api.body = "code")
}

// POST /admin/redemption-codes/list
struct AdminListRedemptionCodesReq {
  1: optional string type (api.body = "type")
  2: optional string status (api.body = "status")
  3: optional i32 page_size (api.body = "page_size")
  4: optional string cursor (api.body = "cursor")
}

struct AdminListRedemptionCodesResp {
  1: optional list<AdminRedemptionCodeView> codes (api.body = "codes")
  2: optional string next_cursor (api.body = "next_cursor")
  3: optional bool has_more (api.body = "has_more")
}

// POST /admin/redemption-codes/:codeId/status
// 只允许 ACTIVE <-> DISABLED。
struct AdminSetRedemptionCodeStatusReq {
  1: optional string code_id (api.path = "codeId")
  2: optional string status (api.body = "status")
  3: optional string reason (api.body = "reason")
}

struct AdminSetRedemptionCodeStatusResp {
  1: optional AdminRedemptionCodeView code (api.body = "code")
}

// POST /admin/wallets/adjustments
// 管理员调账：delta_amount 为正加款、为负扣款，单位 CNY 分。
// 幂等键是 request_id；操作人只来自 JWT，本请求体不接受任何身份字段。
struct AdminAdjustBalanceReq {
  1: optional string user_id (api.body = "user_id")
  2: optional string request_id (api.body = "request_id")
  3: optional i64 delta_amount (api.body = "delta_amount")
  4: optional string reason (api.body = "reason")
}

struct AdminAdjustBalanceResp {
  1: optional AdminTransactionView transaction (api.body = "transaction")
}
