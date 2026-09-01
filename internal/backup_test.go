package internal

import (
	"bytes"
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	databasev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/database/v1"
)

func TestBackupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.db")
	m := NewModule(Config{DBPath: path, GRPCAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(ctx) }()

	if _, err := m.database.Exec(ctx, "CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.database.Exec(ctx, "INSERT INTO notes (body) VALUES (?)", "hello"); err != nil {
		t.Fatal(err)
	}

	data, err := m.ExportState(ctx)
	if err != nil {
		t.Fatalf("ExportState: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty export")
	}

	if _, err := m.database.Exec(ctx, "DELETE FROM notes"); err != nil {
		t.Fatal(err)
	}

	if err := m.ImportState(ctx, data); err != nil {
		t.Fatalf("ImportState: %v", err)
	}

	rows, err := m.database.Query(ctx, "SELECT body FROM notes")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("expected row after restore")
	}
	var body string
	if err := rows.Scan(&body); err != nil {
		t.Fatal(err)
	}
	if body != "hello" {
		t.Fatalf("body=%q", body)
	}
}

func TestBackupEmptyPayload(t *testing.T) {
	m := NewModule(Config{DBPath: filepath.Join(t.TempDir(), "x.db"), GRPCAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(ctx) }()

	if err := m.ImportState(ctx, nil); err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestConcurrentQueryDuringDBSwap(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "a.db")
	path2 := filepath.Join(dir, "b.db")
	m := NewModule(Config{DBPath: path1, GRPCAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(ctx) }()

	if _, err := m.database.Exec(ctx, "CREATE TABLE t (v INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.database.Exec(ctx, "INSERT INTO t (v) VALUES (1)"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.srv.Query(ctx, &databasev1.QueryRequest{Query: "SELECT v FROM t"})
			errs <- err
		}()
	}

	time.Sleep(10 * time.Millisecond)
	if err := m.UpdateSetting("db_path", path2); err != nil {
		t.Fatal(err)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
	}
}

func TestValidateDBPathRejectsTraversal(t *testing.T) {
	if err := validateDBPath("../escape.db"); err == nil {
		t.Fatal("expected error for .. path")
	}
	if err := validateDBPath(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestModuleInfoBackupable(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	found := false
	for _, c := range info.Capabilities {
		if c == "backupable" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("capabilities=%v", info.Capabilities)
	}
}

func TestDefaultGRPCAddrLoopback(t *testing.T) {
	m := NewModule(Config{})
	if m.grpcAddr != defaultGRPCAddr {
		t.Fatalf("addr=%q want %q", m.grpcAddr, defaultGRPCAddr)
	}
}

func TestExportStateUsesCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.db")
	m := NewModule(Config{DBPath: path, GRPCAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(ctx) }()

	data, err := m.ExportState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("SQLite format 3")) {
		t.Fatal("expected sqlite file header")
	}
}
