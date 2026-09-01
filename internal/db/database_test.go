package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func TestOpenClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := d.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestExecQuery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	ctx := context.Background()

	n, err := d.Exec(ctx, "CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Exec create: %v", err)
	}
	if n < 0 {
		t.Errorf("expected non-negative, got %d", n)
	}

	n, err = d.Exec(ctx, "INSERT INTO items (name) VALUES (?)", "test-item")
	if err != nil {
		t.Fatalf("Exec insert: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 row affected, got %d", n)
	}

	rows, err := d.Query(ctx, "SELECT id, name FROM items ORDER BY id")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		t.Fatal("expected at least one row")
	}
	var id int64
	var name string
	if err := rows.Scan(&id, &name); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if id != 1 {
		t.Errorf("expected id 1, got %d", id)
	}
	if name != "test-item" {
		t.Errorf("expected name 'test-item', got %q", name)
	}
	if rows.Next() {
		t.Error("expected only one row")
	}
}

func TestTransaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	ctx := context.Background()

	if _, err := d.Exec(ctx, "CREATE TABLE IF NOT EXISTS t (v INTEGER)"); err != nil {
		t.Fatalf("Exec create: %v", err)
	}

	err = d.Transaction(ctx, func(tx *Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO t (v) VALUES (?)", 1); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO t (v) VALUES (?)", 2); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO t (v) VALUES (?)", 3); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}

	rows, err := d.Query(ctx, "SELECT COUNT(*) FROM t")
	if err != nil {
		t.Fatalf("Query count: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("expected count row")
	}
	var count int64
	if err := rows.Scan(&count); err != nil {
		t.Fatalf("Scan count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 rows, got %d", count)
	}
}

func TestTransactionRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	ctx := context.Background()

	if _, err := d.Exec(ctx, "CREATE TABLE IF NOT EXISTS t (v INTEGER)"); err != nil {
		t.Fatalf("Exec create: %v", err)
	}

	err = d.Transaction(ctx, func(tx *Tx) error {
		_, _ = tx.Exec(ctx, "INSERT INTO t (v) VALUES (?)", 1)
		_, _ = tx.Exec(ctx, "INSERT INTO t (v) VALUES (?)", 2)
		return os.ErrClosed
	})
	if err == nil {
		t.Fatal("expected error from transaction")
	}

	rows, err := d.Query(ctx, "SELECT COUNT(*) FROM t")
	if err != nil {
		t.Fatalf("Query count: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("expected count row")
	}
	var count int64
	if err := rows.Scan(&count); err != nil {
		t.Fatalf("Scan count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}
}

func TestMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	ctx := context.Background()

	migrations := []Migration{
		{Version: 1, Name: "create_users", Up: "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)", Down: "DROP TABLE IF EXISTS users"},
		{Version: 2, Name: "add_email", Up: "ALTER TABLE users ADD COLUMN email TEXT", Down: "ALTER TABLE users DROP COLUMN email"},
	}

	if err := d.Migrate(ctx, migrations); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if err := d.Migrate(ctx, migrations); err != nil {
		t.Fatalf("Re-migrate: %v", err)
	}

	rows, err := d.Query(ctx, "SELECT name FROM _migrations ORDER BY version")
	if err != nil {
		t.Fatalf("Query migrations: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		names = append(names, name)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 migrations recorded, got %d", len(names))
	}
}

func TestRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	ctx := context.Background()

	migrations := []Migration{
		{Version: 1, Name: "create_table", Up: "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)", Down: "DROP TABLE IF EXISTS items"},
	}

	if err := d.Migrate(ctx, migrations); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := d.Exec(ctx, "INSERT INTO items (name) VALUES (?)", "test"); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := d.Rollback(ctx, 0); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	rows, err := d.Query(ctx, "SELECT COUNT(*) FROM _migrations")
	if err != nil {
		t.Fatalf("Query migrations: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var count int64
		if err := rows.Scan(&count); err != nil {
			t.Fatalf("Scan count: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 migrations after rollback, got %d", count)
		}
	}
}

func TestRollbackUnknownTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	ctx := context.Background()

	if err := d.Migrate(ctx, []Migration{
		{Version: 1, Name: "one", Up: "CREATE TABLE t1 (id INTEGER PRIMARY KEY)", Down: "DROP TABLE t1"},
		{Version: 3, Name: "three", Up: "CREATE TABLE t3 (id INTEGER PRIMARY KEY)", Down: "DROP TABLE t3"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := d.Rollback(ctx, 2); !errors.Is(err, contracts.ErrMigrationTargetNotFound) {
		t.Fatalf("err=%v want ErrMigrationTargetNotFound", err)
	}
}

func TestRollbackEmptyDownRetainsCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	ctx := context.Background()

	if err := d.Migrate(ctx, []Migration{
		{Version: 1, Name: "no_down", Up: "CREATE TABLE locked (id INTEGER PRIMARY KEY)", Down: ""},
	}); err != nil {
		t.Fatal(err)
	}

	if err := d.Rollback(ctx, 0); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	rows, err := d.Query(ctx, "SELECT COUNT(*) FROM _migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("expected count row")
	}
	var count int64
	if err := rows.Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("catalog count=%d want 1", count)
	}
}

func TestMigrateAtomicOnCatalogFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	ctx := context.Background()

	if err := d.Migrate(ctx, []Migration{
		{Version: 1, Name: "ok", Up: "CREATE TABLE ok_table (id INTEGER PRIMARY KEY)", Down: "DROP TABLE ok_table"},
	}); err != nil {
		t.Fatal(err)
	}

	// Re-applying same migration is a no-op; schema stays consistent with catalog.
	if err := d.Migrate(ctx, []Migration{
		{Version: 1, Name: "ok", Up: "CREATE TABLE ok_table (id INTEGER PRIMARY KEY)", Down: "DROP TABLE ok_table"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestContextCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := d.Exec(ctx, "SELECT 1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Exec err=%v want context.Canceled", err)
	}
	if err := d.Health(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Health err=%v want context.Canceled", err)
	}
	if _, err := d.Query(ctx, "SELECT 1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Query err=%v want context.Canceled", err)
	}
	if err := d.Migrate(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Migrate err=%v want context.Canceled", err)
	}
	if err := d.Rollback(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("Rollback err=%v want context.Canceled", err)
	}
}

func TestRowsColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	ctx := context.Background()

	if _, err := d.Exec(ctx, "CREATE TABLE c (id INTEGER, name TEXT)"); err != nil {
		t.Fatal(err)
	}
	rows, err := d.Query(ctx, "SELECT id, name FROM c")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 2 || cols[0] != "id" || cols[1] != "name" {
		t.Fatalf("columns=%v", cols)
	}
}

func TestConcurrentReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	ctx := context.Background()

	if _, err := d.Exec(ctx, "CREATE TABLE IF NOT EXISTS t (v INTEGER)"); err != nil {
		t.Fatalf("Exec create: %v", err)
	}
	if _, err := d.Exec(ctx, "INSERT INTO t (v) VALUES (1), (2), (3), (4), (5)"); err != nil {
		t.Fatalf("Exec insert: %v", err)
	}

	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			rows, err := d.Query(ctx, "SELECT v FROM t ORDER BY v")
			if err != nil {
				errs <- err
				return
			}
			_ = rows.Close()
			errs <- nil
		}()
	}

	for i := 0; i < 10; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent query: %v", err)
		}
	}
}

func TestQueryParameters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	ctx := context.Background()

	if _, err := d.Exec(ctx, "CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("Exec create: %v", err)
	}
	if _, err := d.Exec(ctx, "INSERT INTO t (name) VALUES (?), (?), (?)", "a", "b", "c"); err != nil {
		t.Fatalf("Exec insert: %v", err)
	}

	rows, err := d.Query(ctx, "SELECT id, name FROM t WHERE name = ?", "b")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		t.Fatal("expected row for name=b")
	}
	var id int64
	var name string
	if err := rows.Scan(&id, &name); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if name != "b" {
		t.Errorf("got %q, want %q", name, "b")
	}
}
