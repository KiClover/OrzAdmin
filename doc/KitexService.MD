# 新增 Kitex 服务接入指南

本文说明在本项目中新增一个 Kitex RPC 服务时，需要新增或修改哪些文件，以及各类 Fx 依赖应该注册在哪里。

通用占位符：

- `<scope>`：业务域，例如 `system`、`finance`。
- `<name>`：服务名，例如 `user`、`bill`。
- 示例：`finance/bill`，服务发现名为 `finance-bill`，Proto Service 为 `FinanceBillService`。

> Kitex 生成代码只是第一步。服务要真正启动，还必须实现生成接口、注册服务端 provider、增加启动入口和运行配置。RPC Client、数据库和 HTTP API 都是按需项。

## 1. 项目装配链

每个 Kitex 服务的 `cmd/main.go` 都走同一条装配链：

```text
services/<scope>/<name>/cmd/main.go
  -> bootstrap.NewApp
  -> bootstrap.AppEntries
     -> conf.NewLoadConf
     -> instance.Module
     -> discovery.Module
     -> db.Module
     -> services.Module
     -> client.Module
     -> api.Module
  -> constructor.RunService(generated.NewServer)
```

相关源码：

- [`bootstrap/bootstrap.go`](../bootstrap/bootstrap.go)：创建 Fx App，并提供 `context.Context` 和当前 `ServiceName`。
- [`bootstrap/appentry.go`](../bootstrap/appentry.go)：装入配置、实例、服务发现、DB、服务端实现、RPC Client 和 API 模块。
- [`internal/constructor/service_runner.go`](../internal/constructor/service_runner.go)：创建 Kitex Server、注册服务发现、绑定监听地址并管理生命周期。

新增服务通常不需要修改这些通用文件，只需要把 provider 接入现有模块树。

## 2. 必选与按需文件

### 2.1 新服务端必选

| 文件或目录 | 作用 |
| --- | --- |
| `idl/<scope>/<name>.proto` | 定义消息和 RPC Service |
| `kitex_gen/<scope>/<name>/**` | Kitex 生成的消息、Client 和 Server 代码 |
| `services/<scope>/<name>/handler.go` | 实现生成的 Service 接口 |
| `services/<scope>/<name>/cmd/main.go` | 服务独立启动入口 |
| `services/<scope>/0module.go` | 注册服务端实现 provider |
| `services/0module.go` | 仅第一次新增业务域时注册域模块 |
| `etc/...yaml` | 监听端口、服务发现及其他运行配置 |

### 2.2 按需项

| 条件 | 需要处理的位置 |
| --- | --- |
| 其他进程需要调用该服务 | `internal/client/<scope>/client.go`、域级和根级 Client Module |
| 服务需要调用其他 RPC | 当前 handler 的 `fx.In` 中注入对应生成 Client |
| 服务需要 Ent 数据库 | `ent/schema`、`db/<name>.go`、`db/0module.go` |
| 需要 Make 快捷启动 | `Makefile` |
| 需要 Windows 一键启动 | `run-all.ps1` |
| 需要暴露 HTTP API | Thrift IDL、`api/model`、`api/<scope>` handler/router/module/cmd |

## 3. 编写 IDL 并生成代码

`idl/finance/bill.proto` 示例：

```proto
syntax = "proto3";

package finance.bill;
option go_package = "finance/bill";

import "idl/base.proto";

message GetBillReq {
  int64 id = 1;
}

message BillInfo {
  int64 id = 1;
}

service FinanceBillService {
  rpc getBill(GetBillReq) returns (BillInfo);
}
```

以下信息必须一致：

```text
业务域        finance
Proto package finance.bill
Go package    finance/bill
服务目录      services/finance/bill
服务发现名称  finance-bill
```

### 3.1 使用 Makefile 生成

[`Makefile`](../Makefile) 的 `kx`/`hz` 支持 `SCOPE` 变量，默认 `system`：

```bash
make kx user.proto
# 实际执行 kitex idl/system/user.proto

make kx bill.proto SCOPE=finance
# 实际执行 kitex idl/finance/bill.proto
```

`hz` 同理：`make hz user_api.thrift SCOPE=finance`。

不要直接编辑 `kitex_gen`。生成后检查：

```text
kitex_gen/finance/bill/bill.pb.go
kitex_gen/finance/bill/financebillservice/client.go
kitex_gen/finance/bill/financebillservice/server.go
```

Kitex 更新不会可靠删除已经从 Proto 中移除的旧 Service 目录。发现陈旧生成物时应人工确认，不要自动删除。

## 4. 实现服务端 handler

新建 `services/finance/bill/handler.go`：

```go
package bill

import (
    "context"
    "errors"

    billpb "github.com/KiClover/orzAdmin/kitex_gen/finance/bill"
    "go.uber.org/fx"
)

const ServiceName = "finance-bill"

type FinanceBillService struct {
    fx.In

    // 只注入真实使用的依赖。
}

var _ billpb.FinanceBillService = (*FinanceBillService)(nil)

func NewFinanceBillService(svc FinanceBillService) billpb.FinanceBillService {
    return &svc
}

func (s *FinanceBillService) GetBill(
    ctx context.Context,
    req *billpb.GetBillReq,
) (*billpb.BillInfo, error) {
    return nil, errors.New("FinanceBillService.GetBill: not implemented")
}
```

要求：

1. `ServiceName` 全局唯一，推荐 `<scope>-<name>`。
2. 保留编译期接口断言。
3. 实现生成接口中的全部 RPC。
4. `fx.In` 只放实际使用的依赖；每个字段都会成为 Fx 依赖图中的硬依赖。
5. 不要为保持旧文件外观而注入未使用的 `*conf.Config`、Casbin、Redis 或 Client。

如果服务没有任何依赖，不需要空的 `fx.In`：

```go
type FinanceBillService struct{}

func NewFinanceBillService() billpb.FinanceBillService {
    return &FinanceBillService{}
}
```

当前大多数文件使用 `handler.go`；`services/system/token/handle.go` 是命名异例。新服务统一使用 `handler.go`。

## 5. 注册服务端实现

服务端 provider 与 RPC Client provider 是两回事：

- `services/.../0module.go`：让 Kitex Server 获得 handler，所有新服务都需要。
- `internal/client/.../0module.go`：让其他进程远程调用该服务，只有存在调用方时才需要。

### 5.1 域级服务模块

新建或修改 `services/finance/0module.go`：

```go
package finance

import (
    "github.com/KiClover/orzAdmin/services/finance/bill"
    "go.uber.org/fx"
)

func Module() fx.Option {
    return fx.Options(
        fx.Provide(bill.NewFinanceBillService),
    )
}
```

漏掉这个 provider 时，`RunService(financebillservice.NewServer)` 无法从 Fx 获取 handler 接口，服务不会启动。

### 5.2 根服务模块

如果 `finance` 是第一次出现，修改 [`services/0module.go`](../services/0module.go)：

```go
import (
    "github.com/KiClover/orzAdmin/services/finance"
    "github.com/KiClover/orzAdmin/services/system"
    "go.uber.org/fx"
)

func Module() fx.Option {
    return fx.Options(
        system.Module(),
        finance.Module(),
    )
}
```

如果只是向已有域增加服务，只更新该域的 `0module.go`，不再修改根模块。

## 6. 创建启动入口

新建 `services/finance/bill/cmd/main.go`：

```go
package main

import (
    "context"

    "github.com/KiClover/orzAdmin/bootstrap"
    "github.com/KiClover/orzAdmin/internal/constructor"
    "github.com/KiClover/orzAdmin/kitex_gen/finance/bill/financebillservice"
    "github.com/KiClover/orzAdmin/services/finance/bill"
    "go.opentelemetry.io/otel"
    "go.uber.org/fx"
)

var tracer = otel.Tracer("github.com/KiClover/orzAdmin")

func main() {
    ctx, span := tracer.Start(context.Background(), "main")
    defer span.End()

    app := bootstrap.NewApp(
        ctx,
        bill.ServiceName,
        fx.Invoke(constructor.RunService(financebillservice.NewServer)),
    )

    app.Run()
}
```

注意：

- `RunService` 接收生成 service 子包中的 `NewServer`，不是 handler constructor。
- 必须使用 `bill.ServiceName` 常量，不要复制字符串。
- 该名称会写入注册中心，RPC Client 必须使用同一个值发现服务。

## 7. 添加运行配置

新业务域建议使用带域目录的配置，避免同名冲突：

```text
etc/finance/bill.yaml
```

最小示例：

```yaml
SystemConf:
  Network: tcp
  Addr: 127.0.0.1
  Port: 8090

DiscoveryConf:
  Method: etcd

EtcdConf:
  Endpoints: ["127.0.0.1:2379"]
  Username:
  Password:
```

每个同时运行的服务必须使用不同端口。`constructor.RunService` 根据 `DiscoveryConf.Method` 选择 etcd 或 Nacos；缺失或不支持的方法会导致启动失败。

启动：

```bash
go run services/finance/bill/cmd/main.go \
  -env=false \
  -f=etc/finance/bill.yaml
```

按需在 `Makefile` 增加：

```make
.PHONY: run-finance-bill

run-finance-bill:
	$(GO) run services/finance/bill/cmd/main.go -env=false -f=etc/finance/bill.yaml
```

需要一键启动时再修改 `run-all.ps1`。这些属于运维入口，不是 Fx 编译依赖。

## 8. 其他进程需要调用新服务时

只有存在真实远程调用方，才需要 Client provider。仅新增服务端时不必机械添加 Client。

### 8.1 Client constructor

新建或修改 `internal/client/finance/client.go`：

```go
package finance

import (
    "fmt"
    "time"

    "github.com/KiClover/orzAdmin/kitex_gen/finance/bill/financebillservice"
    "github.com/KiClover/orzAdmin/services/finance/bill"
    "github.com/cloudwego/kitex/client"
    "github.com/cloudwego/kitex/pkg/discovery"
)

const (
    DefaultRPCTimeout     = 5 * time.Second
    DefaultConnectTimeout = 5 * time.Second
)

func NewBillServiceClient(
    r *discovery.Resolver,
) (financebillservice.Client, error) {
    c, err := financebillservice.NewClient(
        bill.ServiceName,
        client.WithResolver(*r),
        client.WithRPCTimeout(DefaultRPCTimeout),
        client.WithConnectTimeout(DefaultConnectTimeout),
    )
    if err != nil {
        return nil, fmt.Errorf("create %s client: %w", bill.ServiceName, err)
    }
    return c, nil
}
```

推荐返回 `(Client, error)`，让 Fx 在 Client 创建失败时阻止进程启动；不要只记录错误后继续返回无效 Client。

### 8.2 域级 Client Module

新建或修改 `internal/client/finance/0module.go`：

```go
package finance

import "go.uber.org/fx"

func Module() fx.Option {
    return fx.Options(
        fx.Provide(NewBillServiceClient),
    )
}
```

现有 [`internal/client/system/0module.go`](../internal/client/system/0module.go) 中每一行 `fx.Provide(NewXxxServiceClient)` 都是在提供一种生成的 Kitex Client 接口。它不是注册服务端实现，也不会自动让 handler 使用该 Client。

### 8.3 根 Client Module

第一次增加 `finance` Client 模块时，修改 [`internal/client/0module.go`](../internal/client/0module.go)：

```go
import (
    "github.com/KiClover/orzAdmin/internal/client/finance"
    "github.com/KiClover/orzAdmin/internal/client/system"
    "go.uber.org/fx"
)

func Module() fx.Option {
    return fx.Module(
        "internal.client",
        system.Module(),
        finance.Module(),
    )
}
```

### 8.4 只向真实调用方注入

```go
type SomeService struct {
    fx.In

    BillClient financebillservice.Client
}
```

如果调用方使用独立参数结构，constructor 要显式复制字段：

```go
type SomeServiceParam struct {
    fx.In
    BillClient financebillservice.Client
}

type SomeService struct {
    BillClient financebillservice.Client
}

func NewSomeService(p SomeServiceParam) *SomeService {
    return &SomeService{BillClient: p.BillClient}
}
```

没有调用 bill 的 handler 不应增加字段。Fx provider 是惰性的，只有依赖链需要 `financebillservice.Client` 时才会创建连接。

调用后立即处理错误：

```go
resp, err := s.BillClient.GetBill(ctx, req)
if err != nil {
    return nil, fmt.Errorf("call %s GetBill: %w", bill.ServiceName, err)
}
```

## 9. 服务需要数据库时

数据库不是所有 Kitex 服务的必选项。只有实际使用 Ent 时才增加。

### 9.1 Ent Schema

新增或修改：

```text
ent/schema/bill.go
```

然后运行：

```bash
make ent
```

### 9.2 DB wrapper 和 provider

新建 `db/bill.go`：

```go
package db

import "github.com/KiClover/orzAdmin/ent"

type BillServiceDB struct {
    Bill *ent.BillClient
}

func NewFinanceBillDB(client *ent.Client) *BillServiceDB {
    return &BillServiceDB{Bill: client.Bill}
}
```

在 [`db/0module.go`](../db/0module.go) 注册：

```go
fx.Provide(NewFinanceBillDB),
```

handler 按需注入：

```go
type FinanceBillService struct {
    fx.In
    DB *db.BillServiceDB
}
```

只有需要完整 Ent Client、事务或 Schema 迁移时才注入：

```go
DBAll *ent.Client
```

`InitDatabase` 不是启动框架自动行为。只有 Proto 定义该 RPC 且业务需要时才实现；服务启动不会自动调用它。

## 10. 框架已经提供的依赖

新服务只需声明实际消费的类型，不需要再次 `fx.Provide`：

| 依赖 | 提供位置 | 使用场景 |
| --- | --- | --- |
| `*conf.Config` | `bootstrap/appentry.go` | 业务直接读取配置时 |
| `registry.Registry` | `internal/discovery` | `RunService` 自动使用 |
| `*discovery.Resolver` | `internal/discovery` | RPC Client constructor |
| `*ent.Client` | `instance.Module()` | 数据库和事务 |
| `redis.UniversalClient` | `instance.Module()` | Redis 业务 |
| `*casbin.Enforcer` | `instance.Module()` | 权限业务 |
| `context.Context` | `bootstrap.NewApp()` | 生命周期和 provider |
| `fx.Lifecycle` | Fx | `RunService` 自动使用 |

“provider 已注册”不代表每个进程都会实例化它。Fx 只构造当前 Invoke 依赖链需要的 provider。

## 11. 修改位置判定表

| 位置 | 新服务端 | 有远程调用方 | 使用数据库 | 新业务域 |
| --- | :---: | :---: | :---: | :---: |
| `idl/<scope>/<name>.proto` | 必须 | — | — | — |
| `services/<scope>/<name>/handler.go` | 必须 | — | 按需注入 DB | — |
| `services/<scope>/<name>/cmd/main.go` | 必须 | — | — | — |
| `services/<scope>/0module.go` | 必须 | — | — | 新建 |
| `services/0module.go` | — | — | — | 必须 |
| `internal/client/<scope>/client.go` | — | 必须 | — | 按需新建 |
| `internal/client/<scope>/0module.go` | — | 必须 | — | 按需新建 |
| `internal/client/0module.go` | — | — | — | 有 Client 时必须 |
| 调用方 `fx.In` | — | 必须 | — | — |
| `db/<name>.go` | — | — | 必须 | — |
| `db/0module.go` | — | — | 必须 | — |
| `etc/...yaml` | 运行时必须 | — | 增加 DB 配置 | — |
| `Makefile` / `run-all.ps1` | 按需 | — | — | — |

## 12. 当前项目中不要复制的问题

### 12.1 constructor 名、Client 类型与 ServiceName 必须同属一个服务

历史上 `internal/client/system/client.go` 的 Token constructor 曾被命名为 `NewClientServiceClient`，且创建 `systemtokenservice.Client` 时误用了 `access.ServiceName`（现已修复为 `NewTokenServiceClient` + `token.ServiceName`）。新增 Client 时必须保证下面三者属于同一服务：

```text
NewBillServiceClient
financebillservice.Client
bill.ServiceName
```

### 12.2 Client 创建错误必须返回 error

system Client constructors 现已统一返回 `(Client, error)`，`NewClient` 失败时 Fx 会阻止进程启动。新代码保持这一约定，不要退回"只记录日志后返回无效 Client"的写法。

### 12.3 不要复制未使用依赖

现有部分 handler 的 `fx.In` 包含未使用的 Client、`Cfg` 或 Casbin。即使字段未在方法中使用，Fx 仍会将其作为启动硬依赖。新服务应从空依赖开始，只加入真正使用的类型。

### 12.4 服务端名与客户端目标名必须一致

```go
// server
bootstrap.NewApp(ctx, bill.ServiceName, ...)

// client
financebillservice.NewClient(bill.ServiceName, ...)
```

两端引用同一个常量，不要分别手写 `"finance-bill"`。

### 12.5 新域有两棵模块树

有服务端实现时注册：

```text
services/finance/0module.go
services/0module.go
```

有 RPC Client 时另外注册：

```text
internal/client/finance/0module.go
internal/client/0module.go
```

两棵树用途不同，不能混淆。

## 13. 完成检查清单

### IDL 和实现

- [ ] Proto 的 `package`、`go_package`、scope 和目录一致。
- [ ] Kitex 生成代码存在，没有误用陈旧 Service 目录。
- [ ] handler 实现全部 RPC，并保留编译期接口断言。
- [ ] `ServiceName` 唯一且符合 `<scope>-<name>`。

### Fx 注册

- [ ] handler constructor 已加入 `services/<scope>/0module.go`。
- [ ] 新 scope 已加入 `services/0module.go`。
- [ ] 只有存在远程调用方时才添加 Client provider。
- [ ] 新 Client scope 已加入 `internal/client/0module.go`。
- [ ] 调用方只注入真实使用的 Client。
- [ ] DB wrapper 已加入 `db/0module.go`。

### 运行与发现

- [ ] server 和 Client 引用同一个 `ServiceName`。
- [ ] 配置包含 `DiscoveryConf.Method` 及对应 etcd/Nacos 参数。
- [ ] 监听端口不与其他进程冲突。
- [ ] Client 创建失败能阻止 Fx 启动。

### 验证命令

```bash
gofmt -w <修改的.go文件>

go test ./services/finance/bill/... ./internal/client/finance/...
go test ./services/... ./internal/client/... ./db/...
go test ./...
```

最后使用真实配置启动，确认：

1. Fx 没有 `missing dependencies`。
2. 服务成功监听指定端口。
3. 服务以 `finance-bill` 注册到 etcd/Nacos。
4. 远程调用方能通过同一名称发现并调用。
