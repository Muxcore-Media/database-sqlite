# Changelog


## [0.1.5] — 2026-08-10

### Changed

- Advertise `settings` capability for admin-ui Settings discovery

## [0.1.3] — 2026-08-10

### Fixed
- Sync Info()/muxcore.json version to **0.1.3**.

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- SQLite database sidecar (`database` capability) via `modernc.org/sqlite` (no CGO)
- gRPC `DatabaseService`: Exec, Query, Transaction, Migrate, Rollback
- Config: `SQLITE_DB_PATH` (default `muxcore.db`), `DATABASE_GRPC_ADDR` (default `:9700`)
