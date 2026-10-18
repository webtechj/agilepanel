#!/bin/bash
set -e

# AgilePanel Automated VPS Installer
# Supported OS: Ubuntu 22.04+, Debian 11+

main() {
    # Restrict file creation mask so any generated files/keys default to root-only
    umask 077

    # Check OS compatibility
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        if [ "$ID" = "ubuntu" ]; then
            VERSION_MAJOR=$(echo $VERSION_ID | cut -d'.' -f1)
            if [ "$VERSION_MAJOR" -lt 22 ]; then
                echo "======================================================================"
                echo "ERROR: Ubuntu $VERSION_ID is not supported."
                echo "Ubuntu 20.04 reached End of Standard Support in May 2025, and"
                echo "the PHP repository (ppa:ondrej/php) has dropped support for Focal."
                echo "Please upgrade your VPS to Ubuntu 22.04 LTS or 24.04 LTS to continue."
                echo "======================================================================"
                exit 1
            fi
        elif [ "$ID" = "debian" ]; then
            VERSION_MAJOR=$(echo $VERSION_ID | cut -d'.' -f1)
            if [ "$VERSION_MAJOR" -lt 11 ]; then
                echo "======================================================================"
                echo "ERROR: Debian $VERSION_ID is not supported."
                echo "Please upgrade your VPS to Debian 11 (Bullseye) or Debian 12 (Bookworm)."
                echo "======================================================================"
                exit 1
            fi
        else
            echo "ERROR: Unsupported OS distribution ($NAME)."
            echo "AgilePanel officially supports Ubuntu 22.04+ and Debian 11+."
            exit 1
        fi
    else
        echo "ERROR: Could not detect operating system type."
        exit 1
    fi

    echo "========================================="
    echo "       INSTALLING AGILEPANEL (ap)        "
    echo "========================================="

    # 0. Ask for Server Admin details (for SSL & configuration)
    if [ -t 0 ]; then
        IS_INTERACTIVE=true
    else
        IS_INTERACTIVE=false
    fi

    if [ -z "$ADMIN_NAME" ]; then
        if [ "$IS_INTERACTIVE" = true ]; then
            read -p "Enter Server Admin Full Name: " ADMIN_NAME
            while [ -z "$ADMIN_NAME" ]; do
                echo "Admin name cannot be empty."
                read -p "Enter Server Admin Full Name: " ADMIN_NAME
            done
        else
            ADMIN_NAME="AgilePanel Admin"
        fi
    fi

    if [ -z "$ADMIN_EMAIL" ]; then
        if [ "$IS_INTERACTIVE" = true ]; then
            read -p "Enter Server Admin Email Address: " ADMIN_EMAIL
            while [ -z "$ADMIN_EMAIL" ]; do
                echo "Admin email cannot be empty."
                read -p "Enter Server Admin Email Address: " ADMIN_EMAIL
            done
        else
            # Leave empty in non-interactive mode so ACME issues certificates anonymously
            # rather than failing on an invalid localhost email (e.g. admin@localhost).
            ADMIN_EMAIL=""
        fi
    fi

    export DEBIAN_FRONTEND=noninteractive

    # Ensure GPG and keyrings directories exist with proper permissions
    mkdir -p ~/.gnupg /etc/apt/keyrings
    chmod 700 ~/.gnupg /etc/apt/keyrings

    # 1. Update system package index
    apt-get update
    apt-get install -y curl gpg gnupg2 dirmngr lsb-release debian-keyring debian-archive-keyring apt-transport-https sudo zip unzip ca-certificates

    # 2. Add Caddy official repository
    curl -fsSL --proto '=https' --tlsv1.2 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor --yes -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
    curl -fsSL --proto '=https' --tlsv1.2 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list

    # 3. Add PHP (Ondřej Surý) repository (for Ubuntu/Debian)
    if [ -f /etc/lsb-release ] || [ -f /etc/debian_version ]; then
        echo "Configuring PHP repository..."
        
        # Check if it is Ubuntu
        if [ -f /etc/lsb-release ] || grep -q "Ubuntu" /etc/issue 2>/dev/null; then
            echo "Adding PHP PPA for Ubuntu..."
            
            # Get Ubuntu codename
            if [ -f /etc/lsb-release ]; then
                . /etc/lsb-release
                CODENAME=$DISTRIB_CODENAME
            else
                CODENAME=$(lsb_release -sc)
            fi
            
            # Import GPG key via pinned full 40-character fingerprint
            if curl -fsSL --proto '=https' --tlsv1.2 'https://keyserver.ubuntu.com/pks/lookup?op=get&search=0x14AA40EC0831756756D7F66C4F4EA0AAE5267A6C' | gpg --dearmor --yes -o /etc/apt/keyrings/ondrej-php.gpg 2>/dev/null; then
                echo "deb [signed-by=/etc/apt/keyrings/ondrej-php.gpg] https://ppa.launchpadcontent.net/ondrej/php/ubuntu ${CODENAME} main" > /etc/apt/sources.list.d/ondrej-php.list
            else
                echo "Fallback: Adding Ondrej PHP PPA via add-apt-repository..."
                apt-get install -y software-properties-common
                add-apt-repository -y ppa:ondrej/php
            fi
        else
            # Debian: fetch GPG key and add source directly instead of piping remote script to bash
            echo "Adding PHP package source for Debian..."
            DEBIAN_CODENAME=$(lsb_release -sc 2>/dev/null || cat /etc/debian_version | cut -d'.' -f1)
            curl -fsSL --proto '=https' --tlsv1.2 https://packages.sury.org/php/apt.gpg | gpg --dearmor --yes -o /etc/apt/keyrings/ondrej-php.gpg
            echo "deb [signed-by=/etc/apt/keyrings/ondrej-php.gpg] https://packages.sury.org/php/ ${DEBIAN_CODENAME} main" > /etc/apt/sources.list.d/ondrej-php.list
        fi
    fi
    apt-get update

    # 3.5. Stop, disable and optionally purge conflicting web servers
    echo "Checking for conflicting web servers (apache2/nginx)..."
    if systemctl list-unit-files 2>/dev/null | grep -q -E "apache2|nginx"; then
        DO_PURGE=false
        if [ "$IS_INTERACTIVE" = true ] && [ -z "$AP_FORCE_PURGE" ]; then
            echo "Conflicting web servers (apache2/nginx) detected on this server."
            read -p "Remove and purge conflicting web servers to clear ports 80/443? [y/N]: " CONFIRM_PURGE
            case "$CONFIRM_PURGE" in
                [yY][eE][sS]|[yY]) DO_PURGE=true ;;
                *) DO_PURGE=false ;;
            esac
        else
            DO_PURGE=true
        fi

        if [ "$DO_PURGE" = true ]; then
            echo "Removing and purging conflicting web servers (apache2/nginx)..."
            systemctl stop apache2 nginx 2>/dev/null || true
            systemctl disable apache2 nginx 2>/dev/null || true
            apt-get purge -y apache2 apache2-utils apache2-bin apache2-data apache2.2-common nginx nginx-common nginx-core nginx-full nginx-light 2>/dev/null || true
            apt-get autoremove -y
        else
            echo "Stopping conflicting web servers without deleting existing configurations..."
            systemctl stop apache2 nginx 2>/dev/null || true
            systemctl disable apache2 nginx 2>/dev/null || true
        fi
    fi

    # Kill any residual process on port 80 or 443
    if command -v fuser &> /dev/null; then
        echo "Clearing residual processes on ports 80/443..."
        fuser -k 80/tcp 2>/dev/null || true
        fuser -k 443/tcp 2>/dev/null || true
    fi

    echo "Configuring firewall rules for HTTP/HTTPS/phpMyAdmin..."
    if command -v ufw &> /dev/null; then
        echo "UFW detected. Allowing ports 22, 80, 443, and 8888..."
        ufw allow 22/tcp || true
        ufw allow 80/tcp || true
        ufw allow 443/tcp || true
        ufw allow 8888/tcp || true
    elif command -v firewall-cmd &> /dev/null; then
        echo "Firewalld detected. Allowing HTTP, HTTPS, and port 8888..."
        firewall-cmd --permanent --add-service=http || true
        firewall-cmd --permanent --add-service=https || true
        firewall-cmd --permanent --add-port=8888/tcp || true
        firewall-cmd --reload || true
    fi

    # 4. Install Core Server Dependencies
    apt-get install -y caddy mariadb-server redis-server php8.3-fpm php8.3-mysql php8.3-redis php8.3-curl php8.3-gd php8.3-mbstring php8.3-xml php8.3-zip php8.3-bcmath php8.3-opcache

    # 5. Install WP-CLI globally with SHA512 integrity verification
    echo "Installing WP-CLI with checksum validation..."
    TMP_WP="/tmp/wp-cli.$$.phar"
    TMP_SHA="/tmp/wp-cli.$$.sha512"
    curl -fsSL --proto '=https' --tlsv1.2 -o "$TMP_WP" https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar
    curl -fsSL --proto '=https' --tlsv1.2 -o "$TMP_SHA" https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar.sha512
    if command -v sha512sum &>/dev/null; then
        EXPECTED_HASH=$(awk '{print $1}' "$TMP_SHA")
        ACTUAL_HASH=$(sha512sum "$TMP_WP" | awk '{print $1}')
        if [ "$EXPECTED_HASH" != "$ACTUAL_HASH" ]; then
            echo "ERROR: WP-CLI sha512 checksum mismatch! Aborting."
            rm -f "$TMP_WP" "$TMP_SHA"
            exit 1
        fi
        echo "WP-CLI: Integrity verified successfully."
    fi
    install -m 0755 -o root -g root "$TMP_WP" /usr/local/bin/wp
    rm -f "$TMP_WP" "$TMP_SHA"

    # 6. Download the compiled AgilePanel binary from GitHub
    echo "Downloading AgilePanel CLI..."
    GITHUB_REPO="webtechj/agilepanel"
    AP_CHANNEL="${AP_CHANNEL:-release}"
    AP_BRANCH="${AP_BRANCH:-development}"

    TMP_AP="/tmp/ap.$$"
    if [ "$AP_CHANNEL" = "branch" ]; then
        echo "Installing from branch: ${AP_BRANCH}"
        curl -fsSL --proto '=https' --tlsv1.2 -o "$TMP_AP" \
            "https://raw.githubusercontent.com/${GITHUB_REPO}/${AP_BRANCH}/ap-linux-amd64"
    else
        echo "Installing latest stable release..."
        curl -fsSL --proto '=https' --tlsv1.2 -o "$TMP_AP" \
            "https://github.com/${GITHUB_REPO}/releases/latest/download/ap-linux-amd64"
    fi

    if [ ! -s "$TMP_AP" ]; then
        echo "ERROR: Failed to download AgilePanel CLI binary (file is empty or download failed)."
        rm -f "$TMP_AP"
        exit 1
    fi
    install -m 0755 -o root -g root "$TMP_AP" /usr/local/bin/ap
    rm -f "$TMP_AP"

    # 7. Initialize default AgilePanel State with root-only permissions
    mkdir -p /etc/agilepanel
    chmod 0700 /etc/agilepanel
    if [ ! -f /etc/agilepanel/state.json ]; then
        cat <<EOF > /etc/agilepanel/state.json
{
  "global": {
    "default_php_version": "8.3",
    "supported_php_versions": ["8.1", "8.2", "8.3"],
    "caddy_path": "/usr/bin/caddy",
    "caddy_config_path": "/etc/caddy/Caddyfile",
    "redis_socket_path": "/var/run/redis/redis-server.sock",
    "admin_name": "${ADMIN_NAME}",
    "admin_email": "${ADMIN_EMAIL}"
  },
  "sites": []
}
EOF
        chmod 0600 /etc/agilepanel/state.json
    fi

    # 8. Run Automatic Server Tuning & Optimization
    echo "Running system optimization..."
    if command -v ap &> /dev/null; then
        ap server tune --admin-name "$ADMIN_NAME" --admin-email "$ADMIN_EMAIL"
    elif [ -f /usr/local/bin/ap ]; then
        /usr/local/bin/ap server tune --admin-name "$ADMIN_NAME" --admin-email "$ADMIN_EMAIL"
    else
        echo "Warning: ap CLI not found. Skipping system tuning."
    fi

    echo "========================================="
    echo "       AGILEPANEL SETUP COMPLETE!        "
    echo "========================================="
    echo "You can now use the 'ap' command."
    echo "Create your first site: ap site create domain.com --wp"
}

main "$@"
