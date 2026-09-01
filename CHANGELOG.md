# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.5] — 2026-08-10

### Added

- SQLite database sidecar (`database` capability) via `modernc.org/sqlite` (no CGO)
- gRPC `DatabaseService`: Exec, Query, Transaction, Migrate, Rollback
- Config: `SQLITE_DB_PATH` (default `muxcore.db`), `DATABASE_GRPC_ADDR` (default `127.0.0.1:9700`)
- `Backupable` export/import via WAL checkpoint + file snapshot
- Admin settings for live `db_path` swap with in-flight RPC drain
- Transactional migrations; rollback honors empty Down SQL and unknown-target errors

### Changed

- Advertise `settings` capability for admin-ui Settings discovery
- Default gRPC bind to loopback (`127.0.0.1:9700`)
- SQL syntax/constraint errors map to gRPC `InvalidArgument`; driver faults to `Internal`

## [0.1.3] — 2026-08-10

### Fixed

- Sync Info()/muxcore.json version to **0.1.3**.
