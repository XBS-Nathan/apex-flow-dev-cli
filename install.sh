#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# dev-cli installer
# Installs PHP, Caddy, mkcert, Docker services, and the dev CLI
# Supports: Ubuntu/WSL2 and macOS
# ─────────────────────────────────────────────────────────────────────────────

DEV_DIR="$HOME/.dev"
REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
PHP_VERSIONS=("8.2" "8.1") # Add versions as needed
DEFAULT_PHP="8.2"
NODE_VERSION="22"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}→${NC} $*"; }
ok()    { echo -e "${GREEN}✓${NC} $*"; }
warn()  { echo -e "${YELLOW}!${NC} $*"; }
fail()  { echo -e "${RED}✗${NC} $*"; exit 1; }

# ─────────────────────────────────────────────────────────────────────────────
# Platform detection
# ─────────────────────────────────────────────────────────────────────────────

OS="$(uname -s)"
IS_WSL=false
if [[ "$OS" == "Linux" ]] && grep -qi microsoft /proc/version 2>/dev/null; then
    IS_WSL=true
fi

case "$OS" in
    Linux)  PLATFORM="linux" ;;
    Darwin) PLATFORM="macos" ;;
    *)      fail "Unsupported platform: $OS" ;;
esac

echo ""
echo "  dev-cli installer"
echo "  Platform: $PLATFORM $(${IS_WSL} && echo '(WSL2)' || echo '')"
echo ""

# ─────────────────────────────────────────────────────────────────────────────
# Package manager helpers
# ─────────────────────────────────────────────────────────────────────────────

apt_install() {
    sudo apt-get install -y "$@" > /dev/null 2>&1
}

brew_install() {
    brew install "$@" 2>/dev/null
}

# ─────────────────────────────────────────────────────────────────────────────
# Step 1: System dependencies
# ─────────────────────────────────────────────────────────────────────────────

install_system_deps() {
    info "Installing system dependencies..."

    if [[ "$PLATFORM" == "linux" ]]; then
        sudo apt-get update -qq
        apt_install curl wget git unzip software-properties-common \
            apt-transport-https ca-certificates gnupg lsb-release gzip
    else
        if ! command -v brew &>/dev/null; then
            fail "Homebrew is required on macOS. Install from https://brew.sh"
        fi
    fi

    ok "System dependencies"
}

# ─────────────────────────────────────────────────────────────────────────────
# Step 2: PHP
# ─────────────────────────────────────────────────────────────────────────────

install_php() {
    info "Installing PHP versions: ${PHP_VERSIONS[*]}..."

    if [[ "$PLATFORM" == "linux" ]]; then
        # Add Ondrej PPA for multiple PHP versions
        if ! grep -q "ondrej/php" /etc/apt/sources.list.d/* 2>/dev/null; then
            sudo add-apt-repository -y ppa:ondrej/php > /dev/null 2>&1
            sudo apt-get update -qq
        fi

        for version in "${PHP_VERSIONS[@]}"; do
            info "  Installing PHP $version..."
            apt_install \
                "php${version}-fpm" \
                "php${version}-cli" \
                "php${version}-mysql" \
                "php${version}-pgsql" \
                "php${version}-sqlite3" \
                "php${version}-redis" \
                "php${version}-curl" \
                "php${version}-gd" \
                "php${version}-mbstring" \
                "php${version}-xml" \
                "php${version}-zip" \
                "php${version}-bcmath" \
                "php${version}-intl" \
                "php${version}-soap" \
                "php${version}-xdebug"

            # Disable Xdebug by default (enable with dev xdebug on)
            sudo phpdismod -v "$version" xdebug 2>/dev/null || true

            # Start PHP-FPM
            sudo systemctl enable "php${version}-fpm" 2>/dev/null || true
            sudo systemctl start "php${version}-fpm" 2>/dev/null || true

            ok "  PHP $version"
        done
    else
        for version in "${PHP_VERSIONS[@]}"; do
            info "  Installing PHP $version..."
            brew_install "php@${version}"

            # Start PHP-FPM via Homebrew services
            brew services start "php@${version}" 2>/dev/null || true

            ok "  PHP $version"
        done
    fi

    ok "PHP installed"
}

# ─────────────────────────────────────────────────────────────────────────────
# Step 3: Composer
# ─────────────────────────────────────────────────────────────────────────────

install_composer() {
    if command -v composer &>/dev/null; then
        ok "Composer already installed"
        return
    fi

    info "Installing Composer..."

    if [[ "$PLATFORM" == "linux" ]]; then
        curl -sS https://getcomposer.org/installer | sudo php -- --install-dir=/usr/local/bin --filename=composer
    else
        brew_install composer
    fi

    ok "Composer"
}

# ─────────────────────────────────────────────────────────────────────────────
# Step 4: Node.js
# ─────────────────────────────────────────────────────────────────────────────

install_node() {
    info "Installing Node.js $NODE_VERSION..."

    if command -v nvm &>/dev/null || [[ -f "$HOME/.nvm/nvm.sh" ]]; then
        source "$HOME/.nvm/nvm.sh" 2>/dev/null || true
        nvm install "$NODE_VERSION" 2>/dev/null
        nvm alias default "$NODE_VERSION" 2>/dev/null
    else
        # Install nvm
        curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash
        export NVM_DIR="$HOME/.nvm"
        source "$NVM_DIR/nvm.sh"
        nvm install "$NODE_VERSION"
        nvm alias default "$NODE_VERSION"
    fi

    # Install yarn
    npm install -g yarn 2>/dev/null

    ok "Node.js $NODE_VERSION + yarn"
}

# ─────────────────────────────────────────────────────────────────────────────
# Step 5: Caddy
# ─────────────────────────────────────────────────────────────────────────────

install_caddy() {
    if command -v caddy &>/dev/null; then
        ok "Caddy already installed"
        return
    fi

    info "Installing Caddy..."

    if [[ "$PLATFORM" == "linux" ]]; then
        sudo apt-get install -y debian-keyring debian-archive-keyring apt-transport-https > /dev/null 2>&1
        curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg 2>/dev/null
        curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list > /dev/null
        sudo apt-get update -qq
        apt_install caddy

        # Stop the default Caddy systemd service — we manage it ourselves
        sudo systemctl stop caddy 2>/dev/null || true
        sudo systemctl disable caddy 2>/dev/null || true
    else
        brew_install caddy
    fi

    ok "Caddy"
}

# ─────────────────────────────────────────────────────────────────────────────
# Step 6: mkcert (local HTTPS)
# ─────────────────────────────────────────────────────────────────────────────

install_mkcert() {
    if command -v mkcert &>/dev/null; then
        ok "mkcert already installed"
    else
        info "Installing mkcert..."

        if [[ "$PLATFORM" == "linux" ]]; then
            apt_install libnss3-tools
            curl -Lo /tmp/mkcert "https://dl.filippo.io/mkcert/latest?for=linux/amd64"
            chmod +x /tmp/mkcert
            sudo mv /tmp/mkcert /usr/local/bin/mkcert
        else
            brew_install mkcert nss
        fi

        ok "mkcert"
    fi

    info "Installing local CA..."
    mkcert -install
    ok "Local CA installed"

    if $IS_WSL; then
        warn ""
        warn "WSL2 detected: You also need to trust the CA on Windows."
        warn "Run these steps on the WINDOWS side:"
        warn "  1. Find the CA cert: $(mkcert -CAROOT)/rootCA.pem"
        warn "  2. Copy it to Windows"
        warn "  3. Double-click → Install Certificate → Local Machine → Trusted Root Certification Authorities"
        warn ""
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Step 7: Docker
# ─────────────────────────────────────────────────────────────────────────────

install_docker() {
    if command -v docker &>/dev/null; then
        ok "Docker already installed"
        return
    fi

    info "Installing Docker..."

    if [[ "$PLATFORM" == "linux" ]]; then
        # Docker official install script
        curl -fsSL https://get.docker.com | sudo sh
        sudo usermod -aG docker "$USER"
        warn "You may need to log out and back in for Docker group to take effect"
    else
        fail "Install Docker Desktop from https://www.docker.com/products/docker-desktop/"
    fi

    ok "Docker"
}

# ─────────────────────────────────────────────────────────────────────────────
# Step 8: DNS resolution for *.test
# ─────────────────────────────────────────────────────────────────────────────

setup_dns() {
    info "Setting up DNS for *.test..."

    if [[ "$PLATFORM" == "macos" ]]; then
        # Use dnsmasq on macOS
        brew_install dnsmasq

        mkdir -p /usr/local/etc/dnsmasq.d
        echo "address=/.test/127.0.0.1" > /usr/local/etc/dnsmasq.d/test.conf

        sudo brew services start dnsmasq 2>/dev/null || brew services start dnsmasq

        # Tell macOS to use dnsmasq for .test
        sudo mkdir -p /etc/resolver
        echo "nameserver 127.0.0.1" | sudo tee /etc/resolver/test > /dev/null

        ok "DNS: dnsmasq resolving *.test → 127.0.0.1"

    elif $IS_WSL; then
        warn ""
        warn "WSL2: DNS for *.test must be configured on the Windows side."
        warn "Install Acrylic DNS Proxy on Windows:"
        warn "  1. Download from https://mayakron.altervista.org/support/acrylic/Home.htm"
        warn "  2. Install and open AcrylicHosts.txt"
        warn "  3. Add this line:  127.0.0.1  *.test"
        warn "  4. Set your Windows DNS to 127.0.0.1 (network adapter settings)"
        warn "  5. Restart Acrylic DNS service"
        warn ""

    else
        # Standalone Linux (not WSL) — use dnsmasq
        apt_install dnsmasq

        echo "address=/.test/127.0.0.1" | sudo tee /etc/dnsmasq.d/test.conf > /dev/null

        # Prevent systemd-resolved conflicts
        if systemctl is-active --quiet systemd-resolved; then
            sudo mkdir -p /etc/systemd/resolved.conf.d
            cat <<'RESOLVED' | sudo tee /etc/systemd/resolved.conf.d/dev-cli.conf > /dev/null
[Resolve]
DNS=127.0.0.1
Domains=~test
RESOLVED
            sudo systemctl restart systemd-resolved
        fi

        sudo systemctl enable dnsmasq
        sudo systemctl restart dnsmasq

        ok "DNS: dnsmasq resolving *.test → 127.0.0.1"
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Step 9: Create ~/.dev directory structure
# ─────────────────────────────────────────────────────────────────────────────

setup_dev_dir() {
    info "Setting up $DEV_DIR..."

    mkdir -p "$DEV_DIR"/{caddy/sites,logs,snapshots}

    # Write the main Caddyfile
    cat > "$DEV_DIR/caddy/Caddyfile" <<EOF
import $DEV_DIR/caddy/sites/*.caddy
EOF

    # Write the shared docker-compose.yml (only if it doesn't exist)
    if [[ ! -f "$DEV_DIR/docker-compose.yml" ]]; then
        cat > "$DEV_DIR/docker-compose.yml" <<'COMPOSE'
services:
  mysql:
    image: mysql:8.0
    restart: unless-stopped
    ports:
      - "3306:3306"
    environment:
      MYSQL_ROOT_PASSWORD: root
    volumes:
      - mysql_data:/var/lib/mysql

  redis:
    image: redis:8
    restart: unless-stopped
    ports:
      - "6379:6379"
    command: ["redis-server", "--appendonly", "yes"]
    volumes:
      - redis_data:/data

  typesense:
    image: typesense/typesense:26.0
    restart: unless-stopped
    ports:
      - "8108:8108"
    environment:
      TYPESENSE_API_KEY: dev
    command: "--data-dir /data --enable-cors"
    volumes:
      - typesense_data:/data

  docuseal:
    image: docuseal/docuseal:latest
    restart: unless-stopped
    ports:
      - "3000:3000"
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/docuseal
    volumes:
      - docuseal_data:/data/docuseal

  postgres:
    image: postgres:15
    restart: unless-stopped
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: docuseal
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 5
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  mysql_data:
  redis_data:
  typesense_data:
  docuseal_data:
  postgres_data:
COMPOSE
    fi

    ok "Directory structure at $DEV_DIR"
}

# ─────────────────────────────────────────────────────────────────────────────
# Step 10: Build and install the dev binary
# ─────────────────────────────────────────────────────────────────────────────

install_dev_cli() {
    info "Building dev CLI..."

    # Check for Go
    export PATH="$PATH:$HOME/go-sdk/go/bin:$HOME/go/bin:/usr/local/go/bin"
    if ! command -v go &>/dev/null; then
        info "  Installing Go..."
        if [[ "$PLATFORM" == "linux" ]]; then
            wget -qO /tmp/go.tar.gz "https://go.dev/dl/go1.24.2.linux-amd64.tar.gz"
            if [[ -w /usr/local ]]; then
                sudo tar -C /usr/local -xzf /tmp/go.tar.gz
                export PATH="$PATH:/usr/local/go/bin"
            else
                mkdir -p "$HOME/go-sdk"
                tar -C "$HOME/go-sdk" -xzf /tmp/go.tar.gz
                export PATH="$PATH:$HOME/go-sdk/go/bin"
            fi
            rm /tmp/go.tar.gz
        else
            brew_install go
        fi
        ok "  Go installed"
    fi

    # Build
    cd "$REPO_DIR"
    go build -o dev .

    # Install to ~/bin (or /usr/local/bin)
    mkdir -p "$HOME/bin"
    cp dev "$HOME/bin/dev"
    chmod +x "$HOME/bin/dev"

    # Ensure ~/bin is in PATH
    if ! echo "$PATH" | grep -q "$HOME/bin"; then
        SHELL_RC="$HOME/.bashrc"
        [[ -f "$HOME/.zshrc" ]] && SHELL_RC="$HOME/.zshrc"
        echo 'export PATH="$HOME/bin:$PATH"' >> "$SHELL_RC"
        export PATH="$HOME/bin:$PATH"
        warn "Added ~/bin to PATH in $SHELL_RC — restart your shell or run: source $SHELL_RC"
    fi

    ok "dev CLI installed at $HOME/bin/dev"
}

# ─────────────────────────────────────────────────────────────────────────────
# Step 11: Shell completions
# ─────────────────────────────────────────────────────────────────────────────

setup_completions() {
    info "Setting up shell completions..."

    # Detect shell
    SHELL_NAME="$(basename "$SHELL")"
    case "$SHELL_NAME" in
        bash)
            mkdir -p "$HOME/.local/share/bash-completion/completions"
            "$HOME/bin/dev" completion bash > "$HOME/.local/share/bash-completion/completions/dev"
            ok "Bash completions installed"
            ;;
        zsh)
            mkdir -p "$HOME/.zsh/completions"
            "$HOME/bin/dev" completion zsh > "$HOME/.zsh/completions/_dev"
            warn "Add this to your .zshrc if not already present: fpath=(~/.zsh/completions \$fpath)"
            ok "Zsh completions installed"
            ;;
        *)
            warn "Shell completions not set up for $SHELL_NAME — run: dev completion --help"
            ;;
    esac
}

# ─────────────────────────────────────────────────────────────────────────────
# Run it
# ─────────────────────────────────────────────────────────────────────────────

install_system_deps
install_php
install_composer
install_node
install_caddy
install_mkcert
install_docker
setup_dns
setup_dev_dir
install_dev_cli
setup_completions

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  Installation complete!${NC}"
echo ""
echo "  Quick start:"
echo "    cd /path/to/your/project"
echo "    dev start"
echo ""
echo "  Commands:"
echo "    dev start       Start the current project"
echo "    dev stop        Stop the current project"
echo "    dev php         Run PHP with correct version"
echo "    dev composer    Run composer with correct PHP"
echo "    dev snapshot    Snapshot the database"
echo "    dev info        Show project info"
echo "    dev services    Manage shared Docker services"
echo "    dev down        Stop everything"
echo ""

if $IS_WSL; then
    echo -e "${YELLOW}  Windows-side setup required:${NC}"
    echo "    1. Install Acrylic DNS Proxy for *.test resolution"
    echo "    2. Trust the mkcert CA in Windows cert store"
    echo "    See above for detailed instructions."
    echo ""
fi

echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
