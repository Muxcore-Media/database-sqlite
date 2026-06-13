# Database SQLite

[![CI](https://github.com/Muxcore-Media/database-sqlite/actions/workflows/ci.yml/badge.svg)](https://github.com/Muxcore-Media/database-sqlite/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**SQLite database provider for MuxCore (pure Go, no CGO).**

A MuxCore sidecar module that provides persistent SQL storage via SQLite using `modernc.org/sqlite` (a pure Go port). Without this module, modules that depend on `DatabaseProvider` will fail at discovery.

---

## How It Works

```
Module request ──→ database-sqlite (gRPC) ──→ SQLite (WAL mode)
                     │
                     ▼
              Exec/Query/Transaction/
              Migrate/Rollback
```

### Key features

- **Exec/Query** — Parameterized SQL execution and read queries with typed result sets.
- **Transactions** — Atomic multi-statement transactions with automatic rollback on error.
- **Migrations** — Versioned schema migrations with tracking table (`_migrations`). Idempotent — already-applied migrations are skipped.
- **Rollback** — Revert schema changes down to a target version by applying `Down` SQL in reverse order.
- **WAL mode** — Write-Ahead Logging for better concurrent read performance.
- **Metrics** — Prometheus-format counters for exec, query, and migrate operations, exposed via gRPC.

---

## Configuration

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--db-path` | `muxcore.db` | Path to SQLite database file |
| `--grpc-addr` | `:9700` | gRPC listen address |

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SQLITE_DB_PATH` | `muxcore.db` | Path to SQLite database file |
| `DATABASE_GRPC_ADDR` | `:9700` | gRPC listen address |

---

## Quick Start

```bash
# Build
make build

# Run against local core (dev mode)
export MUXCORE_INSECURE_DISABLE_TLS=true
./database-sqlite --muxcore-mesh-addr localhost:9090
```

---

## Deployment

### Docker

```bash
make docker
docker run -d --restart=unless-stopped \
  -e SQLITE_DB_PATH=/data/muxcore.db \
  -e MUXCORE_GRPC_ADDR=core:9090 \
  -v sqlite-data:/data \
  ghcr.io/muxcore-media/database-sqlite:latest
```

### docker-compose

```bash
docker compose -f deploy/docker-compose.yml up
```

---

## Development

```bash
make dev      # run in dev mode
make test     # run tests
make lint     # golangci-lint
make fmt      # format code
```

### Integration Tests

```bash
# Start core in dev mode, then:
MUXCORE_GRPC_ADDR=localhost:9090 go test -tags=integration -race -count=1 ./test/
```

---

## Implementation

- Registers with capability: `"database"`
- Implements `contracts.DatabaseProvider`
- Uses `modernc.org/sqlite` (pure Go, no CGO)
- WAL journal mode, busy timeout 5s, synchronous NORMAL, foreign keys ON
- Single-writer connection pool (`MaxOpenConns=1`)

---

## License

GPL-3.0
