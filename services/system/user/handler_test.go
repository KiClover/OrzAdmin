package user

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/kitex_gen/system/user"
	"github.com/KiClover/orzAdmin/pkg/utils"
)

type capturedQuery struct {
	query string
	args  []any
}

type captureConnector struct {
	queries *[]capturedQuery
}

func (c *captureConnector) Connect(context.Context) (driver.Conn, error) {
	return &captureConn{queries: c.queries}, nil
}

func (c *captureConnector) Driver() driver.Driver { return nil }

type captureConn struct {
	queries *[]capturedQuery
}

func (c *captureConn) Prepare(string) (driver.Stmt, error) { return nil, nil }
func (c *captureConn) Close() error                        { return nil }
func (c *captureConn) Begin() (driver.Tx, error)           { return nil, nil }
func (c *captureConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &captureTx{}, nil
}

type captureResult struct{}

func (captureResult) LastInsertId() (int64, error) { return 1, nil }
func (captureResult) RowsAffected() (int64, error) { return 1, nil }

func (c *captureConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.record(query, args)
	return captureResult{}, nil
}

func (c *captureConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.record(query, args)
	return &captureRows{query: query}, nil
}

func (c *captureConn) record(query string, args []driver.NamedValue) {
	vals := make([]any, len(args))
	for i, a := range args {
		vals[i] = a.Value
	}
	*c.queries = append(*c.queries, capturedQuery{query: query, args: vals})
}

type captureTx struct{}

func (captureTx) Commit() error   { return nil }
func (captureTx) Rollback() error { return nil }

type captureRows struct {
	query string
	done  bool
}

func (r *captureRows) isSingleScalar() bool {
	return !strings.Contains(r.query, "`created_at`")
}

func (r *captureRows) Columns() []string {
	if r.isSingleScalar() {
		return []string{"val"}
	}
	return []string{"id", "created_at", "updated_at", "name", "avatar", "description", "status", "gender", "mobile", "email", "pwd"}
}

func (r *captureRows) Close() error { return nil }

func (r *captureRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	if r.isSingleScalar() {
		dest[0] = int64(1)
		return nil
	}
	now := time.Now()
	dest[0] = int64(1)
	dest[1] = now
	dest[2] = now
	dest[3] = "user"
	dest[4] = ""
	dest[5] = ""
	dest[6] = int64(0)
	dest[7] = int64(3)
	dest[8] = ""
	dest[9] = "alice@example.com"
	dest[10] = "hashed"
	return nil
}

func newCaptureUserService(t *testing.T) (*SystemUserService, *[]capturedQuery) {
	t.Helper()
	var queries []capturedQuery
	sqlDB := sql.OpenDB(&captureConnector{queries: &queries})
	t.Cleanup(func() { _ = sqlDB.Close() })

	client := ent.NewClient(ent.Driver(entsql.OpenDB("mysql", sqlDB)))
	return &SystemUserService{
		DB:    db.NewSystemUserDB(client),
		DBAll: client,
	}, &queries
}

func hasArg(queries []capturedQuery, want string) bool {
	for _, q := range queries {
		if slices.Contains(q.args, any(want)) {
			return true
		}
	}
	return false
}

func TestSystemUserServiceNormalizesEmailOnWriteAndQuery(t *testing.T) {
	ctx := context.Background()
	svc, queries := newCaptureUserService(t)

	// 1. CreateUser: 捕获写入参数，必须已转小写且去空格。
	if _, err := svc.CreateUser(ctx, &user.UserInfo{
		Name:     utils.GetPointer("alice"),
		Password: utils.GetPointer("Secret123!"),
		Email:    utils.GetPointer("  Alice@Example.COM "),
	}); err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	if !hasArg(*queries, "alice@example.com") {
		t.Fatalf("CreateUser SQL args did not contain normalized email: %+v", *queries)
	}

	// 2. UpdateUser: 捕获更新参数。
	*queries = nil
	if _, err := svc.UpdateUser(ctx, &user.UserInfo{
		Id:    utils.GetPointer(int64(1)),
		Email: utils.GetPointer(" Bob@Example.COM "),
	}); err != nil {
		t.Fatalf("UpdateUser returned error: %v", err)
	}
	if !hasArg(*queries, "bob@example.com") {
		t.Fatalf("UpdateUser SQL args did not contain normalized email: %+v", *queries)
	}

	// 3. GetUserByEmail: 捕获查询条件参数。
	*queries = nil
	if _, err := svc.GetUserByEmail(ctx, &user.GetUserByEmailReq{
		Email: utils.GetPointer("  CAROL@Example.COM "),
	}); err != nil {
		t.Fatalf("GetUserByEmail returned error: %v", err)
	}
	if !hasArg(*queries, "carol@example.com") {
		t.Fatalf("GetUserByEmail SQL args did not contain normalized email: %+v", *queries)
	}

	// 4. GetUserIsExist: 捕获查询条件参数。
	*queries = nil
	resp, err := svc.GetUserIsExist(ctx, &user.GetUserIsExistReq{
		Email: utils.GetPointer(" DAVE@Example.COM "),
	})
	if err != nil {
		t.Fatalf("GetUserIsExist returned error: %v", err)
	}
	if !resp.IsExist {
		t.Fatalf("GetUserIsExist expected true from count=1 stub")
	}
	if !hasArg(*queries, "dave@example.com") {
		t.Fatalf("GetUserIsExist SQL args did not contain normalized email: %+v", *queries)
	}

	// 5. GetUserList: 捕获列表过滤条件参数。
	*queries = nil
	if _, err := svc.GetUserList(ctx, &user.GetUserListReq{
		Page:     1,
		PageSize: 10,
		Email:    utils.GetPointer(" EVE@Example.COM "),
	}); err != nil {
		t.Fatalf("GetUserList returned error: %v", err)
	}
	if !hasArg(*queries, "eve@example.com") {
		t.Fatalf("GetUserList SQL args did not contain normalized email: %+v", *queries)
	}
}
