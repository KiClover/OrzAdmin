# orzAdmin 日志与可观测性审查报告

| 项 | 内容 |
| --- | --- |
| 日期 | 2026-10-28 |
| 状态 | 部分修复（3.1、3.2、3.3 已修复，其余待修复） |
| 审查方式 | 静态源码审查 |
| 审查范围 | Logrus 初始化、HTTP/Kitex 日志链路、OpenTelemetry、错误边界、登录与操作审计、日志安全和测试 |
| 不在范围 | 本次不修改代码、不部署可观测性平台、不验证生产环境配置 |

---

## 1. 结论摘要（TL;DR）

当前项目只有零散的运行时错误输出和部分 OpenTelemetry 埋点，尚未形成可用的日志与审计体系。本次审查确认：

1. `internal/log` 从静态源码看无法编译，且没有接入 Fx 启动流程；运行时实际仍使用默认全局 Logrus。
2. Kitex 服务端错误处理器存在无限循环路径，部分错误请求可能永久挂起，属于最高优先级可用性问题。
3. OpenTelemetry 没有 Provider、Exporter、Propagator 和关闭流程，现有 Span 默认不可导出；HTTP 到 RPC 的 Trace 链路也未贯通。
4. HTTP 层没有访问日志、Request ID、恢复日志和统一结构化字段，无法按请求定位故障。
5. 现有日志可能输出完整 HTTP Request，造成 Token、SecretKey、Cookie、密码或验证码泄露。
6. 部分接口把数据库或 RPC 原始错误直接返回客户端，内部诊断信息与对外错误响应没有隔离。
7. 运行时可返回错误使用 `Fatalf`/`Panicf`，可能直接终止进程并绕过资源清理。
8. 项目没有登录日志、管理员操作审计、查询 API、日志专属 RBAC、导出和保留策略。
9. 仓库没有任何 `*_test.go`，上述关键错误边界均没有回归保护。

建议先处理 Kitex 死循环、敏感信息泄露和进程退出风险，再修复统一 Logger 与 Trace 基础设施，最后建设登录日志和操作审计。

---

## 2. 风险分级

| 等级 | 问题 | 主要影响 |
| --- | --- | --- |
| P0 | Kitex 错误处理可能无限循环 | 错误请求挂死、资源耗尽、拒绝服务 |
| P0 | `internal/log` 静态编译缺陷且未接入 | 日志配置不可用、生产输出不可控 |
| P0 | 完整 Request 可能进入日志 | Token、SecretKey、密码等敏感信息泄露 |
| P0 | Tracing 没有 Provider/Exporter | Span 不可导出，故障链路不可追踪 |
| P1 | 内部错误原文返回客户端 | 数据库/RPC 实现细节泄露 |
| P1 | 运行路径使用 `Fatalf`/`Panicf` | 单次请求或服务异常导致进程退出 |
| P1 | HTTP 无访问日志和 Request ID | 无法按请求检索状态、耗时和调用主体 |
| P1 | Trace 在 HTTP→RPC Client 处断链 | 跨服务调用无法还原 |
| P1 | 文件日志无轮转、关闭和安全权限 | 文件泄露、句柄泄漏、磁盘耗尽 |
| P1 | 缺少登录与操作审计 | 无法追责、调查异常登录和高权限操作 |
| P2 | Span 状态与结构化属性不足 | 指标和告警不准确、检索困难 |
| P2 | Redis 启动失败后仍返回客户端 | 基础设施故障延迟到业务阶段暴露 |
| P2 | 无自动化测试 | 修复后容易回归 |

---

## 3. 详细发现

### 3.1 P0：Kitex 错误处理器可能无限循环

**证据**

`internal/constructor/service_runner.go:58-68`：

```go
var de *kerrors.DetailedError
for kerrors.IsKitexError(err) {
    if ok := errors.As(err, &de); ok {
        switch de.ErrorType() {
        case kerrors.ErrBiz, kerrors.ErrRemoteOrNetwork:
            err = de.Unwrap()
        default:
            break
        }
    }
}
```

**根因**

- `default` 中的 `break` 只退出 `switch`，不退出外层 `for`。
- 对于不属于 `ErrBiz` 或 `ErrRemoteOrNetwork` 的 Kitex 错误，`err` 不发生变化，循环条件持续为真。
- 如果 `kerrors.IsKitexError(err)` 为真但 `errors.As(err, &de)` 为假，循环体同样不会改变 `err`。
- `Unwrap()` 返回 `nil` 或不能推动错误链时也没有保护。

**影响**

- 部分 RPC 错误请求永久挂起。
- `span.RecordError(err)` 和错误返回无法执行。
- 并发错误请求可能持续占用 goroutine、连接和内存，形成拒绝服务风险。

**修复建议**

把解包逻辑抽成独立函数；遇到不能继续解包、非目标错误类型或空 Cause 时立即返回。记录根因错误，但优先向 Kitex 返回原始错误，以保留框架错误分类和元数据。补充「超时测试」，证明非目标 Kitex 错误不会卡住。

**修复记录（2026-10-28，已完成）**

- `internal/constructor/service_runner.go:104-125` 新增 `unwrapKitexError(err error) error`：只有 `kerrors.ErrBiz`、`kerrors.ErrRemoteOrNetwork` 且 `Unwrap()` 非空时才把游标推进到 cause；其余情况（非 Kitex 错误、其他错误类型、空 Cause、`errors.As` 失败）立即 `return err`，循环每轮必然推进，不存在原地打转路径。
- `internal/constructor/service_runner.go:89-99` 新增 `newServerErrorHandler(serviceName string) func(ctx context.Context, err error) error`：解包后的根因只用于 `span.RecordError`，**对外返回原始 `err`**，保留 Kitex 错误分类和业务状态码（旧代码会把 `ErrBiz` 包装错误替换成 cause，等于丢失业务错误码）。
- `internal/constructor/service_runner.go:54` 用 `server.WithErrorHandler(newServerErrorHandler(serviceName))` 替换内联闭包。
- 回归测试 `internal/constructor/service_runner_test.go` 共 7 个用例：非 Kitex 错误原样返回、非目标类型（`kerrors.ErrNoConnection`，即真实触发死循环的路径）立即返回、`ErrBiz` 解包到 cause、多层嵌套解包、空 Cause 保留原错误、处理器返回原始错误、处理器对 `nil` 错误返回 `nil`。
- 缺陷复现证据：把旧循环逻辑原样写成临时测试，`go test -timeout 10s` 输出 `panic: test timed out after 10s / running tests: TestReproLegacyUnwrapLoop`，确认是永久挂起而非慢返回；该临时文件已删除。
- 验证命令与结果：`go test ./internal/constructor/ -v` 全部 PASS；`go vet ./internal/constructor/` 通过；`gofmt -l` 无输出；`go build ./api/... ./services/...`（全部依赖 `RunService` 的 10 个入口包）退出码 0。

### 3.2 P0：日志子系统静态编译缺陷

**证据**

- `internal/log/logger.go:20` 使用不存在的 `conf.LoggerConfig`。
- 同一函数在 `internal/log/logger.go:27-65` 使用未定义标识符 `config`，而参数名是 `cfg`。
- `internal/log/fx.go:8-9` 使用 `fxlogrus.LogrusLogger`，但没有导入或定义 `fxlogrus`。
- 实际配置类型 `conf/logger.go:3-5` 只有 `LoggerConf.Level`，没有 OutputMode、FilePath、Formatter。
- 根配置 `conf/config.go:3-13` 没有 Logger 字段。

**影响**

计划中的级别、JSON、文件和 caller 配置不可用。即使其他包暂未导入 `internal/log` 而没有触发编译，该模块也不能作为生产日志入口使用。

**修复建议**

统一配置类型和字段，删除 `cfg/config` 命名分裂；选定并正确导入 Fx Logrus adapter；为非法 Level、OutputMode、Formatter 和文件打开失败增加单元测试。

**修复记录（2026-10-28，已修复）**

按 `doc/LoggingImplementationPlan.MD` 完成 conf 契约、NewLogger 重写、FxEventLogger 自实现与 bootstrap 接线。本条目状态已由"待修复"更新为"已修复"。

### 3.3 P0：日志模块没有接入启动流程

**证据**

- `bootstrap/appentry.go:14-26` 未加载 `internal/log.Module()`。
- `bootstrap/bootstrap.go:15-22` 未使用 `fx.WithLogger(...)`。
- 业务代码普遍直接调用全局 `logrus`。

**影响**

即使修复 `internal/log` 的编译问题，当前应用仍只会使用默认全局 Logrus；配置中的日志级别、格式和输出目的地不会生效。

**修复建议**

在配置加载之后、其他业务 Provider 之前初始化 Logger；通过 Fx 注入和 `fx.WithLogger` 统一框架日志。明确禁止业务代码自行创建 Logger 或修改全局配置。

**修复记录（2026-10-28，已修复）**

按 `doc/LoggingImplementationPlan.MD` 完成 conf 契约、NewLogger 重写、FxEventLogger 自实现与 bootstrap 接线。本条目状态已由"待修复"更新为"已修复"。

### 3.4 P0：现有 Trace 默认不可导出

**证据**

- `bootstrap/bootstrap.go:9-13`、`internal/constructor/service_runner.go:22,29` 等位置直接使用全局 `otel.Tracer`。
- Kitex Server 使用 `tracing.NewServerSuite()`：`internal/constructor/service_runner.go:44-53`。
- 仓库未发现 `otel.SetTracerProvider`、SDK Tracer Provider、OTLP Exporter、Propagator、Shutdown 或 ForceFlush。

**影响**

默认 Provider 下的手工 Span 不会形成可查询的 Trace。服务看起来已经“埋点”，但生产故障时没有可用链路数据。

**修复建议**

增加 OTel 生命周期模块：创建 Resource、TracerProvider、BatchSpanProcessor 和 OTLP Exporter；设置 W3C Trace Context/Baggage Propagator；在 Fx Stop Hook 中带超时执行 Shutdown。

### 3.5 P0：日志可能泄露认证凭据和请求体

**证据**

`internal/middleware/casbin.go:28-31` 在缺少 `userId` 时记录整个 `c.Request`：

```go
logrus.Errorf("get user id from context err:%v", c.Request)
```

请求实际使用：

- `Authorization`：`internal/middleware/jwt.go:62-71`。
- `SecretKey`：`internal/middleware/token.go:25-31`。
- 登录接口请求体可能包含密码、验证码、手机号或邮箱。

**威胁边界**

HTTP 请求是外部不可信输入；日志系统通常被集中采集并对更多运维人员开放。把完整 Request 写入日志，会扩大凭据和个人信息的暴露面。

**修复建议**

删除完整 Request 输出，采用字段白名单，仅记录 request_id、route、method、status、actor_id 等诊断字段。统一脱敏 Hook，禁止输出 Authorization、Cookie、SecretKey、Token、密码、验证码和服务密钥。

### 3.6 P1：HTTP 层缺少访问日志和恢复日志

**证据**

`internal/constructor/hz_service_runner.go:62-64` 创建 Hertz 后只注册 CORS：

```go
h := server.New(options...)
h.Use(middleware.NewCORSMw())
```

未发现访问日志、Request ID、panic recovery 日志或 Hertz tracing 中间件。

**影响**

无法统一检索请求方法、路由、状态码、业务错误码、耗时、响应大小、客户端 IP、用户、拒绝原因和异常堆栈。

**修复建议**

按固定顺序挂载：Recovery → Request ID → Tracing → Access Log → CORS → Auth/RBAC。访问日志使用路由模板而非完整 URL，避免 query 中的敏感信息进入日志。

### 3.7 P1：HTTP→Kitex Client→Server Trace 断链

**证据**

- `internal/client/system/client.go:26-84` 的五个 Client 仅配置 resolver、RPC timeout 和 connect timeout。
- 未配置 `tracing.NewClientSuite()` 或等价的客户端传播机制。
- Hertz 入口也没有 tracing 中间件。

**影响**

即使补齐 Exporter，HTTP Span、RPC Client Span 和 RPC Server Span 仍无法组成同一条 Trace。

**修复建议**

同时配置 Hertz Server instrumentation、Kitex Client Suite、Kitex Server Suite 和统一 Propagator；增加一个 HTTP 调用下游 RPC 的端到端传播测试。

### 3.8 P1：日志与 Trace 无关联

**证据**

业务层主要使用 `logrus.Errorf(...)`。少量 `logrus.WithContext(ctx)` 也不会自动从 Context 提取 `trace_id` 和 `span_id`。仓库没有相关 Hook。

**影响**

从错误日志无法跳转到 Trace，从 Trace 也无法反查同一请求的业务日志。

**修复建议**

提供统一的 Context Logger 或 Logrus Hook，从有效 SpanContext 注入 `trace_id`、`span_id`，并补充 `service.name`、`environment`、`version`、`request_id` 和 `operation`。

### 3.9 P1：内部错误直接返回客户端

**证据**

- `pkg/hresp/hresp.go:57-62` 的 `KitexCallErr` 把原始错误用 `%v` 拼入响应。
- `internal/middleware/token.go:31-43` 把数据库查询错误直接返回 401 响应。

**影响**

客户端可能看到数据库、RPC、网络地址、表结构或依赖实现细节，为攻击者提供枚举信息；同时 401 被错误地用于服务端查询异常。

**修复建议**

对外仅返回稳定业务错误码、通用消息和 Request ID；对内日志记录完整错误链。认证失败使用 401/403，依赖或数据库故障使用 5xx，二者不能混用。

### 3.10 P1：运行路径使用 `Fatalf` 和 `Panicf`

**证据**

- `services/system/message/templete.go:214-227` 在模板解析失败时调用 `logrus.Fatalf`。
- 该模板函数由请求流程调用：`services/system/message/handler.go:101`。
- `internal/constructor/service_runner.go:82-87` 在 Server goroutine 返回错误时调用 `Panicf`。

**影响**

`Fatalf` 会调用 `os.Exit(1)`，绕过 Fx Stop Hook、日志文件关闭和 Trace Flush。单次业务请求不应拥有终止整个服务进程的能力。

**修复建议**

模板在启动阶段预编译并注入，解析失败让启动明确失败；运行阶段返回错误而不是 Fatal。Server 运行异常应上报给统一生命周期管理，由进程入口决定退出策略。

### 3.11 P1：文件日志存在权限、生命周期和磁盘风险

**证据**

`internal/log/logger.go:38-52`：

- 以 `0666` 创建日志文件，实际权限依赖进程 umask。
- 打开的文件未保存为可关闭资源，也没有 Fx Stop Hook。
- 没有轮转、保留、压缩、容量上限或磁盘写满策略。

**影响**

同机其他用户可能读取或修改日志；文件描述符无法优雅关闭；日志持续增长可能耗尽磁盘并影响整个节点。

**修复建议**

容器环境优先输出 JSON 到 stdout，由平台采集。如必须写文件，使用最小权限、显式 Close、轮转、保留期、压缩和容量监控。

### 3.12 P1：缺少登录日志和管理员操作审计

**证据**

当前 Ent Schema、HTTP/RPC 模块和 IDL 中没有 login log、operation log 或 audit log。登录处理只负责校验和签发 Token，JWT/Casbin 只负责认证鉴权，不记录安全事件。

**影响**

无法回答：谁在何时从哪里登录、谁修改了角色或权限、操作是否成功、失败原因是什么。异常登录和高权限操作无法调查，也无法满足不可抵赖性需求。

**修复建议**

分别建设：

- 登录日志：用户、登录方式、成功/失败、原因分类、IP、UA、request/trace ID、时间。
- 操作审计：actor、action、resource、result、error_code、method/route、IP/UA、request/trace ID、时间。

审计数据采用字段白名单，禁止存储密码、验证码、Token 和服务密钥；为查看、详情、导出和保留策略配置独立 RBAC 权限。

### 3.13 P2：Span 错误语义和属性不足

**证据**

仓库只在少量位置使用 `span.RecordError`，未发现 `span.SetStatus(codes.Error)`，也没有统一的 service、route、rpc.system、peer 等属性。

**影响**

部分后端会把已记录 Exception 但未设置 Status 的 Span 继续视为非错误，导致错误率、筛选和告警不准确。

**修复建议**

错误路径同时调用 `RecordError` 和 `SetStatus(codes.Error, ...)`；按 OTel Semantic Conventions 添加属性，但不要把敏感参数或完整 SQL 写入 Span。

### 3.14 P2：Redis 初始化失败后仍返回客户端

**证据**

`instance/redis.go:32-43` 创建 Client 并 Ping；Ping 失败时仅记录错误，随后仍返回该 Client。对应 Span 不记录错误，应用启动也不会失败。

**影响**

基础设施故障被推迟到业务请求阶段暴露，启动健康状态与实际可用性不一致。

**修复建议**

将构造函数改为返回 `(redis.UniversalClient, error)`；Ping 失败时关闭 Client、记录 Span 错误并让依赖注入失败。若业务允许 Redis 降级，应显式定义降级状态和健康检查，而不是静默继续。

### 3.15 P2：日志缺少结构化上下文

**证据**

服务层大量使用格式化字符串日志，没有统一的 service、operation、request_id、user_id、result、error_code 字段。`SetReportCaller(true)` 只存在于未接入的日志工厂。

**影响**

集中检索时只能依赖文本模糊匹配，难以聚合、去重、统计和告警。

**修复建议**

定义稳定字段字典，业务代码使用 `WithFields` 或统一封装；字段名和类型必须保持一致，错误对象通过 `WithError` 记录。

### 3.16 P2：配置加载早于日志初始化

**证据**

`conf/loadconf.go:15-37` 在 Logger 初始化前使用默认全局 Logrus；配置读取和反序列化失败直接 panic。

**影响**

启动阶段最关键的故障无法使用统一格式、服务名和 Trace 信息记录，也缺少可审计的配置来源和版本。

**修复建议**

提供最小 bootstrap logger，配置加载成功后切换到正式 Logger；只记录配置来源、版本和哈希，绝不打印完整配置或秘密值。

### 3.17 P2：没有自动化测试

**证据**

静态扫描未发现任何 `*_test.go`。

**至少需要覆盖**

1. Kitex 非目标错误类型不会死循环，并在限定时间内返回。
2. Logger 的 level、output、formatter、文件权限和关闭。
3. HTTP 访问日志必需字段和 Authorization/SecretKey/Cookie/密码脱敏。
4. Trace Provider、导出、跨 HTTP/RPC 传播和 Shutdown。
5. Redis Ping 失败时的启动行为。
6. 登录日志和操作审计的成功、失败及敏感字段白名单。
7. 对外错误响应不包含内部错误原文。

---

## 4. 建议修复顺序

### 阶段一：立即止血

1. （已完成，见 3.1 修复记录）修复 Kitex 错误处理死循环，并先增加回归测试。
2. 删除完整 Request 日志，建立敏感字段白名单。
3. 隔离对外错误响应和内部诊断错误。
4. 移除请求路径中的 `Fatalf`，明确服务异常退出策略。

### 阶段二：恢复基础日志和 Trace

1. 修复 `internal/log` 类型、变量和 Fx adapter。
2. 把统一 Logger 接入 Fx 生命周期。
3. 增加 Request ID、Recovery 和 HTTP Access Log。
4. 配置 OTel Provider、Exporter、Propagator 和 Shutdown。
5. 接入 Hertz tracing 与 Kitex Client Suite。
6. 注入 trace_id/span_id 和标准结构化字段。

### 阶段三：建设业务审计

1. 建立登录日志与操作审计数据模型。
2. 覆盖登录、用户、角色、API、Casbin 和系统设置等关键事件。
3. 增加分页、筛选、详情和独立 RBAC 权限。
4. 增加导出、保留、清理、归档与异常告警。
5. 接入 Loki、ELK 或 OpenSearch 等集中检索平台。

---

## 5. 验收标准

修复完成后至少满足：

- 任意 Kitex 错误都能在限定时间内返回，不出现 goroutine 卡死。
- 生产日志为结构化格式，包含 service、environment、request_id、trace_id、operation 和 result。
- 日志及 Trace 中不包含 Authorization、Cookie、SecretKey、密码、验证码和完整 Token。
- 客户端响应不包含数据库、RPC 或堆栈原文，只返回稳定错误码和 Request ID。
- 一次 HTTP→Kitex RPC 请求可以在 Trace 后端看到完整父子关系。
- 服务关闭时 Logger 和 Tracer 能在超时范围内完成 Flush/Shutdown。
- Redis 等必要依赖不可用时，启动健康状态能准确反映失败或显式降级。
- 登录成功/失败及关键管理员操作都有可查询、受 RBAC 保护的审计记录。
- 关键错误边界有自动化测试，且 `go test ./...` 通过。

---

## 6. 验证限制与工作区说明

- 本报告基于静态源码审查，没有修改业务代码，也没有启动完整微服务环境。
- 审查期间尝试编译 `internal/log` 时，运行环境对 Go 缓存目录的访问受限，未取得完整编译器输出；`conf.LoggerConfig`、`config` 和 `fxlogrus` 等问题由源码静态确认。
- 后续验证 3.1 修复时改用 `GOCACHE=<临时目录>` 绕开 `D:\gocache` 的权限问题，`go test`、`go vet`、`go build` 均可正常执行；此时 `go build ./...` 仍会在 `internal/log` 与 `kitex_gen` 报错，属本节记录的既有缺陷，与 3.1 无关（`go build ./api/... ./services/...` 退出码 0）。
- `doc/DistributedTransaction.MD` 是审查前已经存在的未跟踪文件，与本次日志审查无关。
