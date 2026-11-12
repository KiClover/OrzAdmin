package initialize

import (
	"errors"
	"testing"
)

func TestServiceOrderEndsWithWallet(t *testing.T) {
	names := ServiceNames()
	want := []string{
		ServiceUser,
		ServiceAccess,
		ServiceSetting,
		ServiceMessage,
		ServiceToken,
		ServiceWallet,
	}
	if len(names) != len(want) {
		t.Fatalf("service names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("service names = %v, want %v", names, want)
		}
	}
}

func TestLookupServiceAcceptsWallet(t *testing.T) {
	info, err := LookupService(ServiceWallet)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != ServiceWallet {
		t.Fatalf("info = %+v", info)
	}
	if len(info.DependsOn) != 0 {
		t.Fatalf("wallet should not depend on other services: %v", info.DependsOn)
	}
}

func TestLookupServiceRejectsUnknown(t *testing.T) {
	if _, err := LookupService("commerce-payment"); !errors.Is(err, ErrUnknownService) {
		t.Fatalf("error = %v, want ErrUnknownService", err)
	}
}

func TestServiceNamesAreUnique(t *testing.T) {
	seen := make(map[string]struct{})
	for _, name := range ServiceNames() {
		if _, ok := seen[name]; ok {
			t.Fatalf("duplicate service name: %s", name)
		}
		seen[name] = struct{}{}
	}
}
