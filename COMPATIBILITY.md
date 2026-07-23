# Compatibility

## Core Version

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.1.0         | v0.4.0+     | Current |

## Capabilities

| Capability | Status |
|------------|--------|
| `database` | Current |

## Contracts

| Contract | Capability | Status |
|----------|-----------|--------|
| DatabaseProvider (gRPC `DatabaseService`) | `database` | Current |

`muxcore.json` lists `"contracts": []` (no separate contract package ID); the module registers capability `database` and serves the core database gRPC API.

## Breaking Changes

This is a pre-1.0 module. Interfaces may change without notice.
