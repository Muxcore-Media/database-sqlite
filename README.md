# Database SQLite

[![CI](https://github.com/Muxcore-Media/database-sqlite/actions/workflows/ci.yml/badge.svg)](https://github.com/Muxcore-Media/database-sqlite/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**SQLite database provider for MuxCore (pure Go, no CGO).**

A MuxCore sidecar module that exposes a shared SQLite database over gRPC (`Exec` / `Query` / `Transaction` / `Migrate` / `Rollback`). Uses `modernc.org/sqlite` so builds need no C toolchain. Provides the `database` capability.

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
| `DATABASE_GRPC_ADDR` | `:9700` | gRPC listen address |

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
./database-sqlite --muxcore-mesh-addr localhost:9090
```

---

## Capability

`database` — Shared SQLite database provider

## License

GPL-3.0
