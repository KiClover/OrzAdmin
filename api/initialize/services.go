package initialize

import (
	"errors"
	"fmt"
)

// 初始化 API 支持的服务名。服务名同时是注册表键与配置里的端点键。
const (
	ServiceUser    = "system-user"
	ServiceAccess  = "system-access"
	ServiceSetting = "system-setting"
	ServiceMessage = "system-message"
	ServiceToken   = "system-token"
	ServiceWallet  = "commerce-wallet"
)

// 初始化结果里的阶段与状态取值。
const (
	// StageRPC 表示通过 RPC 调用服务完成该服务的初始化。
	StageRPC = "rpc"

	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// ErrUnknownService 表示请求的服务名不在固定注册表里。
var ErrUnknownService = errors.New("unknown service")

// ErrMissingDependency 表示前置条件未满足（例如 access 首次授权需要默认管理员）。
var ErrMissingDependency = errors.New("missing dependency")

// ServiceResult 是单个服务的初始化结果。
type ServiceResult struct {
	Service string
	Stage   string
	Status  string
	Message string
}

// ServiceInfo 描述服务名与依赖关系。
type ServiceInfo struct {
	Name      string
	DependsOn []string
}

// serviceOrder 是初始化的固定顺序：先建用户表与默认管理员，再建 access 并首次授权，
// 最后是独立的 setting / message / token，以及只同步自身四张表的钱包服务。
var serviceOrder = []ServiceInfo{
	{Name: ServiceUser},
	{Name: ServiceAccess, DependsOn: []string{ServiceUser}},
	{Name: ServiceSetting},
	{Name: ServiceMessage},
	{Name: ServiceToken},
	{Name: ServiceWallet},
}

// ServiceNames 返回初始化的服务顺序。
func ServiceNames() []string {
	names := make([]string, 0, len(serviceOrder))
	for _, service := range serviceOrder {
		names = append(names, service.Name)
	}
	return names
}

// LookupService 校验服务名是否在白名单内。
func LookupService(service string) (ServiceInfo, error) {
	for _, info := range serviceOrder {
		if info.Name == service {
			return info, nil
		}
	}
	return ServiceInfo{}, fmt.Errorf("%w: %s", ErrUnknownService, service)
}
