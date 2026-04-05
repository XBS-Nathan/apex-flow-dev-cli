# Docker-Based Stack Refactor

**Date:** 2026-04-05
**Status:** Approved

## Goal

Refactor `dev` CLI from host-native PHP/Caddy to a fully Docker-based stack. The only host dependency becomes Docker itself. This eliminates OS-specific install complexity (Ondrej PPA, Homebrew PHP, dnsmasq, mkcert) and gives every developer an identical environment.

## Architecture

### Container Stack

All services run in a single `docker-compose.yml` managed by the `dev` CLI at `~/.dev/docker-compose.yml`:

| Service | Image | Purpose |
|---------|-------|---------|
| `caddy` | `caddy:2-alpine` | Reverse proxy, auto HTTPS via internal CA |
| `php82` | Built from `~/.dev/dockerfiles/php/8.2/Dockerfile` | PHP-FPM 8.2 |
| `php83` | Built from `~/.dev/dockerfiles/php/8.3/Dockerfile` | PHP-FPM 8.3 |
| `mysql` | `mysql:8.0` | Shared MySQL |
| `redis` | `redis:8` | Shared Redis |
| `typesense` | `typesense/typesense:26.0` | Shared Typesense |
| `postgres` | `postgres:15` | Shared Postgres |

All containers join a `dev` Docker network. Caddy routes to PHP-FPM containers by service name (e.g. `php82:9000`).

### Volume Mounts

- **Project files:** A configurable parent directory (default `~/Projects`) mounted at `/srv` in Caddy and all PHP containers
- **Caddy config:** `~/.dev/caddy` mounted at `/etc/caddy` in the Caddy container
- **Caddy data:** `~/.dev/caddy/data` mounted at `/data` for certificates and CA
- **PHP conf.d:** `~/.dev/php/<version>/conf.d` mounted for Xdebug ini toggling
- **Database volumes:** Named Docker volumes for MySQL, Redis, Typesense, Postgres data

### Docker Compose Generation

The CLI generates `docker-compose.yml` dynamically based on:
- Which PHP versions are needed across all projects
- Which database services are configured
- The global `projects_dir` setting

The compose file is regenerated when a new PHP version is needed. Existing containers are not restarted unless the compose file changes.

## Caddy Configuration

### Site Config

Each project gets a site config at `~/.dev/caddy/sites/<project>.caddy`:

```
myproject.test {
    root * /srv/myproject/public
    php_fastcgi php82:9000
    file_server
    encode gzip
}
```

### Main Caddyfile

```
{
    local_certs
}

import /etc/caddy/sites/*.caddy
```

`local_certs` enables Caddy's built-in CA for automatic local HTTPS.

### Reload

After writing site configs: `docker exec caddy caddy reload --config /etc/caddy/Caddyfile`

## PHP Images

### Dockerfile Template

Shipped per version at `~/.dev/dockerfiles/php/<version>/Dockerfile`:

```dockerfile
FROM php:<version>-fpm-alpine

RUN apk add --no-cache linux-headers $PHPIZE_DEPS \
    && docker-php-ext-install pdo_mysql pdo_pgsql opcache pcntl bcmath \
    && pecl install redis xdebug \
    && docker-php-ext-enable redis \
    && apk del $PHPIZE_DEPS

# Project-specific extensions appended here at build time

COPY php.ini /usr/local/etc/php/php.ini

WORKDIR /srv
```

Xdebug is installed but not enabled in the image. Enabling happens via mounted ini file.

### Extension Management

Projects declare extensions in `.dev.yaml`:

```yaml
php: "8.2"
extensions:
  - imagick
  - swoole
```

Extensions are **unioned** across all projects sharing the same PHP version. If project A needs imagick and project B needs swoole, the single `php82` container gets both.

The CLI computes the union, generates the Dockerfile with the combined extensions, and builds with a tag that includes a hash of the extension list. Rebuilds only happen when the extension set changes.

### Xdebug Toggle

`dev xdebug on`:
1. Writes `~/.dev/php/<version>/conf.d/xdebug.ini`:
   ```ini
   zend_extension=xdebug
   xdebug.mode=debug
   xdebug.client_host=host.docker.internal
   xdebug.start_with_request=yes
   ```
2. Sends `kill -USR2 1` to the PHP-FPM container (graceful reload, no restart)

`dev xdebug off`:
1. Removes the ini file
2. Same reload signal

Sub-second toggle.

## DNS / Hosts Management

### /etc/hosts

`dev start` adds `127.0.0.1 myproject.test` to `/etc/hosts` via `sudo` if the entry is missing. User sees a sudo prompt only on the first start of a new project.

`dev stop` does not remove the entry — it's harmless and avoids needing sudo on every stop.

### WSL2 Support

Detected by checking `/proc/version` for `microsoft` or existence of `/mnt/c/Windows`.

When WSL2 is detected, `dev start` also writes to `/mnt/c/Windows/System32/drivers/etc/hosts`.

`dev trust` on WSL2:
1. Trusts Caddy's CA cert in the Linux trust store
2. Exports the cert and trusts it in the Windows certificate store via `powershell.exe -Command "Import-Certificate ..."`

## Global Config

Stored at `~/.dev/config.yaml`:

```yaml
projects_dir: ~/Projects
php_versions:
  - "8.2"
  - "8.3"
```

`projects_dir` is the parent directory mounted into containers. Defaults to `~/Projects`.

`php_versions` lists available versions. Auto-populated when projects reference new versions.

## CLI Command Changes

### Modified Commands

| Command | Before | After |
|---------|--------|-------|
| `dev start` | Start host PHP-FPM, Caddy, Docker services | Ensure PHP image built, docker compose up, write Caddy config, add hosts entry, create DB |
| `dev stop` | Kill host processes, unlink Caddy | Unlink Caddy config, reload (containers stay running) |
| `dev down` | Stop Caddy + docker compose down | `docker compose down` (stops everything) |
| `dev artisan` | `php8.2 artisan` on host | `docker exec -w /srv/project php82 php artisan` |
| `dev composer` | `php8.2 composer` on host | `docker exec -w /srv/project php82 composer` |
| `dev xdebug` | `phpenmod`/`phpdismod` + systemctl | Write/remove ini + kill signal |

### New Commands

| Command | Purpose |
|---------|---------|
| `dev trust` | Extract Caddy CA cert, install in host (+ Windows on WSL2) trust store |
| `dev build` | Force rebuild PHP images with current extension union |

### Removed Host Dependencies

PHP, Caddy binary, mkcert, dnsmasq, Acrylic, nvm. Only Docker is required on the host.

## Package Changes

### Removed
- `internal/php` — host PHP management no longer needed

### New
- `internal/hosts` — manages `/etc/hosts` entries, WSL2 detection, Windows hosts file
- `internal/phpimage` — builds PHP Docker images from template, manages extension union

### Refactored
- `internal/caddy` — writes site configs to mounted volume, reloads via `docker exec`
- `internal/docker` — generates `docker-compose.yml` dynamically, provides `Exec()` for running commands in containers
- `internal/lifecycle` — updated interfaces for Docker/Caddy/Hosts services

### Unchanged
- `internal/config` — adds `extensions` field to `ProjectConfig`, adds global config loading
- `internal/db` — `Store` interface and MySQL/Postgres adapters unchanged
- `internal/project` — same detection logic

### Updated Interfaces

```go
// lifecycle.DockerService
type DockerService interface {
    Up(phpVersions []string) error
    Down() error
    Exec(service string, args ...string) error
}

// lifecycle.CaddyService
type CaddyService interface {
    Start() error
    Stop() error
    Link(siteName, docroot, phpService string) error
    Unlink(siteName string) error
    Reload() error
}

// lifecycle.HostsService (new)
type HostsService interface {
    Ensure(domain string) error
}
```

## `dev start` Flow

1. `project.Detect()` — find project root
2. `config.LoadGlobal()` — get `projects_dir`
3. `config.Load(projectDir)` — get PHP version, extensions, db_driver
4. `phpimage.EnsureBuilt(version, extensions)` — build image if extensions changed
5. `docker.Up(neededVersions)` — generate compose, start containers
6. `caddy.Link(name, docroot, phpService)` — write site config, reload
7. `hosts.Ensure("myproject.test")` — add `/etc/hosts` entry (+ Windows on WSL2)
8. `db.Store.CreateIfNotExists(dbName)` — create database
9. Run post-start hooks via `docker exec` into PHP container
10. Print URL, PHP version, database info

## `dev stop` Flow

1. Run post-stop hooks via `docker exec`
2. `caddy.Unlink(name)` + reload
3. PHP container stays running (shared)
4. Hosts entry stays (harmless)

## `dev down` Flow

1. `docker compose down` — stops all containers

## Install

1. Build Go binary: `go build -o dev .`
2. Run `dev trust` — extracts Caddy CA cert and installs in host trust store
3. First `dev start` builds PHP images and creates compose file

## Migration Notes

- **Hooks run inside the PHP container.** Existing hooks like `php artisan horizon &` work because PHP is in the container. Host-level commands (e.g. `open http://...`) would not work inside the container. If needed, hooks could support a `host: true` flag in the future, but this is out of scope for the initial refactor.
- **Existing snapshots remain compatible.** The `db` package is unchanged. Snapshots stored at `~/.dev/snapshots/` work as before. The `mydumper`/`myloader` and `pg_dump`/`pg_restore` tools would need to be available on the host (not in a container) since they connect to the database over localhost. This is unchanged from current behavior.
- **Existing `.dev.yaml` files are backward compatible.** The new `extensions` field is optional. All existing fields keep the same defaults.

## Testing Strategy

- `internal/hosts` — unit testable with temp files
- `internal/phpimage` — unit test Dockerfile generation, integration test image builds
- `internal/caddy` — unit test config generation, mock `docker exec` for reload
- `internal/lifecycle` — mock all interfaces (same pattern as current tests)
- `internal/docker` — unit test compose YAML generation
- Integration tests with testcontainers for DB operations (existing, unchanged)
