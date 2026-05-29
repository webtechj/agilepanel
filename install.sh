#!/bin/bash
set -e

# AgilePanel Automated VPS Installer
# Supported OS: Ubuntu 22.04+, Debian 11+

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
        ADMIN_EMAIL="admin@localhost"
    fi
fi

# 1. Update system package index
apt-get update && apt-get install -y curl gpg lsb-release debian-keyring debian-archive-keyring apt-transport-https sudo unzip

# 2. Add Caddy official repository
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list

# 3. Add PHP (Ondřej Surý) repository (for Ubuntu/Debian)
if [ -f /etc/lsb-release ] || [ -f /etc/debian_version ]; then
    echo "Configuring PHP repository..."
    apt-get install -y gnupg2 ca-certificates
    
    # Check if it is Ubuntu
    if [ -f /etc/lsb-release ] || grep -q "Ubuntu" /etc/issue 2>/dev/null; then
        echo "Adding PHP PPA for Ubuntu..."
        # Import GPG key securely using keyserver
        gpg --no-default-keyring --keyring /tmp/ondrej.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 4F4EA0AAE5267A6C
        mkdir -p /etc/apt/keyrings
        gpg --no-default-keyring --keyring /tmp/ondrej.gpg --export --yes -o /etc/apt/keyrings/ondrej-php.gpg
        rm -f /tmp/ondrej.gpg
        
        # Get Ubuntu codename
        if [ -f /etc/lsb-release ]; then
            . /etc/lsb-release
            CODENAME=$DISTRIB_CODENAME
        else
            CODENAME=$(lsb_release -sc)
        fi
        
        echo "deb [signed-by=/etc/apt/keyrings/ondrej-php.gpg] http://ppa.launchpad.net/ondrej/php/ubuntu ${CODENAME} main" > /etc/apt/sources.list.d/ondrej-php.list
    else
        # Debian
        echo "Adding PHP package source for Debian..."
        curl -sSL https://packages.sury.org/php/README.txt | bash
    fi
fi
apt-get update

# 3.5. Stop, disable and purge conflicting web servers
echo "Removing and purging conflicting web servers (apache2/nginx) to clear port 80/443..."
if systemctl list-unit-files | grep -q -E "apache2|nginx"; then
    systemctl stop apache2 nginx 2>/dev/null || true
    systemctl disable apache2 nginx 2>/dev/null || true
    apt-get purge -y apache2 apache2-utils apache2-bin apache2-data apache2.2-common nginx nginx-common nginx-core nginx-full nginx-light 2>/dev/null || true
    apt-get autoremove -y
fi

# Kill any residual process on port 80 or 443
if command -v fuser &> /dev/null; then
    echo "Killing any residual processes on ports 80/443..."
    fuser -k 80/tcp || true
    fuser -k 443/tcp || true
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

# 5. Install WP-CLI globally
echo "Installing WP-CLI..."
curl -O https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar
chmod +x wp-cli.phar
mv wp-cli.phar /usr/local/bin/wp

# 6. Download the compiled AgilePanel binary from GitHub
echo "Downloading AgilePanel CLI..."
GITHUB_REPO="webtechj/agilepanel"
AP_VERSION="${AP_VERSION:-main}"
curl -L -o /usr/local/bin/ap "https://raw.githubusercontent.com/${GITHUB_REPO}/${AP_VERSION}/ap-linux-amd64"
chmod +x /usr/local/bin/ap

# 7. Initialize default AgilePanel State
mkdir -p /etc/agilepanel
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
