package initialize

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apimodel "github.com/KiClover/orzAdmin/api/model/initialize"
	"github.com/KiClover/orzAdmin/conf"
	orzinit "github.com/KiClover/orzAdmin/internal/initialize"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

// fakeInitializer 记录调用参数并返回预设结果，便于验证 handler 的映射逻辑。
type fakeInitializer struct {
	allResults    []orzinit.ServiceResult
	allErr        error
	oneResult     orzinit.ServiceResult
	oneErr        error
	requestedName string
}

func (f *fakeInitializer) InitializeAll(context.Context) ([]orzinit.ServiceResult, error) {
	return f.allResults, f.allErr
}

func (f *fakeInitializer) InitializeService(_ context.Context, service string) (orzinit.ServiceResult, error) {
	f.requestedName = service
	return f.oneResult, f.oneErr
}

// newTestServer 构造一个不监听端口的 Hertz 实例，路由与生产保持一致。
func newTestServer(t *testing.T, initializer Initializer) *server.Hertz {
	t.Helper()
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	NewRegisterRouter(h, &HzInitializeService{Cfg: &conf.Config{}, Initializer: initializer})
	return h
}

// successResult 构造一条成功结果。
func successResult(service string, stage orzinit.Stage) orzinit.ServiceResult {
	return orzinit.ServiceResult{Service: service, Stage: stage, Status: orzinit.StatusSuccess}
}

func TestInitAllSuccess(t *testing.T) {
	fake := &fakeInitializer{allResults: []orzinit.ServiceResult{
		successResult(orzinit.ServiceUser, orzinit.StageSchema),
		successResult(orzinit.ServiceUser, orzinit.StageSeed),
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
	if first.GetService() != orzinit.ServiceUser {
		t.Fatalf("service = %q", first.GetService())
	}
	if first.GetStage() != string(orzinit.StageSchema) {
		t.Fatalf("stage = %q", first.GetStage())
	}
	if first.GetStatus() != string(orzinit.StatusSuccess) {
		t.Fatalf("status = %q", first.GetStatus())
	}
}

func TestInitAllFailureKeepsPartialResults(t *testing.T) {
	fake := &fakeInitializer{
		allResults: []orzinit.ServiceResult{successResult(orzinit.ServiceUser, orzinit.StageSchema)},
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
	fake := &fakeInitializer{oneErr: orzinit.ErrUnknownService}
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
	fake := &fakeInitializer{oneErr: orzinit.ErrMissingDependency}
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

func TestInitServicePassesPathParam(t *testing.T) {
	fake := &fakeInitializer{oneResult: successResult(orzinit.ServiceSetting, orzinit.StageSeed)}
	h := newTestServer(t, fake)

	w := ut.PerformRequest(h.Engine, "POST", "/init/services/system-setting", nil)
	if got := w.Code; got != 200 {
		t.Fatalf("状态码 = %d，期望 200", got)
	}
	if fake.requestedName != orzinit.ServiceSetting {
		t.Fatalf("传给初始化器的服务名 = %q，期望 %q", fake.requestedName, orzinit.ServiceSetting)
	}
	var resp apimodel.InitServiceResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Result.GetService() != orzinit.ServiceSetting {
		t.Fatalf("result.service = %q", resp.Result.GetService())
	}
}
