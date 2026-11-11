package initialize

import "errors"

// ErrUnknownService 表示请求初始化一个未注册的服务名。
var ErrUnknownService = errors.New("initialize: unknown service")

// ErrMissingDependency 表示目标服务的前置表或前置数据尚未初始化。
var ErrMissingDependency = errors.New("initialize: missing dependency")

// Stage 表示初始化执行到的阶段。
type Stage string

const (
	// StageSchema 是 Ent 表结构同步阶段。
	StageSchema Stage = "schema"
	// StageSeed 是缺失种子补齐阶段。
	StageSeed Stage = "seed"
)

// Status 表示单个服务在某阶段的执行结果。
type Status string

const (
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

// ServiceResult 记录一个服务在一个阶段的结果，供 HTTP 层映射为稳定响应。
type ServiceResult struct {
	Service string
	Stage   Stage
	Status  Status
	Message string
}

// ServiceInfo 描述注册表中的服务及其依赖。
type ServiceInfo struct {
	Name      string
	DependsOn []string
}

// 服务名常量。HTTP 层只接受这些固定值。
const (
	ServiceUser    = "system-user"
	ServiceAccess  = "system-access"
	ServiceSetting = "system-setting"
	ServiceMessage = "system-message"
	ServiceToken   = "system-token"
)

// services 是注册表的唯一来源，顺序即全部初始化的依赖顺序：
// 用户表先建（access 的角色关联需要管理员用户），其余服务互不依赖。
var services = []ServiceInfo{
	{Name: ServiceUser},
	{Name: ServiceAccess, DependsOn: []string{ServiceUser}},
	{Name: ServiceSetting},
	{Name: ServiceMessage},
	{Name: ServiceToken},
}

// Services 返回注册表副本，避免调用方修改内部顺序。
func Services() []ServiceInfo {
	out := make([]ServiceInfo, len(services))
	copy(out, services)
	for i := range out {
		out[i].DependsOn = append([]string(nil), services[i].DependsOn...)
	}
	return out
}

// ServiceNames 按依赖顺序返回全部服务名。
func ServiceNames() []string {
	out := make([]string, 0, len(services))
	for _, s := range services {
		out = append(out, s.Name)
	}
	return out
}

// LookupService 返回服务定义；未知服务返回 ErrUnknownService。
func LookupService(name string) (ServiceInfo, error) {
	for _, s := range services {
		if s.Name == name {
			s.DependsOn = append([]string(nil), s.DependsOn...)
			return s, nil
		}
	}
	return ServiceInfo{}, ErrUnknownService
}
