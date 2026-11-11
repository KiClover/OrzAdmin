package initialize

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/KiClover/orzAdmin/kitex_gen/base"
	accesssvc "github.com/KiClover/orzAdmin/kitex_gen/system/access/systemaccessservice"
	messagesvc "github.com/KiClover/orzAdmin/kitex_gen/system/message/systemmessageservice"
	settingsvc "github.com/KiClover/orzAdmin/kitex_gen/system/setting/systemsettingservice"
	tokensvc "github.com/KiClover/orzAdmin/kitex_gen/system/token/systemtokenservice"
	userrpc "github.com/KiClover/orzAdmin/kitex_gen/system/user"
	usersvc "github.com/KiClover/orzAdmin/kitex_gen/system/user/systemuserservice"
	"github.com/casbin/casbin/v2"
)

// rpcInitializer 编排各服务初始化，并通过统一注入的 Casbin 实例完成首次授权。
// 各服务客户端与其它 API 一样由 Fx 注入（服务发现解析实例）。
type rpcInitializer struct {
	user    usersvc.Client
	access  accesssvc.Client
	setting settingsvc.Client
	message messagesvc.Client
	token   tokensvc.Client
	casbin  *casbin.Enforcer
}

// NewRPCInitializer 把各服务的 Kitex 客户端包装成 handler 依赖的 Initializer。
func NewRPCInitializer(
	user usersvc.Client,
	access accesssvc.Client,
	setting settingsvc.Client,
	message messagesvc.Client,
	token tokensvc.Client,
	enforcer *casbin.Enforcer,
) Initializer {
	return &rpcInitializer{
		user:    user,
		access:  access,
		setting: setting,
		message: message,
		token:   token,
		casbin:  enforcer,
	}
}

// InitializeAll 按依赖顺序串行调用各服务的初始化 RPC；失败即停止并标记后续服务。
func (r *rpcInitializer) InitializeAll(ctx context.Context, isFirst bool) ([]ServiceResult, error) {
	names := ServiceNames()
	results := make([]ServiceResult, 0, len(names))
	for idx, name := range names {
		result, err := r.InitializeService(ctx, name, isFirst)
		results = append(results, result)
		if err != nil {
			for _, rest := range names[idx+1:] {
				results = append(results, ServiceResult{
					Service: rest,
					Stage:   StageRPC,
					Status:  StatusSkipped,
					Message: "skipped after failure",
				})
			}
			return results, err
		}
	}
	return results, nil
}

// InitializeService 只调用指定服务的初始化 RPC，不会顺带初始化其它服务。
func (r *rpcInitializer) InitializeService(ctx context.Context, service string, isFirst bool) (ServiceResult, error) {
	if _, err := LookupService(service); err != nil {
		return ServiceResult{
			Service: service,
			Stage:   StageRPC,
			Status:  StatusFailed,
			Message: "unknown service",
		}, err
	}
	result := ServiceResult{Service: service, Stage: StageRPC, Status: StatusSuccess}
	if err := r.callService(ctx, service, isFirst); err != nil {
		result.Status = StatusFailed
		result.Message = safeRPCMessage(err)
		return result, err
	}
	return result, nil
}

// callService 分派到具体服务的初始化 RPC。
//
// access RPC 只建表、补种子；is_first=true 时由 InitAPI 读取用户和 API 列表，
// 再通过注入的 Casbin 实例写默认策略并绑定管理员。
func (r *rpcInitializer) callService(ctx context.Context, service string, isFirst bool) error {
	first := isFirst
	switch service {
	case ServiceUser:
		_, err := r.user.InitDatabase(ctx, &userrpc.InitDatabaseReq{IsFirst: &first})
		return err
	case ServiceAccess:
		if _, err := r.access.InitDatabase(ctx, &base.Empty{}); err != nil {
			return err
		}
		if isFirst {
			return r.grantBootstrapAdmin(ctx)
		}
		return nil
	case ServiceSetting:
		_, err := r.setting.InitDatabase(ctx, &base.Empty{})
		return err
	case ServiceMessage:
		_, err := r.message.InitDatabase(ctx, &base.Empty{})
		return err
	case ServiceToken:
		_, err := r.token.InitDatabase(ctx, &base.Empty{})
		return err
	default:
		return fmt.Errorf("%w: %s", ErrUnknownService, service)
	}
}

// grantBootstrapAdmin 沿用原 Permission API 的授权方式，复用带 watcher 的 Casbin 实例。
func (r *rpcInitializer) grantBootstrapAdmin(ctx context.Context) error {
	admin, err := r.user.GetBootstrapAdmin(ctx, &base.Empty{})
	if err != nil {
		return err
	}
	if !admin.GetExists() || admin.GetId() <= 0 {
		return fmt.Errorf("%w: default administrator is not available", ErrMissingDependency)
	}
	apis, err := r.access.GetApiList(ctx, &base.Empty{})
	if err != nil {
		return err
	}
	for _, api := range apis.List {
		if _, err := r.casbin.AddPolicy("admin", api.GetPath(), api.GetMethod()); err != nil {
			return err
		}
	}
	_, err = r.casbin.AddRoleForUser(strconv.FormatInt(admin.GetId(), 10), "admin")
	return err
}

// safeRPCMessage 返回可安全写入 HTTP 响应的文案，不透出地址、SQL 或驱动细节。
func safeRPCMessage(err error) string {
	if errors.Is(err, ErrMissingDependency) {
		return "missing dependency: initialize the required service first"
	}
	return "initialization failed"
}
