package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Muxcore-Media/core/pkg/contracts"
	databasev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/database/v1"
	"github.com/Muxcore-Media/database-sqlite/internal/db"
)

func newTestServer(t *testing.T) (*Server, context.Context) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close(context.Background()) })
	return New(d), context.Background()
}

func TestExecEmptyQuery(t *testing.T) {
	s, ctx := newTestServer(t)
	_, err := s.Exec(ctx, &databasev1.ExecRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code=%v want InvalidArgument", status.Code(err))
	}
}

func TestQueryEmptyQuery(t *testing.T) {
	s, ctx := newTestServer(t)
	_, err := s.Query(ctx, &databasev1.QueryRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code=%v want InvalidArgument", status.Code(err))
	}
}

func TestExecQueryRoundTrip(t *testing.T) {
	s, ctx := newTestServer(t)

	_, err := s.Exec(ctx, &databasev1.ExecRequest{
		Query: "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, active INTEGER)",
	})
	if err != nil {
		t.Fatalf("Exec create: %v", err)
	}

	_, err = s.Exec(ctx, &databasev1.ExecRequest{
		Query: "INSERT INTO items (name, active) VALUES (?, ?)",
		Args:  []*databasev1.Value{{Kind: &databasev1.Value_StringVal{StringVal: "alpha"}}, {Kind: &databasev1.Value_BoolVal{BoolVal: true}}},
	})
	if err != nil {
		t.Fatalf("Exec insert: %v", err)
	}

	resp, err := s.Query(ctx, &databasev1.QueryRequest{Query: "SELECT * FROM items"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(resp.Columns) != 3 {
		t.Fatalf("columns=%v want 3", resp.Columns)
	}
	if resp.Columns[0] != "id" || resp.Columns[1] != "name" || resp.Columns[2] != "active" {
		t.Fatalf("columns=%v", resp.Columns)
	}
	if len(resp.Rows) != 1 {
		t.Fatalf("rows=%d want 1", len(resp.Rows))
	}
	row := resp.Rows[0]
	if row.Values[1].GetStringVal() != "alpha" {
		t.Fatalf("name=%q", row.Values[1].GetStringVal())
	}
	if row.Values[2].GetIntVal() != 1 {
		t.Fatalf("expected active=1, got %v", row.Values[2])
	}
}

func TestQueryMixedCaseAlias(t *testing.T) {
	s, ctx := newTestServer(t)
	if _, err := s.Exec(ctx, &databasev1.ExecRequest{Query: "CREATE TABLE t (value TEXT)"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Exec(ctx, &databasev1.ExecRequest{
		Query: "INSERT INTO t (value) VALUES (?)",
		Args:  []*databasev1.Value{{Kind: &databasev1.Value_StringVal{StringVal: "x"}}},
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := s.Query(ctx, &databasev1.QueryRequest{
		Query: "SELECT value AS ItemName FROM t",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(resp.Columns) != 1 || resp.Columns[0] != "ItemName" {
		t.Fatalf("columns=%v want [ItemName]", resp.Columns)
	}
}

func TestProtoValueRoundTrip(t *testing.T) {
	s, ctx := newTestServer(t)
	if _, err := s.Exec(ctx, &databasev1.ExecRequest{
		Query: "CREATE TABLE v (s TEXT, i INTEGER, b INTEGER, blob BLOB, n TEXT)",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := s.Exec(ctx, &databasev1.ExecRequest{
		Query: "INSERT INTO v (s, i, b, blob, n) VALUES (?, ?, ?, ?, NULL)",
		Args: []*databasev1.Value{
			{Kind: &databasev1.Value_StringVal{StringVal: "str"}},
			{Kind: &databasev1.Value_IntVal{IntVal: 42}},
			{Kind: &databasev1.Value_BoolVal{BoolVal: true}},
			{Kind: &databasev1.Value_BytesVal{BytesVal: []byte{1, 2, 3}}},
		},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	resp, err := s.Query(ctx, &databasev1.QueryRequest{Query: "SELECT s, i, b, blob, n FROM v"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	vals := resp.Rows[0].Values
	if vals[0].GetStringVal() != "str" {
		t.Fatalf("string=%v", vals[0])
	}
	if vals[1].GetIntVal() != 42 {
		t.Fatalf("int=%v", vals[1])
	}
	if vals[2].GetIntVal() != 1 {
		t.Fatalf("bool-as-int=%v", vals[2])
	}
	if string(vals[3].GetBytesVal()) != string([]byte{1, 2, 3}) {
		t.Fatalf("bytes=%v", vals[3])
	}
	if !vals[4].GetNullVal() {
		t.Fatalf("null=%v", vals[4])
	}
}

func TestTransaction(t *testing.T) {
	s, ctx := newTestServer(t)
	if _, err := s.Exec(ctx, &databasev1.ExecRequest{Query: "CREATE TABLE t (v INTEGER)"}); err != nil {
		t.Fatal(err)
	}

	_, err := s.Transaction(ctx, &databasev1.TransactionRequest{
		Statements: []*databasev1.Statement{
			{Query: "INSERT INTO t (v) VALUES (?)", Args: []*databasev1.Value{{Kind: &databasev1.Value_IntVal{IntVal: 1}}}},
			{Query: "INSERT INTO t (v) VALUES (?)", Args: []*databasev1.Value{{Kind: &databasev1.Value_IntVal{IntVal: 2}}}},
		},
	})
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}

	resp, err := s.Query(ctx, &databasev1.QueryRequest{Query: "SELECT COUNT(*) FROM t"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Rows[0].Values[0].GetIntVal() != 2 {
		t.Fatalf("count=%v", resp.Rows[0].Values[0])
	}
}

func TestTransactionEmpty(t *testing.T) {
	s, ctx := newTestServer(t)
	_, err := s.Transaction(ctx, &databasev1.TransactionRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code=%v want InvalidArgument", status.Code(err))
	}
}

func TestMigrateRollback(t *testing.T) {
	s, ctx := newTestServer(t)

	_, err := s.Migrate(ctx, &databasev1.MigrateRequest{
		Migrations: []*databasev1.Migration{
			{Version: 1, Name: "create", UpSql: "CREATE TABLE users (id INTEGER PRIMARY KEY)", DownSql: "DROP TABLE IF EXISTS users"},
		},
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	_, err = s.Rollback(ctx, &databasev1.RollbackRequest{TargetVersion: 0})
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
}

func TestRollbackUnknownTarget(t *testing.T) {
	s, ctx := newTestServer(t)
	_, err := s.Migrate(ctx, &databasev1.MigrateRequest{
		Migrations: []*databasev1.Migration{
			{Version: 1, Name: "one", UpSql: "CREATE TABLE t1 (id INTEGER PRIMARY KEY)", DownSql: "DROP TABLE t1"},
			{Version: 3, Name: "three", UpSql: "CREATE TABLE t3 (id INTEGER PRIMARY KEY)", DownSql: "DROP TABLE t3"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Rollback(ctx, &databasev1.RollbackRequest{TargetVersion: 2})
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code=%v want InvalidArgument (mapped from ErrMigrationTargetNotFound)", status.Code(err))
	}
	if !strings.Contains(err.Error(), contracts.ErrMigrationTargetNotFound.Error()) {
		t.Fatalf("err=%v", err)
	}
}

func TestSyntaxErrorInvalidArgument(t *testing.T) {
	s, ctx := newTestServer(t)
	_, err := s.Exec(ctx, &databasev1.ExecRequest{Query: "SELEC 1"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code=%v want InvalidArgument", status.Code(err))
	}
}

func TestReplaceDatabaseDrain(t *testing.T) {
	path1 := filepath.Join(t.TempDir(), "a.db")
	path2 := filepath.Join(t.TempDir(), "b.db")
	d1, err := db.Open(path1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d1.Close(context.Background()) }()
	d2, err := db.Open(path2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d2.Close(context.Background()) }()

	s := New(d1)
	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		_, err := s.Query(ctx, &databasev1.QueryRequest{Query: "SELECT 1"})
		done <- err
	}()

	old := s.ReplaceDatabase(d2)
	if old != d1 {
		t.Fatal("expected old db")
	}
	s.Drain()
	_ = old.Close(ctx)

	if err := <-done; err != nil {
		t.Fatalf("query during swap: %v", err)
	}
}
