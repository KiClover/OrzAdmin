// Package initialize 提供独立的初始化 API：只做表结构同步与种子补齐，
// 不加载服务发现、业务服务或业务客户端。
package initialize

import (
	"context"
	"errors"
	"net/http"

	apimodel "github.com/KiClover/orzAdmin/api/model/initialize"
	"github.com/KiClover/orzAdmin/conf"
	orzinit "github.com/KiClover/orzAdmin/internal/initialize"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"go.uber.org/fx"
)

// ServiceName 是初始化 API 在配置文件里的服务名。
const ServiceName = "api.initialize"

// 稳定的错误码，供部署脚本与运维按码分支，不随文案变化。
const (
	codeUnknownService       = "unknown_service"
	codeMissingDependency    = "missing_dependency"
	codeInitializationFailed = "initialization_failed"
)

// Initializer 是初始化 API 依赖的最小能力接口，便于 handler 单测注入假实现。
type Initializer interface {
	InitializeAll(ctx context.Context) ([]orzinit.ServiceResult, error)
	InitializeService(ctx context.Context, service string) (orzinit.ServiceResult, error)
}

// HzInitializeServiceP 是 Fx 的入参集合。
type HzInitializeServiceP struct {
	fx.In

	Cfg         *conf.Config
	Initializer Initializer
}

// HzInitializeService 是初始化 API 的 handler。
type HzInitializeService struct {
	Cfg         *conf.Config
	Initializer Initializer
}

// NewHzInitializeService 由 Fx 调用构造 handler。
func NewHzInitializeService(p HzInitializeServiceP) *HzInitializeService {
	return &HzInitializeService{
		Cfg:         p.Cfg,
		Initializer: p.Initializer,
	}
}

// initErrorResp 是失败响应体：稳定错误码 + 安全文案 + 已完成阶段结果。
type initErrorResp struct {
	Code    string                    `json:"code"`
	Message string                    `json:"message"`
	Results []*apimodel.ServiceResult `json:"results,omitempty"`
	Result  *apimodel.ServiceResult   `json:"result,omitempty"`
}

// InitAll 初始化全部服务（POST /init/services）。
func (s *HzInitializeService) InitAll(ctx context.Context, c *app.RequestContext) {
	results, err := s.Initializer.InitializeAll(ctx)
	if err != nil {
		status, code := mapInitError(err)
		c.JSON(status, &initErrorResp{
			Code:    code,
			Message: errorMessage(code),
			Results: toServiceResults(results),
		})
		return
	}
	c.JSON(http.StatusOK, &apimodel.InitAllResp{Results: toServiceResults(results)})
}

// InitService 只初始化指定服务（POST /init/services/:service）。
func (s *HzInitializeService) InitService(ctx context.Context, c *app.RequestContext) {
	service := c.Param("service")
	result, err := s.Initializer.InitializeService(ctx, service)
	if err != nil {
		status, code := mapInitError(err)
		c.JSON(status, &initErrorResp{
			Code:    code,
			Message: errorMessage(code),
			Result:  toServiceResult(result),
		})
		return
	}
	c.JSON(http.StatusOK, &apimodel.InitServiceResp{Result: toServiceResult(result)})
}

// mapInitError 把内部错误映射成稳定的 HTTP 状态码与错误码。
func mapInitError(err error) (int, string) {
	switch {
	case errors.Is(err, orzinit.ErrUnknownService):
		return http.StatusNotFound, codeUnknownService
	case errors.Is(err, orzinit.ErrMissingDependency):
		return http.StatusConflict, codeMissingDependency
	default:
		return http.StatusInternalServerError, codeInitializationFailed
	}
}

// errorMessage 返回固定安全文案，不透出 DSN、SQL 或驱动错误。
func errorMessage(code string) string {
	switch code {
	case codeUnknownService:
		return "unknown service"
	case codeMissingDependency:
		return "missing dependency: initialize the required service first"
	default:
		return "initialization failed"
	}
}

// toServiceResults 批量转换内部结果。
func toServiceResults(results []orzinit.ServiceResult) []*apimodel.ServiceResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]*apimodel.ServiceResult, 0, len(results))
	for _, result := range results {
		out = append(out, toServiceResult(result))
	}
	return out
}

// toServiceResult 转换单条结果；空结果返回 nil，避免出现空对象。
func toServiceResult(result orzinit.ServiceResult) *apimodel.ServiceResult {
	if result.Service == "" && result.Stage == "" && result.Status == "" {
		return nil
	}
	service := result.Service
	stage := string(result.Stage)
	status := string(result.Status)
	message := result.Message
	return &apimodel.ServiceResult{
		Service: &service,
		Stage:   &stage,
		Status:  &status,
		Message: &message,
	}
}

// ProvideInitializer 把具体初始化器暴露为 handler 依赖的接口。
func ProvideInitializer(initializer *orzinit.Initializer) Initializer {
	return initializer
}

// NewService 构造一个不带服务发现的 Hertz 实例（初始化 API 不注册到注册中心）。
func NewService(cfg *conf.Config) *server.Hertz {
	return newHertzServer(cfg)
}

// CloseInitializer 让初始化器在进程退出前释放数据库连接。
func CloseInitializer(lc fx.Lifecycle, initializer *orzinit.Initializer) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return initializer.Close()
		},
	})
}
