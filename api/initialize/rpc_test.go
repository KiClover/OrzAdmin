package initialize

import (
	"context"
	"errors"
	"testing"

	"github.com/KiClover/orzAdmin/kitex_gen/base"
	accessrpc "github.com/KiClover/orzAdmin/kitex_gen/system/access"
	accesssvc "github.com/KiClover/orzAdmin/kitex_gen/system/access/systemaccessservice"
	userrpc "github.com/KiClover/orzAdmin/kitex_gen/system/user"
	usersvc "github.com/KiClover/orzAdmin/kitex_gen/system/user/systemuserservice"
	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/kitex/client/callopt"
)

type initializationUserClient struct {
	usersvc.Client
	admin       *userrpc.BootstrapAdmin
	lookupCalls int
}

func (c *initializationUserClient) GetBootstrapAdmin(context.Context, *base.Empty, ...callopt.Option) (*userrpc.BootstrapAdmin, error) {
	c.lookupCalls++
	return c.admin, nil
}

type initializationAccessClient struct {
	accesssvc.Client
	initErr   error
	listErr   error
	initCalls int
	listCalls int
}

func (c *initializationAccessClient) InitDatabase(context.Context, *base.Empty, ...callopt.Option) (*base.BaseResp, error) {
	c.initCalls++
	return &base.BaseResp{}, c.initErr
}

func (c *initializationAccessClient) GetApiList(context.Context, *base.Empty, ...callopt.Option) (*accessrpc.ApiInfoListResp, error) {
	c.listCalls++
	path, method := "/custom/report", "GET"
	return &accessrpc.ApiInfoListResp{List: []*accessrpc.ApiInfo{{Path: &path, Method: &method}}}, c.listErr
}

func authorizationFixture(t *testing.T) (*casbin.Enforcer, *initializationUserClient, *initializationAccessClient) {
	t.Helper()
	enforcer, err := casbin.NewEnforcer("../../etc/casbin.conf")
	if err != nil {
		t.Fatal(err)
	}
	exists, id := true, int64(42)
	return enforcer, &initializationUserClient{admin: &userrpc.BootstrapAdmin{Exists: &exists, Id: &id}}, &initializationAccessClient{}
}

func TestFirstInitializationGrantsThroughInjectedCasbin(t *testing.T) {
	enforcer, user, access := authorizationFixture(t)
	initializer := NewRPCInitializer(user, access, nil, nil, nil, enforcer)
	result, err := initializer.InitializeService(context.Background(), ServiceAccess, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusSuccess {
		t.Fatalf("result = %+v", result)
	}
	allowed, err := enforcer.Enforce("42", "/custom/report", "GET")
	if err != nil || !allowed {
		t.Fatalf("admin permission = %v, %v", allowed, err)
	}
}

func TestLaterInitializationDoesNotRestoreRevokedPermissions(t *testing.T) {
	enforcer, user, access := authorizationFixture(t)
	initializer := NewRPCInitializer(user, access, nil, nil, nil, enforcer)
	if _, err := initializer.InitializeService(context.Background(), ServiceAccess, true); err != nil {
		t.Fatal(err)
	}
	if _, err := enforcer.RemovePolicy("admin", "/custom/report", "GET"); err != nil {
		t.Fatal(err)
	}
	if _, err := enforcer.DeleteRoleForUser("42", "admin"); err != nil {
		t.Fatal(err)
	}
	user.lookupCalls, access.listCalls = 0, 0
	if _, err := initializer.InitializeService(context.Background(), ServiceAccess, false); err != nil {
		t.Fatal(err)
	}
	policies, err := enforcer.GetFilteredPolicy(0, "admin")
	if err != nil || len(policies) != 0 {
		t.Fatalf("revoked policies restored: %v, %v", policies, err)
	}
	roles, err := enforcer.GetRolesForUser("42")
	if err != nil || len(roles) != 0 {
		t.Fatalf("revoked roles restored: %v, %v", roles, err)
	}
	if user.lookupCalls != 0 || access.listCalls != 0 {
		t.Fatal("non-first initialization performed authorization lookups")
	}
	if access.initCalls != 2 {
		t.Fatalf("schema/seed initialization calls = %d", access.initCalls)
	}
}

func TestFirstInitializationStopsBeforeGrantOnRPCFailure(t *testing.T) {
	for _, failure := range []string{"schema", "api_list", "missing_admin"} {
		t.Run(failure, func(t *testing.T) {
			enforcer, user, access := authorizationFixture(t)
			upstreamErr := errors.New("upstream failed")
			wantErr := upstreamErr
			switch failure {
			case "schema":
				access.initErr = upstreamErr
			case "api_list":
				access.listErr = upstreamErr
			case "missing_admin":
				user.admin = &userrpc.BootstrapAdmin{}
				wantErr = ErrMissingDependency
			}
			initializer := NewRPCInitializer(user, access, nil, nil, nil, enforcer)
			result, err := initializer.InitializeService(context.Background(), ServiceAccess, true)
			if !errors.Is(err, wantErr) || result.Status != StatusFailed {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			policies, err := enforcer.GetPolicy()
			if err != nil || len(policies) != 0 {
				t.Fatalf("granted after failed prerequisite: %v, %v", policies, err)
			}
		})
	}
}
