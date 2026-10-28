# 统一日志模块设计

| 项目 | 内容 |
| --- | --- |
| 状态 | 已实施 |
| 日期 | 2026-10-28 |
| 关联文档 | [LoggingAuditReport.MD](LoggingAuditReport.MD)（问题清单与背景） |
| 影响范围 | `conf/`、`internal/log/`、`bootstrap/`、`etc/*.yaml` |

## 1. 背景与目标

实施前 `internal/log` 存在确定性编译错误（`conf.LoggerConfig` 不存在、函数体引用未声明的 `config` 变量、`fxlogrus` 未导入），且模块当时未接入 `bootstrap.AppEntries()`，项目实际全部依赖 Logrus 默认的全局配置（文本格式、stderr、无文件输出、无轮转）。

本设计目标：

1. 修复 `internal/log` 编译缺陷并接入 Fx，使统一 Logger 真正生效。
2. 输出格式统一为 `[模块][级别] 时间: 消息`，例如 `[DB][INFO] 2026-10-28 10:51:22: database connected`（消息内容沿用业务代码原文）。
3. 支持 console 与 file 同时输出。
4. 启用文件输出时按天轮转，保留天数可配置，所有日志文件放在同一目录。
5. 压缩为可选能力，默认关闭。
6. 兼容现有全部包级 `logrus.*` 调用，业务代码零改动即可获得统一配置。

## 2. 日志格式规范

### 2.1 输出格式

```text
[MODULE][LEVEL] YYYY-MM-DD HH:mm:ss: message key=value ...
```

示例（消息内容保持业务代码原样，本设计不改写、不翻译任何已有日志文案）：

```text
[DB][INFO] 2026-10-28 10:51:22: database connected
[AUTH][WARN] 2026-10-28 10:51:25: login failed user_id=10086 reason="password mismatch"
[HTTP][ERROR] 2026-10-28 10:51:30: request handling failed error="connection refused" path=/api/user
```

规则：

- `MODULE`：大写模块名，来自 `module` 字段；缺省为 `APP`。
- `LEVEL`：大写日志级别（`DEBUG`/`INFO`/`WARN`/`ERROR`）。
- 时间：本地时区，格式 `2006-01-02 15:04:05`，秒级精度。
- 其余结构化字段（`error`、`user_id` 等）按 key 字典序追加在消息之后；`module` 字段本身不重复输出。
- 消息体原样输出 `entry.Message`：Formatter 不翻译、不改写、不包装业务日志文案，已有 `logrus.Infof("...")` 等内容保持不变。

### 2.2 模块字段来源

模块名只能来自代码常量，不接受外部输入，防止伪造日志前缀：

```go
const (
	ModuleApp   = "APP"
	ModuleDB    = "DB"
	ModuleRedis = "REDIS"
	ModuleHTTP  = "HTTP"
	ModuleRPC   = "RPC"
	ModuleAuth  = "AUTH"
	ModuleFx    = "FX"
)
```

使用方式：

```go
log.WithModule(log.ModuleDB).Info("database connected")
log.WithModule(log.ModuleAuth).WithError(err).Error("login failed")
```

`WithModule` 是 `logrus.WithField("module", name)` 的薄封装，返回 `*logrus.Entry`，与现有 `logrus.WithContext` / `WithError` 链式调用兼容。未带 `module` 字段的日志输出 `[APP]`。

### 2.3 Formatter 实现

新增 `internal/log/formatter.go`，实现 `logrus.Formatter` 接口：

```go
type ModuleFormatter struct {
	TimestampFormat string // 缺省 "2006-01-02 15:04:05"
}

func (f *ModuleFormatter) Format(entry *logrus.Entry) ([]byte, error)
```

- 实现必须是无状态的（只读 `entry`），可安全并发。
- console 与 file 共用同一 Formatter，两个输出目标内容一致。

## 3. 配置契约

### 3.1 结构体

`conf/logger.go` 扩写为：

```go
package conf

type LoggerConf struct {
	Level      string `mapstructure:"level"`        // debug|info|warn|error，默认 info
	OutputMode string `mapstructure:"output_mode"`  // console|file|all，默认 console
	Formatter  string `mapstructure:"formatter"`    // module_text|text|json，默认 module_text
	FilePath   string `mapstructure:"file_path"`    // 基础路径，file/all 模式必填
	MaxAgeDays int    `mapstructure:"max_age_days"` // 保留天数，默认 30
	Compress   bool   `mapstructure:"compress"`     // 轮转后是否 gzip，默认 false
}
```

`conf/config.go` 的 `Config` 增加字段：

```go
Logger LoggerConf `mapstructure:"logger"`
```

### 3.2 YAML 示例

```yaml
logger:
  level: info
  output_mode: all          # console + file 同时输出
  formatter: module_text
  file_path: ./logs/app.log # 基础路径；实际文件名追加日期
  max_age_days: 30
  compress: false
```

### 3.3 加载与覆盖顺序

修复 `conf/loadconf.go` 当前"环境变量与文件互斥"的问题，统一为：

```text
SetDefault / BindEnv
        ↓
读取 YAML（-f 指定，文件不存在时仅警告并使用默认值）
        ↓
AutomaticEnv（环境变量覆盖 YAML）
        ↓
Unmarshal
```

优先级：环境变量 > YAML > 默认值。

默认值在 loader 中用 `SetDefault` 注册（同时解决 `AutomaticEnv` + `Unmarshal` 无法发现未注册嵌套 key 的 Viper 行为问题）：

```go
v.SetDefault("logger.level", "info")
v.SetDefault("logger.output_mode", "console")
v.SetDefault("logger.formatter", "module_text")
v.SetDefault("logger.max_age_days", 30)
v.SetDefault("logger.compress", false)
```

环境变量绑定（显式契约，不依赖 Viper 实验特性）：

| 环境变量 | 配置键 |
| --- | --- |
| `LOG_LEVEL` | `logger.level` |
| `LOG_OUTPUT_MODE` | `logger.output_mode` |
| `LOG_FORMATTER` | `logger.formatter` |
| `LOG_FILE_PATH` | `logger.file_path` |
| `LOG_MAX_AGE_DAYS` | `logger.max_age_days` |
| `LOG_COMPRESS` | `logger.compress` |

现有 `json:",default=info,env=LOG_LEVEL"` 标签对 Viper 无效，随本次重构移除。

## 4. 输出与按天轮转设计

### 4.1 输出模式

| `output_mode` | 行为 |
| --- | --- |
| `console` | 只写 `os.Stdout` |
| `file` | 只写按天轮转的文件 |
| `all` | `io.MultiWriter(os.Stdout, dailyWriter)`，两边内容一致 |

### 4.2 按天轮转（dailyRotateWriter）

不引入新依赖。现有间接依赖 lumberjack 只支持按大小轮转，不满足"按天"需求，故实现 `internal/log/rotate.go`：

```go
type dailyRotateWriter struct {
	mu         sync.Mutex
	basePath   string      // 如 ./logs/app.log
	maxAgeDays int
	compress   bool
	now        func() time.Time // 可注入，便于测试

	file    *os.File
	curDate string // 当前文件对应日期，"2006-01-02"
}
```

行为：

1. **文件名派生**：`./logs/app.log` → `./logs/app-2026-10-28.log`。所有日期的文件都在同一目录。
2. **写路径**：`Write(p)` 持有互斥锁，比较当前日期与 `curDate`；不同则先轮转再写入。
3. **轮转**：关闭旧文件 →（可选）gzip 压缩旧文件并删除原文件 → 打开新日期文件（`O_CREATE|O_WRONLY|O_APPEND`，权限 `0o640`）→ 触发保留清理。
4. **保留清理**：启动时与每次轮转时，扫描 `basePath` 同目录下符合 `app-*.log[.gz]` 模式的文件，按文件名日期删除早于 `max_age_days` 的文件。只删模式匹配的文件，绝不触碰目录中其他内容。
5. **关闭**：`Close()` 只关闭自己打开的文件，绝不关闭 `os.Stdout`。
6. **线程安全**：所有写、轮转、关闭共享同一把互斥锁；Logrus 本身写路径已串行化，双保险成本可忽略。

压缩说明：压缩只是把已轮转的前一日文件改名 gzip，属于可选的磁盘优化，默认关闭；不影响当天日志的直接查看。

### 4.3 目录与权限

- 打开文件前先 `os.MkdirAll(dir, 0o750)`。
- 新建文件权限 `0o640`（受 umask 进一步收紧）；不主动 `chmod` 既有文件。

## 5. Fx 装配

### 5.1 构造函数

`internal/log/logger.go` 重写为：

```go
func NewLogger(
	lc fx.Lifecycle,
	cfg *conf.Config,
) (*logrus.Logger, error)
```

要点：

- 直接读 `cfg.Logger`，不新增 `*conf.LoggerConf` 投影 provider。
- **先完成全部纯校验**（level / output_mode / formatter / file 模式下 file_path 非空），再打开文件，最后一次性修改 `logrus.StandardLogger()`，保证失败路径不留下半配置的全局状态。
- 返回 `logrus.StandardLogger()`（而非 `logrus.New()`）：项目大量包级 `logrus.Info/Error/...` 调用只作用于标准 Logger，只有这样业务代码零改动即可生效。
- 打开文件后若后续步骤失败，立即关闭文件再返回错误，不泄漏句柄。
- 注册 `fx.Hook{OnStop}`：先把 `logger.SetOutput(os.Stderr)` 切换到安全输出，再关闭轮转 writer——Fx 在 hook 返回后还会写 `OnStopExecuted` / `Stopped` 事件，直接关文件会写到已关闭句柄。

### 5.2 模块定义与接入

`internal/log/0module.go`：

```go
func Module() fx.Option {
	return fx.Module(
		"internal.logger",
		fx.Provide(NewLogger),
	)
}
```

`bootstrap/appentry.go` 的 `AppEntries()` 增加：

```go
internallog.Module(),
fx.WithLogger(internallog.NewFxEventLogger),
```

- `fx.WithLogger(NewFxEventLogger)` 消费 `*logrus.Logger`，在 `fx.New` 阶段即强制实例化 Logger（Fx provider 是惰性的，仅 `Provide` 不保证执行）。
- `WithLogger` 必须放在根级 option，放在子模块内只能接管该模块的 Fx 事件。
- 配置模块（`fx.Provide(conf.NewLoadConf)`）先于日志模块声明，保证 `*conf.Config` 可用。

### 5.3 Fx 事件日志适配

`internal/log/fx.go` 删除不存在的 `fxlogrus` 引用，自实现：

```go
type FxEventLogger struct{ logger *logrus.Logger }

func NewFxEventLogger(l *logrus.Logger) fxevent.Logger
func (l *FxEventLogger) LogEvent(e fxevent.Event)
```

- 参考 `fxevent/zap.go` 的事件 type-switch，转为 `logrus.Fields`。
- 模块字段固定为 `module=FX`，从而 Fx 事件输出形如 `[FX][INFO] ...`。
- 含 `Err` 的事件 → `Error` 级别；生命周期事件（OnStart/OnStop/Invoking 等）→ `Info`；依赖图噪音（Provided/Run 等）→ `Debug`。

已知限制：`conf.NewLoadConf` 内部在产生配置前就会调用包级 `logrus.Infof`，这几条启动期日志仍使用 Logrus 默认格式；统一它们需要两阶段 bootstrap，列入非目标。

## 6. 错误处理

- 非法 `level` / `output_mode` / `formatter`：返回带具体值的包装错误，例如 `fmt.Errorf("%w: %q", ErrOutputModeNotSupported, mode)`。
- `file`/`all` 模式下 `file_path` 为空：返回明确错误。
- 构造期间不向全局 Logger 写错误日志（配置尚未生效，写了也是默认格式），直接返回 error 交给 Fx 输出。
- 轮转、清理、压缩失败不中断写路径：记录一条 `Error` 级别日志（写入当前文件）后继续服务，避免日志子系统故障拖垮业务。

## 7. 安全注意

- 日志文件权限 `0o640`，目录 `0o750`；不使用 `0666`。
- 模块名只允许代码常量（见 2.2），避免日志伪造。
- 本设计不处理日志内容脱敏；脱敏属后续独立工作项（见 LoggingAuditReport.MD §3.5、§3.11）。
- `conf.NewLoadConf` 当前的 `panic` 错误处理保持不变，不在本次范围内改动。

## 8. 测试计划

`internal/log/` 新增测试（首个 `*_test.go`，同时是仓库测试起点）：

Formatter：
- 格式精确匹配 `[DB][INFO] 2026-10-28 10:51:22: msg`（注入固定时间）。
- 缺省 `module` → `[APP]`；级别大小写正确。
- 附加字段按 key 排序输出，`module` 不重复出现。

dailyRotateWriter（注入 `now` 假时钟）：
- 同一天多次写入同一文件。
- 跨天写入触发轮转，新旧文件内容各自正确。
- `max_age_days=3` 时超期文件被删除，未超期与非匹配文件保留。
- `compress=true` 时旧文件变为 `.gz` 且原文件删除；`false` 时保留原文。
- `Close` 后文件可删除/重命名（Windows 验证句柄释放）。
- 并发写 `go test -race` 无数据竞争。

构造函数：
- 表驱动：合法/非法 level、output_mode、formatter；`file`/`all` 空路径失败，`console` 空路径成功。
- 非法 formatter 时不创建文件、不泄漏句柄。
- 失败路径后全局 Logger 状态（level/formatter/output）与构造前一致。
- 返回实例 `== logrus.StandardLogger()`；包级与实例写入同一输出。
- 操作全局 Logger 的测试禁止 `t.Parallel()`，保存并恢复全局状态。

配置加载：
- 默认值、YAML 嵌套、纯环境变量、env 覆盖 YAML、YAML 覆盖默认值。

Fx 集成：
- `WithLogger` 使 `NewLogger` 恰好执行一次；`app.Stop` 后文件有内容且可删除（验证写入器已关闭并释放句柄）。

## 9. 验收标准

1. 验收构建 `go build ./conf/... ./internal/log/... ./bootstrap/... ./api/... ./services/...` 通过（exit 0）；`internal/log` 原编译错误全部消除。
   仓库全局 `go build ./...` 仍因 `kitex_gen/base/conn`、`kitex_gen/system/access/systemapiservice` 的既有编译错误失败，与本计划无关（见 `doc/LoggingImplementationPlan.MD` 全局约束），不作为验收命令。
2. `output_mode: all` 时 console 与文件同时输出且内容一致。
3. 日志格式精确为 `[MODULE][LEVEL] YYYY-MM-DD HH:mm:ss: message`。
4. 文件日志按天生成 `app-YYYY-MM-DD.log`，全部位于同一目录。
5. 超过 `max_age_days` 的文件被自动删除；`compress: true` 时历史文件为 gzip。
6. `go test ./internal/log/ ./conf/` 全绿；全局状态相关测试串行。
7. 现有业务代码不改一行即获得统一格式与文件输出。

## 10. 实施顺序

1. `conf/`：`LoggerConf` 扩字段 → `Config` 挂载 → loader 默认值/BindEnv/非互斥加载 → `etc/*.yaml` 增加 `logger` 节点。
2. `internal/log/`：`formatter.go`（ModuleFormatter）→ `rotate.go`（dailyRotateWriter）→ `logger.go`（重写 NewLogger）→ `fx.go`（FxEventLogger）→ 模块常量与 `WithModule`。
3. `bootstrap/`：`AppEntries()` 接入 `internallog.Module()` 与根级 `fx.WithLogger`。
4. 测试与验证（按第 8 节）。

## 11. 非目标

- 日志内容脱敏（独立工作项）。
- 接入 OpenTelemetry Trace ID 注入（独立工作项）。
- 集中式采集（Loki/ELK）与 JSON 生产格式切换——`formatter: json` 已在配置面预留。
- 两阶段 bootstrap 统一配置加载期的启动日志。
- 各入口 `main.go` 未调用 `app.Run()`/`Stop()` 的生命周期问题（另行修复）。
