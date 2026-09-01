package internal

import (
	"context"
	"fmt"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func (m *Module) Settings() []contracts.SettingDef {
	return m.settingsDefs()
}

func (m *Module) UpdateSetting(key, value string) error {
	return m.updateSetting(key, value)
}

func (m *Module) settingsDefs() []contracts.SettingDef {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return []contracts.SettingDef{
		{
			Key:         "db_path",
			Label:       "SQLite Database Path",
			Type:        contracts.SettingTypeString,
			Value:       m.dbPath,
			Default:     "muxcore.db",
			Description: "Path to the SQLite database file (SQLITE_DB_PATH); updates reopen the DB live",
			Group:       "Storage",
		},
	}
}

func (m *Module) updateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	switch key {
	case "db_path", "SQLITE_DB_PATH":
		if value == "" {
			return fmt.Errorf("db_path must not be empty")
		}
		return m.setDBPath(value)
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}

func (m *Module) setDBPath(path string) error {
	if err := validateDBPath(path); err != nil {
		return err
	}
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	if path == m.dbPath {
		return nil
	}
	if m.srv == nil {
		m.dbPath = path
		return nil
	}
	d, err := openValidatedDB(path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	old := m.srv.ReplaceDatabase(d)
	m.database = d
	m.dbPath = path
	if old != nil {
		m.srv.Drain()
		_ = old.Close(context.Background())
	}
	return nil
}
