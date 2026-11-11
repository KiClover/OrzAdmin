// 统一初始化 API 的 HTTP 契约（任务 2 冻结）。
//
// 设计约束：
//   - 只暴露固定的服务白名单；未知服务返回 404，不接受 DSN、SQL、库名或表名入参；
//   - 初始化幂等：表结构用 Ent 自动同步，种子按业务键补齐，已存在数据保持不变；
//   - 仅限本机或受信内网调用，不注册 JWT / Casbin 鉴权，也不接受初始化密钥。
namespace go initialize

// ServiceResult 描述单个服务单个阶段的执行结果。
struct ServiceResult {
    1: optional string service (api.body="service") // 服务名，如 system-user
    2: optional string stage (api.body="stage")     // schema | seed
    3: optional string status (api.body="status")   // success | failed | skipped
    4: optional string message (api.body="message") // 安全文案，不含 DSN / SQL 细节
}

// InitAllReq 初始化全部服务。
//
// is_first 由调用方显式声明：true 表示这是第一次初始化，除建表与补齐种子外
// 还要创建默认管理员并完成首次授权；false 则跳过这段赋权，只做其它初始化。
struct InitAllReq {
    1: optional bool is_first (api.body="is_first")
}

// InitAllResp 返回全部服务的逐阶段结果。
struct InitAllResp {
    1: optional list<ServiceResult> results (api.body="results")
}

// InitServiceReq 只初始化指定的单个服务。
struct InitServiceReq {
    1: optional string service (api.path="service")
    2: optional bool is_first (api.body="is_first")
}

// InitServiceResp 返回指定服务的执行结果。
struct InitServiceResp {
    1: optional ServiceResult result (api.body="result")
}

// InitializeService 是初始化 API 的服务定义。
//
// POST /init/services          初始化全部服务
// POST /init/services/:service 只初始化指定服务
service InitializeService {
    InitAllResp InitAll(1: InitAllReq req) (api.post="/init/services")
    InitServiceResp InitService(1: InitServiceReq req) (api.post="/init/services/:service")
}