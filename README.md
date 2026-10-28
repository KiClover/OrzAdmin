# orzAdmin

## 简介

`orzAdmin` 是一个基于Kitex和Hertz开发面向中大型项目的分布式微服务后端管理系统脚手架。


## 核心特性

- 用户注册与认证：支持邮箱或手机号验证码注册、密码登录、验证码登录、密码找回和图形验证码。
- 用户管理：用户创建、更新、删除、分页查询、用户详情、个人资料和密码修改。
- API 与角色管理：维护 API 路径、HTTP 方法、分组、角色编码、角色名称和备注。
- Casbin RBAC：内置完善的动态路由权限生成方案, 集成RBAC权限控制
- 系统设置：Kitex 设置服务保存注册默认角色、邮件服务商和短信服务商配置。
- 消息服务：支持短信和邮箱验证码
- 服务治理：Kitex 和 Hertz 均支持 etcd 或 Nacos 注册与发现。
- 可观测性基础：Kitex 服务运行器接入 OpenTelemetry tracing，日志使用 logrus。

## 技术栈

| 类别 | 组件 | 版本或说明 |
| --- | --- | --- |
| 语言 | Go | `go 1.24.4` |
| HTTP | CloudWeGo Hertz | `v0.10.3` |
| RPC | CloudWeGo Kitex | `v0.15.2` |
| IDL | Thrift、Protocol Buffers | Thrift 用于 HTTP 模型，Proto 用于 Kitex 服务 |
| ORM / 迁移 | Ent | `v0.14.5` |
| 权限 | Casbin | `v2.103.0`，Ent Adapter 和 Redis Watcher |
| 配置 | Viper | `v1.21.0` |
| 数据库 | MySQL 驱动；PostgreSQL/SQLite DSN 分支 | 当前数据库实例显式导入 MySQL 驱动；Casbin 实际固定使用 MySQL Adapter |
| 缓存 | Redis | `go-redis/v9` |
| 服务发现 | etcd、Nacos | 通过 `DiscoveryConf.Method` 选择 |
| 认证 | JWT | `golang-jwt/jwt/v5` |
| 验证码 | base64Captcha | 图形验证码存 Redis |
| 邮件 | go-mail | SMTP SSL、TLS 或无加密模式 |
| 日志 | logrus | 默认使用标准日志输出 |
| 依赖注入 | Uber Fx | 负责应用启动、服务装配和生命周期 |
| 链路追踪 | OpenTelemetry | 当前代码创建 tracing span |



## 项目目录

```text
.
├── api/                         # Hertz HTTP API 层、请求模型和路由
│   ├── model/                   # 由 Thrift 生成的 HTTP 请求/响应模型
│   └── system/                  # auth、user、permission、setting、token
├── bootstrap/                   # Fx 应用装配和启动入口
├── conf/                        # 配置结构体和 Viper 加载逻辑
├── db/                          # Ent Client 到各业务服务的数据库封装
├── doc/                         # RBAC、错误码和 API JSON 文档
├── ent/                         # Ent Schema、生成代码和迁移代码
│   └── schema/                  # user、role、api、setting、msg、token
├── etc/                         # 服务配置 YAML 和 etc/casbin.conf
│   └── api/                     # HTTP 服务配置
├── idl/                         # Thrift、Proto IDL 源文件
├── instance/                    # 数据库、Redis、Casbin 实例
├── internal/                    # 服务发现、RPC Client、中间件和启动器
├── kitex_gen/                   # Kitex/Proto 生成代码
├── pkg/                         # JWT、验证码、响应和通用工具
├── services/                    # Kitex 业务服务实现
├── script/                      # 辅助脚本
├── Makefile                     # Ent/Kitex 代码生成和部分运行命令
├── run-all.ps1                  # Windows 本地多进程启动脚本
├── build.sh                     # 现有构建脚本，主要面向 system.user
├── go.mod / go.sum              # Go 模块依赖
└── README.md
```

`api/model`、`kitex_gen` 和部分 `ent` 文件是生成代码。修改接口时应先修改 `idl` 或 `ent/schema`，再使用生成命令更新代码，不建议直接编辑生成文件。

Makefile 的代码生成、单服务启动和参数说明见 [Makefile 使用指南](doc/Makefile.MD)。

## 运行环境

最低要求和默认依赖如下：

- Go `1.24.4` 或更高版本。
- MySQL 8.x 或兼容版本。仓库示例配置使用 MySQL，Casbin Adapter 当前也固定使用 MySQL。
- Redis 6.x 或更高版本
- etcd 3.x，也可以配置 Nacos 作为服务发现组件。

## 安装与本地启动

### 1. 获取代码和依赖

```bash
git clone https://github.com/KiClover/orzAdmin.git
cd orzAdmin
go mod tidy
```

### 2. 准备数据库

默认配置使用数据库 `core`。先创建数据库，表由项目的初始化接口创建：

```sql
CREATE DATABASE core
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;
```

然后修改各 `etc/*.yaml` 和 `etc/api/*.yaml` 中的数据库用户名、密码、地址和数据库名。不要将示例中的 `123456` 用于生产环境。

### 3. 启动依赖服务

确保 MySQL、Redis 和 etcd 已启动。例如本地默认地址为：

```text
MySQL  127.0.0.1:3306
Redis  localhost:6379
etcd   127.0.0.1:2379
```

当前默认配置使用 etcd：

```yaml
DiscoveryConf:
  Method: etcd

EtcdConf:
  Endpoints: ["127.0.0.1:2379"]
```

如果使用 Nacos，需要在所有服务配置中将 `DiscoveryConf.Method` 改为 `nacos`，并补全 `NacosConf`。详细字段见[配置说明](#配置说明)。

### 4. 启动服务

#### Windows 一键启动

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\run-all.ps1
```

脚本会为以下进程打开独立的命令窗口：

```text
system-user              etc/user.yaml
system-setting           etc/setting.yaml
system-message           etc/msg.yaml
system-access            etc/access.yaml
api.system.auth          etc/api/auth.yaml
api.system.user          etc/api/user.yaml
api.system.permission    etc/api/permission.yaml
```

停止服务可以关闭对应窗口或在窗口中按 `Ctrl+C`。

#### 手动启动

每条命令需要在单独的终端中运行：

```bash
go run services/system/user/cmd/main.go -env=false -f=etc/user.yaml
go run services/system/setting/cmd/main.go -env=false -f=etc/setting.yaml
go run services/system/message/cmd/main.go -env=false -f=etc/msg.yaml
go run services/system/access/cmd/main.go -env=false -f=etc/access.yaml

go run api/system/auth/cmd/main.go -env=false -f=etc/api/auth.yaml
go run api/system/user/cmd/main.go -env=false -f=etc/api/user.yaml
go run api/system/permission/cmd/main.go -env=false -f=etc/api/permission.yaml
```

`-env=false` 表示读取 `-f` 指定的 YAML 文件。`-f` 是相对当前工作目录解析的路径。

### 5. 初始化表和基础数据

服务启动后，先初始化用户、设置和消息表：

```bash
curl -X POST http://127.0.0.1:8889/init/database
```

再初始化 API、角色、Casbin 策略和管理员用户：

```bash
curl -X POST http://127.0.0.1:8891/init/database
```

这组凭据来自当前代码，仅适用于本地初始化。首次登录后应立即修改密码；对外部署前应修改初始化逻辑并删除或保护初始化接口。

检查服务健康状态：

```bash
curl http://127.0.0.1:8889/health
curl http://127.0.0.1:8890/health
curl http://127.0.0.1:8891/health
```

健康接口返回类似：

```json
{"status":"ok"}
```

## 配置说明

### 配置加载方式

配置加载代码位于 `conf/loadconf.go`，支持以下启动参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-env` | `true` | 为 `true` 时启用 Viper 环境变量读取；为 `false` 时读取配置文件 |
| `-f` | `etc/cfg.yaml` | `-env=false` 时使用的 YAML 文件 |

### YAML 配置字段

各文件字段基本相同，API 文件额外包含认证和 Casbin 数据库配置。字段含义如下：

#### `DatabaseConf`

| 字段 | 说明 |
| --- | --- |
| `Type` | 数据库类型，DSN 分支包含 `mysql`、`postgres`、`sqlite3` |
| `Host` / `Port` | 数据库地址和端口 |
| `Username` / `Password` | 数据库账号和密码 |
| `DBName` | 数据库名称，示例为 `core` |
| `SSLMode` | PostgreSQL DSN 的 `sslmode` 参数 |
| `MaxOpenConn` | 最大打开连接数，示例为 100 |
| `CacheTime` | 配置字段已定义，当前数据库连接实现没有使用该值 |
| `DBPath` | SQLite 数据库文件路径 |
| `MysqlConfig` / `PGConfig` / `SqliteConfig` | 追加到对应 DSN 的额外参数 |


## 许可证与联系方式

MIT LICENSE

项目模块路径和仓库地址为：

- GitHub：<https://github.com/KiClover/orzAdmin>
- 问题反馈和功能建议：<https://github.com/KiClover/orzAdmin/issues>

联系方式以 GitHub Issues 为准。提交问题时请附上 Go 版本、操作系统、使用的配置文件、依赖服务版本、完整错误日志以及最小复现步骤，并注意移除密码、JWT Secret、验证码和第三方服务密钥。
