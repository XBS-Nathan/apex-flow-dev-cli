# CLAUDE.md

## Project

`dev` — a fast, lightweight CLI for managing Laravel (and generic) development environments on Linux and macOS. Alternative to DDEV. Orchestrates PHP-FPM, Caddy, Docker (MySQL/Redis/Typesense/Postgres), database snapshots, and Xdebug.

## Quick Reference

```bash
# Build
go build ./...

# Run all unit tests
go test ./...

# Run with race detector
go test ./... -race -count=1

# Run fuzz tests (10s each)
go test ./internal/db/ -fuzz=FuzzSanitizeDBName -fuzztime=10s
go test ./internal/config/ -fuzz=FuzzDbNameFromDir -fuzztime=10s

# Run integration tests (requires Docker + mysql-client + postgresql-client)
go test -tags=integration -count=1 ./internal/db/ -timeout 300s

# Vet
go vet ./...
```

## Architecture

```
cmd/                     # Thin cobra command wrappers — no business logic
internal/
  config/                # .dev.yaml loading, ProjectConfig, MySQLConfig, PostgresConfig
  project/               # Project detection (walks up looking for markers)
  lifecycle/             # Orchestration: Start, Stop, Down (testable via interfaces)
  db/                    # db.Store interface + MySQLStore/PostgresStore adapters
  caddy/                 # Caddy reverse proxy management + Service adapter
  docker/                # Docker compose management + Service adapter
  php/                   # PHP-FPM helpers (socket paths, service names)
```

**Dependency direction:** `cmd/ → lifecycle → {caddy, docker, db, project} → config`

**Key interfaces:**
- `db.Store` — `CreateIfNotExists`, `Drop`, `Snapshot`, `Restore`
- `lifecycle.DockerService` — `Up`, `Down`
- `lifecycle.CaddyService` — `Start`, `Stop`, `Link`, `Unlink`

## Conventions

### Go Standards

- **Imports:** stdlib, blank line, external, blank line, internal
- **Errors:** always wrap with context (`fmt.Errorf("doing X: %w", err)`), never bare `return nil, err`
- **Ignored errors:** must use `_ = fn()` with a comment explaining why
- **Line length:** soft limit 99 characters
- **Tests:** table-driven for pure functions, subtests for behavior with setup, `t.Parallel()` where safe, `t.Helper()` on all helpers, `t.Cleanup()` over `defer`

### Project Patterns

- **cmd/ must be thin** — all orchestration lives in `internal/lifecycle`
- **Adapter pattern for DB** — `db.NewStore(config)` returns `MySQLStore` or `PostgresStore` based on `db_driver`
- **Service wrappers** — `caddy.Service{}` and `docker.Service{}` wrap package-level functions to satisfy lifecycle interfaces
- **Config defaults** — `config.Load()` fills all defaults so callers never see zero values for PHP, Node, DBDriver, MySQL/Postgres connection settings
- **Snapshot formats** — snapshots are directories (mydumper/pg_dump -Fd). `ListSnapshots` also finds `.sql` and `.sql.gz` files for manual restore.

### Database Support

- MySQL: `mydumper`/`myloader` for parallel snapshot/restore, falls back to `mysqldump`/`mysql`
- Postgres: `pg_dump -Fd -j4` / `pg_restore -j4` with lz4 compression
- Configured via `db_driver: mysql|postgres` in `.dev.yaml`

### Integration Tests

- Guarded by `//go:build integration` tag
- Use `testcontainers-go` for real MySQL/Postgres containers
- Skip gracefully if CLI tools (`mysql`, `psql`) aren't in PATH
- Never run with `go test ./...` — require explicit `-tags=integration`
