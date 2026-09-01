# Database SQLite

[![CI](https://git.zem.systems/muxcore/database-sqlite/actions/workflows/ci.yml/badge.svg)](https://git.zem.systems/muxcore/database-sqlite/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**SQLite database provider for MuxCore (pure Go, no CGO).**

A MuxCore sidecar module that exposes a shared SQLite database over gRPC (`Exec` / `Query` / `Transaction` / `Migrate` / `Rollback`). Uses `modernc.org/sqlite` so builds need no C toolchain. Provides the `database` capability and implements `Backupable` for `backup-local` snapshots.

---

## How It Works

```
Module request ──→ database-sqlite (gRPC) ──→ SQLite (WAL)
```

Opened with WAL journaling, busy timeout, and foreign keys enabled. Single-connection pool for safe concurrent access through the sidecar.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SQLITE_DB_PATH` | `muxcore.db` | SQLite database file path |
| `DATABASE_GRPC_ADDR` | `127.0.0.1:9700` | gRPC listen address (loopback by default) |

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
export DATABASE_GRPC_ADDR=127.0.0.1:9700
./database-sqlite --muxcore-mesh-addr localhost:9090
```

---

## Capabilities

- `database` — Shared SQLite database provider
- `backupable` — WAL checkpoint + file snapshot for `backup-local`

## License

GPL-3.0
