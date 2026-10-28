# Makefile 使用指南

项目根目录的 [Makefile](../Makefile) 将代码生成和服务运行分开，每条命令只负责一项任务。所有命令都需要在项目根目录执行。

## 1. 环境准备

Windows 下建议在 Git Bash 中使用：

```bash
cd /d/project/orzAdmin
make help
```

需要提前安装 GNU Make 和 Go，并保证它们在当前终端的 `PATH` 中。Git Bash 本身不一定附带 `make`。

按使用的功能准备对应工具：

| 功能 | 所需工具 |
| --- | --- |
| 查看帮助 | `make`、`sh`、`printf`，Git Bash 提供后两项 |
| Ent 生成、运行服务 | `go`，项目要求 Go 1.24.4 或更高版本 |
| Kitex 生成 | `kitex` 及其生成流程所需依赖 |
| Hertz 模型生成 | `hz` 及其生成流程所需依赖 |

首次生成或运行时，Go 可能需要下载模块依赖。

## 2. 查看帮助

```bash
make
# 或
make help
```

不指定命令时默认显示帮助，不会生成代码或启动服务。

## 3. Kitex：生成指定 Proto 的代码

格式：

```bash
make kx <文件名.proto>
```

例如：

```bash
make kx user.proto
```

实际执行：

```bash
kitex idl/system/user.proto
```

替换文件名即可生成其他 IDL：

```bash
make kx access.proto
make kx message.proto
make kx setting.proto
make kx token.proto
```

文件名相对于 `idl/system/`，不需要填写该目录前缀。每次调用指定一个 `.proto` 文件；生成多个文件时分别执行命令。

## 4. Hertz：生成指定 Thrift 的模型

格式：

```bash
make hz <文件名.thrift>
```

例如：

```bash
make hz user_api.thrift
```

实际执行：

```bash
hz model -idl=idl/system/user_api.thrift -model_dir=api/model
```

其他模型可以分别生成：

```bash
make hz auth_api.thrift
make hz permission_api.thrift
make hz setting_api.thrift
```

输入文件位于 `idl/system/`，模型输出目录为 `api/model/`。这条命令只执行 `hz model`。

## 5. Ent：选择生成方式

### 直接根据 Schema 生成

```bash
make ent
# 兼容原来的命令名
make gen-ent
```

两者都执行原来的 Ent 生成命令：

```bash
go run -mod=mod entgo.io/ent/cmd/ent generate ./ent/schema
```

### 使用项目的 go:generate 指令

```bash
make generate-ent
```

实际执行：

```bash
go generate ./ent
```

当前 [ent/generate.go](../ent/generate.go) 中的指令包含 `--template ./template`，因此该入口会使用项目的自定义模板。需要模板生成内容时，使用 `make generate-ent`。

这两种方式保留了原脚本中的两条独立命令，按需要选择执行。

## 6. 单独运行服务

每个 `run-*` 命令在当前终端前台运行一个服务。需要同时运行多个服务时，打开多个终端，在每个终端中先进入项目根目录，再执行对应命令；按 `Ctrl+C` 停止当前服务。

### Kitex RPC 服务

| 命令 | Go 入口 | 配置文件 |
| --- | --- | --- |
| `make run-user` | [user/cmd/main.go](../services/system/user/cmd/main.go) | [etc/user.yaml](../etc/user.yaml) |
| `make run-setting` | [setting/cmd/main.go](../services/system/setting/cmd/main.go) | [etc/setting.yaml](../etc/setting.yaml) |
| `make run-message` | [message/cmd/main.go](../services/system/message/cmd/main.go) | [etc/msg.yaml](../etc/msg.yaml) |
| `make run-access` | [access/cmd/main.go](../services/system/access/cmd/main.go) | [etc/access.yaml](../etc/access.yaml) |

例如，`make run-user` 实际执行：

```bash
go run services/system/user/cmd/main.go -env=false -f=etc/user.yaml
```

### Hertz HTTP API 服务

| 命令 | Go 入口 | 配置文件 |
| --- | --- | --- |
| `make run-api-auth` | [auth/cmd/main.go](../api/system/auth/cmd/main.go) | [etc/api/auth.yaml](../etc/api/auth.yaml) |
| `make run-api-user` | [user/cmd/main.go](../api/system/user/cmd/main.go) | [etc/api/user.yaml](../etc/api/user.yaml) |
| `make run-api-permission` | [permission/cmd/main.go](../api/system/permission/cmd/main.go) | [etc/api/permission.yaml](../etc/api/permission.yaml) |

例如，`make run-api-auth` 实际执行：

```bash
go run api/system/auth/cmd/main.go -env=false -f=etc/api/auth.yaml
```

所有运行命令都保留 `-env=false` 和各自的 `-f` 参数，读取对应 YAML 配置。运行前请按 [README 的本地启动说明](../README.md#安装与本地启动) 准备数据库、Redis、服务发现等依赖。

## 7. 指定工具路径

Makefile 支持覆盖以下变量：

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `GO` | `go` | Ent 生成和服务运行 |
| `KITEX` | `kitex` | Kitex 代码生成 |
| `HZ` | `hz` | Hertz 模型生成 |

例如，在 Git Bash 中指定工具位置：

```bash
make kx user.proto KITEX=/d/gopath/bin/kitex
make hz user_api.thrift HZ=/d/gopath/bin/hz
make ent GO=/e/gvm/g/go/bin/go
```

路径包含空格时，需要让路径的双引号传递到执行命令的 shell，例如：

```bash
make ent GO='"/c/Program Files/Go/bin/go.exe"'
```

## 8. 参数规则与常见问题

### 没有提供文件名

```bash
make kx
make hz
```

这两种调用都会报用法错误。请补充文件名：

```bash
make kx user.proto
make hz user_api.thrift
```

### 文件不存在或扩展名错误

Makefile 会检查文件是否位于 `idl/system/`，并检查扩展名：`kx` 接收 `.proto`，`hz` 接收 `.thrift`。

例如，`make kx missing.proto` 会提示：

```text
IDL file not found: idl/system/missing.proto
```

### 一次传入多个文件或混合多个目标

`kx`、`hz` 使用“命令 + 文件名”的格式，文件名由 Makefile 作为参数接收；每次调用只允许这两个目标。

以下调用不支持：

```bash
make kx user.proto access.proto
make kx user.proto help
make ent kx user.proto
```

请拆成独立命令：

```bash
make ent
make kx user.proto
make kx access.proto
```

变量赋值不属于目标，因此 `make kx user.proto KITEX=/d/gopath/bin/kitex` 可以使用。

### 工具找不到

若终端提示 `go`、`kitex` 或 `hz` 找不到，请检查当前 Git Bash 的 `PATH`，或使用上一节的工具变量指定路径。若 `make` 本身找不到，需要先安装 GNU Make 并加入 `PATH`。

## 9. 与原命令的对应关系

| 原命令或操作 | 当前用法 |
| --- | --- |
| `make gen-ent` | 继续可用，也可以使用 `make ent` |
| `gen-kx` 中的 `go generate ./ent` | `make generate-ent` |
| `gen-kx` 中的 `hz model ...` | `make hz <文件名.thrift>` |
| `gen-kx` 中的 `kitex ...` | `make kx <文件名.proto>` |
| `make run-kx` 中的多条启动命令 | 分别使用对应的 `make run-*` 命令 |

`gen-kx` 和 `run-kx` 已拆分，不再作为聚合目标提供。生成代码、运行 RPC 服务和运行 HTTP API 服务分别选择对应命令。
