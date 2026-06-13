# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Database proto definition (`muxcore/database/v1/database.proto`) with gRPC service spec
- Generated Go code from database proto (messages + gRPC stubs)
- Full test suite: 16 db unit tests, module lifecycle test
- Parameterized SQL execution with typed argument conversion (string/int/float/bool/bytes/null)
- Versioned schema migrations with `_migrations` tracking table
- Rollback support with down-sql replay in reverse version order
- WAL journal mode, busy timeout, synchronous NORMAL, foreign keys
- Prometheus-format metrics counters (exec/query/migrate)
- Production deployment: Dockerfile, docker-compose, systemd unit
- Contract declaration: `DatabaseProvider` with `MinCoreVersion: 0.4.0`

### Changed

- Module info now declares `Contracts` and `MinCoreVersion`
- Makefile binary name and Docker org set to `database-sqlite`
- Dockerfile: fixed binary name, removed invalid `--health-check`

## [0.1.0] - 2026-06-13

### Added

- Initial project scaffold from muxcore-module-starter
