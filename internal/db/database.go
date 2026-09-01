package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Muxcore-Media/core/pkg/contracts"
	_ "modernc.org/sqlite"
)

type Database struct {
	mu   sync.Mutex
	db   *sql.DB
	path string
}

type Rows struct {
	rows *sql.Rows
}

func (r *Rows) Next() bool {
	return r.rows.Next()
}

func (r *Rows) Scan(dest ...any) error {
	return r.rows.Scan(dest...)
}

func (r *Rows) Close() error {
	return r.rows.Close()
}

func (r *Rows) Columns() ([]string, error) {
	return r.rows.Columns()
}

func (r *Rows) Err() error {
	return r.rows.Err()
}

type Tx struct {
	tx *sql.Tx
}

func (t *Tx) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite exec: %w", err)
	}
	return result.RowsAffected()
}

func (t *Tx) Query(ctx context.Context, query string, args ...any) (*Rows, error) {
	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite query: %w", err)
	}
	return &Rows{rows: rows}, nil
}

func Open(path string) (*Database, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	d := &Database{
		db:   db,
		path: path,
	}

	if err := d.Health(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite health check: %w", err)
	}

	slog.Info("database: opened SQLite", "path", path)
	return d, nil
}

func (d *Database) Path() string {
	return d.path
}

func (d *Database) Close(_ context.Context) error {
	return d.db.Close()
}

func (d *Database) Health(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *Database) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := d.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite exec: %w", err)
	}
	return result.RowsAffected()
}

func (d *Database) Query(ctx context.Context, query string, args ...any) (*Rows, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite query: %w", err)
	}
	return &Rows{rows: rows}, nil
}

func (d *Database) Transaction(ctx context.Context, fn func(tx *Tx) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite begin tx: %w", err)
	}
	if err := fn(&Tx{tx: tx}); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			slog.Error("sqlite rollback failed", "error", rbErr)
		}
		return err
	}
	return tx.Commit()
}

func (d *Database) CheckpointWAL(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

type Migration struct {
	Version int
	Name    string
	Up      string
	Down    string
}

func createMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS _migrations (
		version INTEGER PRIMARY KEY,
		name    TEXT NOT NULL,
		up_sql  TEXT NOT NULL DEFAULT '',
		down_sql TEXT NOT NULL DEFAULT '',
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	return err
}

func (d *Database) Migrate(ctx context.Context, migrations []Migration) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := createMigrationsTable(ctx, d.db); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	for _, m := range migrations {
		var exists int
		err := d.db.QueryRowContext(ctx, "SELECT 1 FROM _migrations WHERE version = ?", m.Version).Scan(&exists)
		if err == nil {
			slog.Debug("migration already applied", "version", m.Version, "name", m.Name)
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check migration %d: %w", m.Version, err)
		}

		slog.Info("applying migration", "version", m.Version, "name", m.Name)
		tx, err := d.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration tx %d: %w", m.Version, err)
		}
		if _, err := tx.ExecContext(ctx, m.Up); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d %q: %w", m.Version, m.Name, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO _migrations (version, name, up_sql, down_sql) VALUES (?, ?, ?, ?)",
			m.Version, m.Name, m.Up, m.Down,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}
	}
	return nil
}

func (d *Database) Rollback(ctx context.Context, targetVersion int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := createMigrationsTable(ctx, d.db); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	var maxVersion sql.NullInt64
	if err := d.db.QueryRowContext(ctx, "SELECT MAX(version) FROM _migrations").Scan(&maxVersion); err != nil {
		return fmt.Errorf("query max migration version: %w", err)
	}
	if !maxVersion.Valid || int(maxVersion.Int64) <= targetVersion {
		return nil
	}

	if targetVersion > 0 {
		var exists int
		err := d.db.QueryRowContext(ctx, "SELECT 1 FROM _migrations WHERE version = ?", targetVersion).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return contracts.ErrMigrationTargetNotFound
		}
		if err != nil {
			return fmt.Errorf("check target version %d: %w", targetVersion, err)
		}
	}

	type migInfo struct {
		Version int
		Name    string
		DownSQL string
	}

	rows, err := d.db.QueryContext(ctx,
		"SELECT version, name, down_sql FROM _migrations WHERE version > ? ORDER BY version DESC",
		targetVersion,
	)
	if err != nil {
		return fmt.Errorf("query migrations for rollback: %w", err)
	}

	var toRollback []migInfo
	for rows.Next() {
		var m migInfo
		if err := rows.Scan(&m.Version, &m.Name, &m.DownSQL); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan migration: %w", err)
		}
		toRollback = append(toRollback, m)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close migration rows: %w", err)
	}

	for _, m := range toRollback {
		slog.Info("rolling back migration", "version", m.Version, "name", m.Name)
		if m.DownSQL == "" {
			slog.Warn("migration has no down SQL; catalog row retained", "version", m.Version, "name", m.Name)
			continue
		}
		tx, err := d.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin rollback tx %d: %w", m.Version, err)
		}
		if _, err := tx.ExecContext(ctx, m.DownSQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("rollback migration %d: %w", m.Version, err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM _migrations WHERE version = ?", m.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete migration record %d: %w", m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit rollback %d: %w", m.Version, err)
		}
	}

	return nil
}
