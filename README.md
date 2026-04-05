# dev

A fast, lightweight PHP development environment for Linux and macOS. Uses native PHP-FPM + Caddy + shared Docker services to keep RAM usage minimal across many projects.

## Why

When working on multiple branches simultaneously via git worktrees, traditional container-per-project setups eat RAM fast (~6 GB each). `dev` takes a different approach:

- **PHP runs natively** via PHP-FPM — one process per version, shared across all projects (~50 MB each)
- **Caddy** handles routing — one process serves all `*.test` domains (~10 MB)
- **Docker only for services** — one shared MySQL, Redis, Typesense instance instead of one per project

**RAM comparison for 5 projects:**

| | Container-per-project | dev |
|---|---|---|
| Web/PHP | 5 x ~4 GB | ~100 MB total |
| MySQL | 5 x ~500 MB | ~500 MB total |
| Redis | 5 x ~10 MB | ~10 MB total |
| **Total** | **~25 GB** | **~1.5 GB** |

## Install

```bash
git clone https://github.com/XBS-Nathan/apex-flow-dev-cli.git
cd dev-cli
./install.sh
```

The install script detects your platform and installs:

| Component | Linux/WSL2 | macOS |
|-----------|-----------|-------|
| PHP versions | Ondrej PPA | Homebrew |
| Caddy | Cloudsmith repo | Homebrew |
| mkcert | Binary download | Homebrew |
| DNS (*.test) | Acrylic (Windows, manual) | dnsmasq |
| Docker | docker.io | Docker Desktop |
| Node.js | nvm | nvm |

### WSL2 Windows-side setup (manual)

1. **DNS**: Install [Acrylic DNS Proxy](https://mayakron.altervista.org/support/acrylic/Home.htm), add `127.0.0.1 *.test` to AcrylicHosts.txt, set Windows DNS to `127.0.0.1`
2. **HTTPS**: Trust the mkcert CA cert in Windows Certificate Store (Trusted Root Certification Authorities)

## Usage

```bash
cd /path/to/your/project
dev start
```

### Commands

| Command | Description |
|---------|-------------|
| `dev start` | Start the current project (services, Caddy, DB, hooks) |
| `dev stop` | Stop the current project |
| `dev restart` | Stop + start |
| `dev down` | Stop all projects and shared services |
| `dev php [args]` | Run PHP with the project's configured version |
| `dev artisan [args]` | Run `php artisan` with the project's PHP version (Laravel) |
| `dev composer [args]` | Run `composer` with the project's PHP version |
| `dev snapshot [name]` | Create a database snapshot |
| `dev snapshot restore [name]` | Restore from a snapshot (latest if no name given) |
| `dev snapshot list` | List available snapshots |
| `dev info` | Show project URL, PHP version, DB, service status |
| `dev xdebug on/off` | Toggle Xdebug for the project's PHP version |
| `dev share` | Share via Cloudflare Tunnel or ngrok |
| `dev use php <version>` | Set the PHP version for this project |
| `dev use node <version>` | Set the Node version for this project |
| `dev use db <mysql\|postgres>` | Set the database driver |
| `dev services up` | Start shared Docker services |
| `dev services down` | Stop shared Docker services |

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

Drop a `.dev.yaml` in your project root to override defaults. Everything is optional — sensible defaults are used when omitted.

```yaml
# PHP version (default: 8.2)
php: "8.1"

# Node version (default: 22)
node: "22"

# Database driver: mysql or postgres (default: mysql)
db_driver: mysql

# Database name (default: derived from directory name)
db: my_project

# MySQL connection (defaults shown)
mysql:
  user: root
  pass: root
  host: 127.0.0.1
  port: "3306"

# PostgreSQL connection (defaults shown)
postgres:
  user: postgres
  pass: postgres
  host: 127.0.0.1
  port: "5432"

# Hooks run after start/stop
hooks:
  post-start:
    - "some-queue-worker &"
    - "yarn run hot &"
  post-stop: []

# Extra per-project Docker services
services:
  mssql:
    image: mcr.microsoft.com/azure-sql-edge:latest
    ports: ["1439:1433"]
    environment:
      MSSQL_SA_PASSWORD: "PASSword123@"
      ACCEPT_EULA: "Y"
```

### Shared services: `~/.dev/docker-compose.yml`

The install script creates this with MySQL 8.0, Redis 8, Typesense 26.0, Docuseal + Postgres. Edit it to add or remove services.

### Directory structure

```
~/.dev/
├── caddy/
│   ├── Caddyfile            # Main config (auto-generated)
│   └── sites/               # Per-project site configs (auto-generated)
├── docker-compose.yml       # Shared services
├── logs/                    # Caddy access logs
└── snapshots/               # Database snapshots
    └── <db_name>/
        └── <db_name>_<timestamp>/
```

## How it works

```
Browser → *.test DNS → Caddy → PHP-FPM (native)
                                  ↓
                          MySQL / Redis / Typesense (shared Docker)
```

1. **DNS**: `*.test` resolves to `127.0.0.1` via dnsmasq (macOS/Linux) or Acrylic (Windows)
2. **Caddy**: Reverse proxy routing each `<project>.test` domain to the correct PHP-FPM socket
3. **PHP-FPM**: Native PHP processes, one pool per version. `dev start` creates a Caddy config pointing to the right socket for the project's PHP version
4. **Docker**: Shared services run once, accessible to all projects on localhost

## Database Snapshots

Snapshots use parallel tools for speed when available:

| Driver | Snapshot | Restore | Fallback |
|--------|----------|---------|----------|
| MySQL | `mydumper` (4 threads) | `myloader` (4 threads) | `mysqldump`/`mysql` + gzip |
| Postgres | `pg_dump -Fd -j4` (lz4) | `pg_restore -j4` | — (built-in) |

You can also drop `.sql` or `.sql.gz` files into `~/.dev/snapshots/<db_name>/` and restore them with `dev snapshot restore <filename>`.

## Building from source

```bash
# Requires Go 1.25+
go build -o dev .

# Cross-compile for macOS
GOOS=darwin GOARCH=arm64 go build -o dev-darwin-arm64 .
GOOS=darwin GOARCH=amd64 go build -o dev-darwin-amd64 .
```

## License

MIT
