# Per-project & Global php.ini and my.cnf Overrides

**Date:** 2026-04-06

## Problem

PHP ini and MySQL cnf settings are hardcoded with minimal defaults. There's no way to customize them per-project or globally. The defaults are also not optimized for local development.

## Goals

1. Allow per-project and global overrides for php.ini and my.cnf settings via flat key-value maps
2. Ship dev-optimized hardcoded defaults for both PHP and MySQL
3. Deliver settings at runtime (no image rebuild required)

## Config Format

### Per-project (`.dev.yaml`)

```yaml
php_ini:
  memory_limit: 1G
  upload_max_filesize: 500M

mysql_cnf:
  innodb_buffer_pool_size: 1G
  max_connections: 500
```

### Global (`~/.dev/config.yaml`)

```yaml
php_ini:
  memory_limit: 512M

mysql_cnf:
  innodb_buffer_pool_size: 256M
```

Both use `map[string]string` — keys are the setting name, values are the setting value.

## Merge Order

```
Hardcoded dev defaults → Global (~/.dev/config.yaml) → Per-project (.dev.yaml)
```

Per-project wins over global, global wins over hardcoded defaults.

### Protected Keys

Some keys are forced after the merge and cannot be overridden by users:

- **my.cnf:** `ssl = 0`

Protected keys are applied last, overwriting any user-provided value.

## Hardcoded Dev-Optimized Defaults

### PHP ini

| Setting | Value | Reason |
|---------|-------|--------|
| `error_reporting` | `E_ALL` | Show all errors, warnings, notices, deprecations |
| `display_errors` | `On` | Render errors in browser output |
| `display_startup_errors` | `On` | Show PHP startup errors |
| `log_errors` | `On` | Also log to file |
| `memory_limit` | `512M` | Headroom for Composer, tests, static analysis |
| `max_execution_time` | `300` | Long-running migrations, Xdebug step-through |
| `max_input_time` | `300` | Match execution time for large uploads |
| `upload_max_filesize` | `256M` | Remove "file too large" friction |
| `post_max_size` | `256M` | Must be >= upload_max_filesize |
| `max_input_vars` | `5000` | Large forms in CMS/admin panels |
| `opcache.enable` | `1` | Keep OPcache on for realistic performance |
| `opcache.revalidate_freq` | `0` | Check timestamps every request — instant code pickup |
| `opcache.validate_timestamps` | `1` | Required for revalidate_freq=0 |
| `opcache.max_accelerated_files` | `20000` | Large frameworks + vendor exceed the 10000 default |
| `opcache.memory_consumption` | `256` | Prevent "OPcache buffer full" restarts |
| `opcache.interned_strings_buffer` | `32` | Frameworks generate many interned strings |
| `realpath_cache_size` | `4096k` | Reduce filesystem stat calls for large vendor dirs |
| `realpath_cache_ttl` | `600` | 10 minutes — files don't move often during dev |
| `session.gc_maxlifetime` | `86400` | 24-hour sessions for long dev days |
| `date.timezone` | `UTC` | Predictable baseline, avoids "timezone not set" warnings |
| `zend.assertions` | `1` | Compile and execute assertions in dev |

### MySQL cnf

| Setting | Value | Reason |
|---------|-------|--------|
| `skip-log-bin` | *(flag)* | No replication needed; saves disk I/O |
| `innodb_flush_log_at_trx_commit` | `0` | Flush once/sec instead of every commit — massive write speedup |
| `innodb_flush_method` | `O_DIRECT` | Avoid double-buffering with OS page cache |
| `innodb_buffer_pool_size` | `512M` | Most dev datasets fit in memory |
| `innodb_log_file_size` | `128M` | Reduce checkpoint frequency during bulk imports |
| `innodb_doublewrite` | `0` | No torn-page protection needed for dev data |
| `innodb_io_capacity` | `2000` | Assume SSD/NVMe — aggressive background flushing |
| `innodb_io_capacity_max` | `4000` | Upper bound for burst I/O |
| `max_connections` | `200` | Multiple projects + workers + tests concurrently |
| `table_open_cache` | `4000` | Large frameworks touch many tables |
| `performance_schema` | `OFF` | Saves ~200-400MB RAM |
| `slow_query_log` | `1` | Catch unintentionally slow queries |
| `long_query_time` | `2` | 2-second threshold — flags genuinely slow queries |
| `host_cache_size` | `0` | Avoid "Host is blocked" errors on container restarts |
| `skip-name-resolve` | *(flag)* | Skip DNS lookups — faster connects in Docker |

### Protected (always forced)

| File | Setting | Value | Reason |
|------|---------|-------|--------|
| my.cnf | `ssl` | `0` | Local dev doesn't need SSL for DB connections |

## Runtime Delivery

Settings are delivered at runtime via mounted config files — no image rebuild required.

### PHP ini

On `dev start`:

1. Merge hardcoded defaults → global → per-project `php_ini` maps
2. Write `~/.dev/php/{version}/conf.d/dev-overrides.ini` with merged settings
3. This directory is already mounted to `/usr/local/etc/php/conf.custom` in the container via `PHP_INI_SCAN_DIR`
4. PHP-FPM picks up the file on container start

The file is written before `docker compose up`, so it's available immediately.

### MySQL cnf

On `dev start` (or `dev services up`):

1. Merge hardcoded defaults → global → per-project `mysql_cnf` maps
2. Apply protected keys (force `ssl=0`)
3. Write `~/.dev/mysql/conf.d/dev-overrides.cnf` with merged settings under `[mysqld]`
4. Mount `~/.dev/mysql/conf.d/` to `/etc/mysql/conf.d/` in the MySQL container (new mount in docker-compose generation)
5. MySQL reads conf.d on container start

### File format

**dev-overrides.ini:**
```ini
; Generated by dev — do not edit manually
memory_limit = 512M
upload_max_filesize = 256M
; ...
```

**dev-overrides.cnf:**
```ini
; Generated by dev — do not edit manually
[mysqld]
skip-log-bin
innodb_flush_log_at_trx_commit = 0
ssl = 0
; ...
```

Flag-style settings (like `skip-log-bin`, `skip-name-resolve`) are written without `=` value.

## Changes Required

| Component | File | Change |
|-----------|------|--------|
| Config structs | `internal/config/config.go` | Add `PhpIni map[string]string` and `MysqlCnf map[string]string` to `ProjectConfig` |
| Global config | `internal/config/global.go` | Add `PhpIni map[string]string` and `MysqlCnf map[string]string` to `GlobalConfig` |
| Config loading | `internal/config/config.go` | Merge global → per-project for both maps in `Load()` |
| Defaults | `internal/config/defaults.go` (new) | Hardcoded dev-optimized defaults for php.ini and my.cnf as `map[string]string` constants. Merge function: `mergeSettings(defaults, global, project, protected) map[string]string` |
| INI writer | `internal/phpimage/ini.go` (new) | `WritePhpIni(version string, settings map[string]string) error` — writes `dev-overrides.ini` to `~/.dev/php/{version}/conf.d/` |
| CNF writer | `internal/docker/cnf.go` (new) | `WriteMysqlCnf(settings map[string]string) error` — writes `dev-overrides.cnf` to `~/.dev/mysql/conf.d/` |
| Docker compose | `internal/docker/docker.go` | Add volume mount for `~/.dev/mysql/conf.d/` → `/etc/mysql/conf.d/` on MySQL service |
| Lifecycle start | `internal/lifecycle/start.go` | Call INI and CNF writers before docker up |
| Existing php.ini | `internal/phpimage/phpimage.go` | Keep the baked-in `php.ini` minimal (just a base). The runtime overrides layer on top. |

## What Doesn't Change

- The existing `php.ini` baked into the Dockerfile stays as-is (provides a base)
- The existing `my.cnf` with `ssl=0` stays baked into the image
- Xdebug toggling via `conf.d/xdebug.ini` is unaffected
- No new CLI commands — settings flow through config files only
