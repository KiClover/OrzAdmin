package initialize

import (
	"errors"
	"reflect"
	"testing"
)

// TestServiceRegistryOrderAndDependencies 锁定注册表的顺序与依赖：
// 顺序即全部初始化的执行顺序，不得随意调整。
func TestServiceRegistryOrderAndDependencies(t *testing.T) {
	want := []ServiceInfo{
		{Name: "system-user"},
		{Name: "system-access", DependsOn: []string{"system-user"}},
		{Name: "system-setting"},
		{Name: "system-message"},
		{Name: "system-token"},
	}
	if got := Services(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Services() = %#v，期望 %#v", got, want)
	}
}

// TestServicesReturnsCopy 确认调用方修改返回值不会污染注册表。
func TestServicesReturnsCopy(t *testing.T) {
	first := Services()
	first[0].Name = "mutated"
	first[1].DependsOn[0] = "mutated"

	second := Services()
	if second[0].Name != "system-user" {
		t.Fatalf("注册表被外部修改: %q", second[0].Name)
	}
	if second[1].DependsOn[0] != "system-user" {
		t.Fatalf("依赖切片被外部修改: %v", second[1].DependsOn)
	}
}

// TestServiceNamesMatchServices 确认两个导出函数顺序一致。
func TestServiceNamesMatchServices(t *testing.T) {
	want := []string{"system-user", "system-access", "system-setting", "system-message", "system-token"}
	if got := ServiceNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ServiceNames() = %v，期望 %v", got, want)
	}
}

// TestLookupService 覆盖已知与未知服务。
func TestLookupService(t *testing.T) {
	svc, err := LookupService("system-access")
	if err != nil {
		t.Fatalf("已知服务返回错误: %v", err)
	}
	if svc.Name != "system-access" {
		t.Fatalf("Name = %q", svc.Name)
	}
	if !reflect.DeepEqual(svc.DependsOn, []string{"system-user"}) {
		t.Fatalf("DependsOn = %v", svc.DependsOn)
	}

	for _, name := range []string{"", "system-wallet", "SYSTEM-USER", "system-user "} {
		if _, err := LookupService(name); !errors.Is(err, ErrUnknownService) {
			t.Fatalf("LookupService(%q) 应返回 ErrUnknownService，实际 %v", name, err)
		}
	}
}

// TestNoCommerceWalletServiceYet 确认任务 2 的注册表里还没有钱包服务，
// 钱包服务的注册属于任务 3；这个断言防止提前把半成品接进初始化流程。
func TestNoCommerceWalletServiceYet(t *testing.T) {
	for _, name := range ServiceNames() {
		if name == "commerce-wallet" {
			t.Fatalf("commerce-wallet 应在任务 3 注册，当前注册表不应包含它")
		}
	}
}

// TestStageAndStatusConstants 锁定协议取值，HTTP 层直接透出这些字符串。
func TestStageAndStatusConstants(t *testing.T) {
	if string(StageSchema) != "schema" || string(StageSeed) != "seed" {
		t.Fatalf("阶段常量被改动: %q %q", StageSchema, StageSeed)
	}
	if string(StatusSuccess) != "success" || string(StatusFailed) != "failed" || string(StatusSkipped) != "skipped" {
		t.Fatalf("状态常量被改动: %q %q %q", StatusSuccess, StatusFailed, StatusSkipped)
	}
}

// TestSentinelErrorsAreDistinct 确认两个哨兵错误可被 errors.Is 区分。
func TestSentinelErrorsAreDistinct(t *testing.T) {
	if errors.Is(ErrUnknownService, ErrMissingDependency) {
		t.Fatalf("两个哨兵错误不应相等")
	}
	if ErrUnknownService.Error() == "" || ErrMissingDependency.Error() == "" {
		t.Fatalf("哨兵错误必须有文案")
	}
}
