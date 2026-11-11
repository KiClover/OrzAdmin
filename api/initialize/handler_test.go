package initialize

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apimodel "github.com/KiClover/orzAdmin/api/model/initialize"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

// fakeInitializer 记录调用参数并返回预设结果，便于验证 handler 的映射逻辑。
type fakeInitializer struct {
	allResults    []ServiceResult
	allErr        error
	oneResult     ServiceResult
	oneErr        error
	requestedName string
	gotIsFirst    bool
	allCalls      int
}

func (f *fakeInitializer) InitializeAll(_ context.Context, isFirst bool) ([]ServiceResult, error) {
	f.allCalls++
	f.gotIsFirst = isFirst
	return f.allResults, f.allErr
}

func (f *fakeInitializer) InitializeService(_ context.Context, service string, isFirst bool) (ServiceResult, error) {
	f.requestedName = service
	f.gotIsFirst = isFirst
	return f.oneResult, f.oneErr
}

// newTestServer 构造一个不监听端口的 Hertz 实例，路由与生产保持一致。
func newTestServer(t *testing.T, initializer Initializer) *server.Hertz {
	t.Helper()
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	NewRegisterRouter(h, &HzInitializeService{Cfg: &conf.Config{}, Initializer: initializer})
	return h
}

// jsonBody 构造带 JSON Content-Type 的请求体。
func jsonBody(body string) (*ut.Body, ut.Header) {
	return &ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"}
}

// successResult 构造一条成功结果。
func successResult(service string) ServiceResult {
	return ServiceResult{Service: service, Stage: StageRPC, Status: StatusSuccess}
}

func TestInitAllSuccess(t *testing.T) {
	fake := &fakeInitializer{allResults: []ServiceResult{
		successResult(ServiceUser),
		successResult(ServiceAccess),
	}}
	h := newTestServer(t, fake)

	w := ut.PerformRequest(h.Engine, "POST", "/init/services", nil)
	if got := w.Code; got != 200 {
		t.Fatalf("状态码 = %d，期望 200", got)
	}
	var resp apimodel.InitAllResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("结果条数 = %d，期望 2", len(resp.Results))
	}
	first := resp.Results[0]
	if first.GetService() != ServiceUser {
		t.Fatalf("service = %q", first.GetService())
	}
	if first.GetStage() != StageRPC {
		t.Fatalf("stage = %q", first.GetStage())
	}
	if first.GetStatus() != StatusSuccess {
		t.Fatalf("status = %q", first.GetStatus())
	}
	if fake.gotIsFirst {
		t.Fatal("未传 is_first 时应按 false 处理")
	}
}

func TestInitAllPassesIsFirst(t *testing.T) {
	fake := &fakeInitializer{allResults: []ServiceResult{successResult(ServiceUser)}}
	h := newTestServer(t, fake)

	body, header := jsonBody(`{"is_first":true}`)
	w := ut.PerformRequest(h.Engine, "POST", "/init/services", body, header)
	if got := w.Code; got != 200 {
		t.Fatalf("状态码 = %d，期望 200", got)
	}
	if !fake.gotIsFirst {
		t.Fatal("is_first=true 应透传给初始化器")
	}

	// 第二次调用通常带 is_first=false，只做建表与种子补齐。
	body, header = jsonBody(`{"is_first":false}`)
	w = ut.PerformRequest(h.Engine, "POST", "/init/services", body, header)
	if got := w.Code; got != 200 {
		t.Fatalf("状态码 = %d，期望 200", got)
	}
	if fake.gotIsFirst {
		t.Fatal("is_first=false 时不应触发首次初始化逻辑")
	}
}

func TestInitAllFailureKeepsPartialResults(t *testing.T) {
	fake := &fakeInitializer{
		allResults: []ServiceResult{successResult(ServiceUser)},
		allErr:     errors.New("boom with dsn=secret"),
	}
	h := newTestServer(t, fake)

	w := ut.PerformRequest(h.Engine, "POST", "/init/services", nil)
	if got := w.Code; got != 500 {
		t.Fatalf("状态码 = %d，期望 500", got)
	}
	var resp initErrorResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Code != codeInitializationFailed {
		t.Fatalf("code = %q，期望 %q", resp.Code, codeInitializationFailed)
	}
	if resp.Message != "initialization failed" {
		t.Fatalf("message = %q，应使用固定安全文案", resp.Message)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("失败时也应返回已完成结果，实际 %d 条", len(resp.Results))
	}
	if body := w.Body.String(); strings.Contains(body, "dsn=secret") {
		t.Fatalf("响应不应包含底层错误细节: %s", body)
	}
}

func TestInitServiceUnknownReturns404(t *testing.T) {
	fake := &fakeInitializer{oneErr: ErrUnknownService}
	h := newTestServer(t, fake)

	w := ut.PerformRequest(h.Engine, "POST", "/init/services/unknown-thing", nil)
	if got := w.Code; got != 404 {
		t.Fatalf("状态码 = %d，期望 404", got)
	}
	var resp initErrorResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Code != codeUnknownService {
		t.Fatalf("code = %q，期望 %q", resp.Code, codeUnknownService)
	}
}

func TestInitServiceMissingDependencyReturns409(t *testing.T) {
	fake := &fakeInitializer{oneErr: ErrMissingDependency}
	h := newTestServer(t, fake)

	w := ut.PerformRequest(h.Engine, "POST", "/init/services/system-access", nil)
	if got := w.Code; got != 409 {
		t.Fatalf("状态码 = %d，期望 409", got)
	}
	var resp initErrorResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Code != codeMissingDependency {
		t.Fatalf("code = %q，期望 %q", resp.Code, codeMissingDependency)
	}
}

func TestInitServicePassesPathParamAndIsFirst(t *testing.T) {
	fake := &fakeInitializer{oneResult: successResult(ServiceSetting)}
	h := newTestServer(t, fake)

	body, header := jsonBody(`{"is_first":true}`)
	w := ut.PerformRequest(h.Engine, "POST", "/init/services/system-setting", body, header)
	if got := w.Code; got != 200 {
		t.Fatalf("状态码 = %d，期望 200", got)
	}
	if fake.requestedName != ServiceSetting {
		t.Fatalf("传给初始化器的服务名 = %q，期望 %q", fake.requestedName, ServiceSetting)
	}
	if !fake.gotIsFirst {
		t.Fatal("is_first=true 应透传给指定服务的初始化")
	}
	var resp apimodel.InitServiceResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Result.GetService() != ServiceSetting {
		t.Fatalf("result.service = %q", resp.Result.GetService())
	}
}

func TestInitServiceUsesPathServiceName(t *testing.T) {
	fake := &fakeInitializer{oneResult: successResult(ServiceSetting)}
	h := newTestServer(t, fake)

	// 请求体里的 service 字段不应覆盖路径参数，避免执行到非预期服务。
	body, header := jsonBody(`{"service":"system-user"}`)
	ut.PerformRequest(h.Engine, "POST", "/init/services/system-setting", body, header)
	if fake.requestedName != ServiceSetting {
		t.Fatalf("服务名应来自路径参数，实际 %q", fake.requestedName)
	}
}
