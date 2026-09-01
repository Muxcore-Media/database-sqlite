package internal

import (
	"context"
	"fmt"
	"os"
)

// ExportState implements contracts.Backupable — WAL checkpoint then read the DB file.
func (m *Module) ExportState(ctx context.Context) ([]byte, error) {
	m.cfgMu.RLock()
	path := m.dbPath
	m.cfgMu.RUnlock()
	if path == "" {
		return nil, fmt.Errorf("db path not set")
	}
	if m.database != nil {
		if err := m.database.CheckpointWAL(ctx); err != nil {
			return nil, fmt.Errorf("wal checkpoint: %w", err)
		}
	}
	return os.ReadFile(path)
}

// ImportState replaces the SQLite database from a backup snapshot.
func (m *Module) ImportState(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty backup payload")
	}
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	if err := validateDBPath(m.dbPath); err != nil {
		return err
	}
	if m.srv != nil {
		m.srv.Drain()
	}
	if m.database != nil {
		_ = m.database.Close(ctx)
		m.database = nil
	}
	if err := os.WriteFile(m.dbPath, data, 0600); err != nil {
		return fmt.Errorf("write db: %w", err)
	}
	d, err := openValidatedDB(m.dbPath)
	if err != nil {
		return err
	}
	if m.srv != nil {
		old := m.srv.ReplaceDatabase(d)
		if old != nil {
			_ = old.Close(ctx)
		}
	}
	m.database = d
	return nil
}
