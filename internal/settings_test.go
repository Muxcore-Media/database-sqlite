package internal

import (
	"path/filepath"
	"testing"
)

func TestSettingsDBPath(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "a.db")
	path2 := filepath.Join(dir, "b.db")
	m := NewModule(Config{DBPath: path1, GRPCAddr: "127.0.0.1:0"})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(t.Context()) }()

	if err := m.UpdateSetting("db_path", path2); err != nil {
		t.Fatal(err)
	}
	if got := m.Settings()[0].Value; got != path2 {
		t.Fatalf("path=%q", got)
	}
	if err := m.Health(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("db_path", ""); err == nil {
		t.Fatal("expected error")
	}
	if err := m.UpdateSetting("db_path", "../escape.db"); err == nil {
		t.Fatal("expected error for path traversal")
	}
}
