# dev

A fast, lightweight PHP development environment for Linux and macOS. Uses shared Docker containers (PHP-FPM, Caddy, MySQL, Redis) to keep RAM usage minimal across many projects. Only requires Docker on the host.

## Why

When working on multiple branches simultaneously via git worktrees, traditional container-per-project setups eat RAM fast (~6 GB each). `dev` takes a different approach:

- **One PHP-FPM container per version** — shared across all projects (~150 MB each)
- **One Caddy container** — reverse proxy for all `*.test` domains with automatic local HTTPS (~15 MB)
- **Shared database/cache containers** — one MySQL, Redis instance instead of one per project

**RAM comparison for 5 projects:**

| | Container-per-project | dev |
|---|---|---|
| Web/PHP | 5 x ~4 GB | ~300 MB total |
| MySQL | 5 x ~500 MB | ~500 MB total |
| Redis | 5 x ~10 MB | ~10 MB total |
| **Total** | **~25 GB** | **~1.5 GB** |

## Requirements

| | Linux/WSL2 | macOS |
|---|---|---|
| **Docker** | [Docker Engine](https://docs.docker.com/engine/install/) | [OrbStack](https://orbstack.dev/) (recommended) or [Docker Desktop](https://www.docker.com/products/docker-desktop/) |
| **Go** | 1.25+ (for building from source) | 1.25+ |

> **OrbStack** is recommended on macOS for significantly better performance and lower resource usage compared to Docker Desktop.

## Getting Started

### 1. Install

```bash
git clone https://github.com/XBS-Nathan/nova.git
cd nova
go build -o dev .

# Add to PATH
sudo mv dev /usr/local/bin/
```

### 2. Set up a project

```bash
cd /path/to/your/project
dev init
```

`dev init` walks you through an interactive setup:

```
  dev init — my-project

  ✓ Detected laravel project

  General
  ? Project type (laravel):
  ? Domain (my-project.test):

  Language
  ? PHP version (8.2):
  ? Node version (22):
  ? Package manager (npm/yarn/pnpm):

  Database
  ? Driver (mysql):
  ? Version (8.0):
  ? Name (my_project):

  ✓ Created .dev.yaml
  Run dev start to get going.
```

### 3. Start the project

```bash
dev start
```

First run builds the PHP Docker image (takes a minute), then starts everything. Subsequent starts are instant.

### 4. Trust HTTPS (one-time)

```bash
dev trust
```

`dev start` will remind you if you haven't done this yet.

### WSL2

`dev trust` automatically detects WSL2 and:
- Installs the Caddy CA cert in both Linux and Windows trust stores
- `dev start` adds hosts entries to both `/etc/hosts` and the Windows hosts file

## Commands

| Command | Description |
|---------|-------------|
| **Setup** | |
| `dev init` | Create a `.dev.yaml` config for the current project (interactive) |
| `dev trust` | Trust the Caddy local CA certificate for HTTPS |
| `dev build` | Force rebuild PHP images |
| **Lifecycle** | |
| `dev start` | Start the current project |
| `dev stop` | Stop the current project |
| `dev restart` | Stop + start |
| `dev down` | Stop all containers |
| **Run commands** | |
| `dev artisan [args]` | Run `php artisan` inside the PHP container (Laravel) |
| `dev composer [args]` | Run `composer` inside the PHP container |
| `dev exec [command...]` | Run any command in the project's PHP container |
| **Database** | |
| `dev snapshot [name]` | Create a database snapshot |
| `dev snapshot restore [name]` | Restore from a snapshot (latest if no name) |
| `dev snapshot list` | List available snapshots |
| **Debugging** | |
| `dev logs [service]` | Stream container logs (all or specific service) |
| `dev xdebug on/off` | Toggle Xdebug (sub-second, no container restart) |
| `dev info` | Show project URL, PHP version, DB, service status |
| **Config** | |
| `dev use php <version>` | Set the PHP version for this project |
| `dev use node <version>` | Set the Node version for this project |
| `dev use db <mysql\|postgres>` | Set the database driver |
| **Other** | |
| `dev share` | Share via Cloudflare Tunnel or ngrok |
| `dev services up/down` | Start/stop shared Docker services |

### Shell completions

```bash
# Bash
dev completion bash > ~/.local/share/bash-completion/completions/dev

# Zsh
dev completion zsh > ~/.zsh/completions/_dev

# Fish
dev completion fish > ~/.config/fish/completions/dev.fish
```

## Configuration

### Per-project: `.dev.yaml`

Created by `dev init` or manually. Everything is optional — sensible defaults are used.

```yaml
# Project type (auto-detected: laravel or generic)
type: laravel

# Domain (default: <project-name>.test)
domain: my-project.test

# PHP version (default: 8.2)
php: "8.2"

# Node.js version (default: 22)
node: "22"

# Package manager: npm, yarn, or pnpm (default: npm)
package_manager: yarn

# Database driver: mysql or postgres (default: mysql)
db_driver: mysql
db_version: "8.0"
db: my_project

# Redis version
redis_version: latest

# PHP extensions (added to the shared PHP image)
extensions:
  - imagick
  - swoole

# Extra ports routed through Caddy (e.g. webpack HMR)
ports:
  - "8080"

# Node.js command to run on dev start
node_command: yarn run hot

# Hooks run inside the PHP container after start/stop
hooks:
  post-start:
    - "php artisan horizon &"
  post-stop: []

# PHP ini overrides (layered on top of dev-optimized defaults)
php_ini:
  memory_limit: 1G
  upload_max_filesize: 500M

# MySQL cnf overrides (layered on top of dev-optimized defaults)
mysql_cnf:
  innodb_buffer_pool_size: 1G
  max_connections: 500

# Per-project Docker services (only run when this project is started)
services:
  typesense:
    image: typesense/typesense:26.0
    ports:
      - "8108:8108"
    environment:
      TYPESENSE_API_KEY: dev
    volumes:
      - typesense_data:/data
    command: "--data-dir /data --enable-cors"

# Shared services (run once, shared across all projects)
shared_services:
  meilisearch:
    image: getmeili/meilisearch:v1.6
    ports:
      - "7700:7700"
    environment:
      MEILI_NO_ANALYTICS: "true"
```

### Global: `~/.dev/config.yaml`

Optional. Created automatically with defaults on first `dev start`.

```yaml
# Parent directory mounted into containers (default: ~/Projects)
projects_dir: ~/Projects

# Service image versions
versions:
  mysql: "8.0"
  redis: latest
  mailpit: latest

# PHP ini overrides (apply to all projects)
php_ini:
  memory_limit: 1G

# MySQL cnf overrides (apply to all projects)
mysql_cnf:
  innodb_buffer_pool_size: 1G
```

### Directory structure

```
~/.dev/
├── caddy/
│   ├── Caddyfile              # Main config (auto-generated)
│   ├── data/                  # Caddy CA certificates
│   └── sites/                 # Per-project site configs
├── dockerfiles/
│   └── php/
│       └── 8.2/
│           ├── Dockerfile     # Generated from extensions
│           └── php.ini
├── mysql/
│   └── conf.d/
│       └── dev-overrides.cnf  # Generated MySQL settings
├── php/
│   └── 8.2/
│       └── conf.d/
│           ├── dev-overrides.ini  # Generated PHP settings
│           └── xdebug.ini     # Written by dev xdebug on
├── docker-compose.yml         # Generated dynamically
├── config.yaml                # Global config
└── snapshots/                 # Database snapshots
```

## Shared Services

By default, MySQL, Redis, and Mailpit are shared across all projects. You can also define custom shared services in any project's `.dev.yaml` under `shared_services:`. Unlike per-project `services:` (which only run when that project is started), shared services run once and are available to all projects.

```bash
# Start all shared services (MySQL, Redis, Mailpit, + custom)
dev services up

# Stop all shared services
dev services down
```

When you run `dev services up`, it scans every `.dev.yaml` in your projects directory and collects all `shared_services` definitions. If multiple projects define the same service name, the first definition wins — they share a single container.

This is useful for services like search engines, message queues, or monitoring tools that multiple projects depend on.

## How it works

```
Browser → project.test → /etc/hosts → 127.0.0.1
                                          ↓
                                    Caddy (Docker, ports 80/443)
                                          ↓
                                    PHP-FPM (Docker, per-version)
                                          ↓
                                MySQL / Redis (Docker)
```

1. **DNS**: `dev start` adds `127.0.0.1 project.test` to `/etc/hosts` (+ Windows hosts on WSL2)
2. **Caddy**: Routes each `*.test` domain to the correct PHP-FPM container. Automatic local HTTPS via built-in CA.
3. **PHP-FPM**: One container per PHP version with Node.js included. Extensions configurable per project. Xdebug toggled via mounted ini + FPM reload signal.
4. **Docker Compose**: Generated dynamically — only starts services the project needs.

## Database Snapshots

Snapshots use parallel tools for speed when available:

| Driver | Snapshot | Restore | Fallback |
|--------|----------|---------|----------|
| MySQL | `mydumper` (4 threads) | `myloader` (4 threads) | `mysqldump`/`mysql` + gzip |
| Postgres | `pg_dump -Fd -j4` (lz4) | `pg_restore -j4` | -- (built-in) |

You can also drop `.sql` or `.sql.gz` files into `~/.dev/snapshots/<db_name>/` and restore them with `dev snapshot restore <filename>`.

## Building from source

```bash
# Requires Go 1.25+
go build -o dev .
```

### Run tests

```bash
go test ./...
```

## License

MIT
