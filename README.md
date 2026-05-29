# AgilePanel (ap)

AgilePanel is a secure, lightweight, and hyper-fast CLI-based WordPress control panel written in Go. Designed as a modern, high-performance alternative to legacy platforms (like WordOps or Webinoly), AgilePanel manages native **Caddy**, **PHP-FPM**, **MariaDB**, and **Redis** directly on bare-metal Linux without complex web UI overhead.

---

## 🚀 Key Features

- **Dynamic & Beautiful CLI**: Built with ANSI colors and structured box layouts for a modern, professional terminal experience.
- **System Isolation**: Automatically creates isolated system users (`wp_[sanitized_domain]`) for each website.
- **Robust Security**:
  - Automatically configured HTTP/3 (HTTP/1, HTTP/2, and HTTP/3 support).
  - Pre-configured security filters in Caddy (blocking hidden files, preventing PHP execution in `/wp-content/uploads/` recursively, blocking `xmlrpc.php` and `/wp-admin/install.php`).
  - PHP pool hardening (`open_basedir` locks, disabled dangerous system functions like `exec`, `shell_exec`, etc.).
  - Unique database & user prefixes (e.g. `db_prefix_`) and random WordPress table prefixes (e.g. `wp_prefix_`) to protect against SQL injections and discovery attacks.
- **High Performance Tuning**:
  - Scaling database buffer pools to 30% of system RAM dynamically.
  - Custom transactional database commit configs (`innodb_flush_log_at_trx_commit = 2`, `O_DIRECT`).
  - Automatic response compression (`encode gzip zstd`) and Redis UNIX socket caching.
  - Swap file creation (2GB swapfile + kernel sysctl pressure optimizations) on resource-constrained servers.
- **On-Demand phpMyAdmin**: Install phpMyAdmin on-demand and hide it behind a custom global port (`IP:8888`) guarded by bcrypt-based HTTP Basic Authentication.
- **Non-Destructive Repairs**: Run `ap repair` to restore all system configurations and pools without disturbing databases or site directories.
- **Auto-Sync Scanning**: Scan `/var/www/` for pre-existing website directories and import them back into `state.json` automatically, extracting database details from `wp-config.php`.
- **Easy Maintenance**: Single-command system package updates/upgrades and CLI self-updates.

---

## 📥 Installation

Ensure your server is running a clean install of **Ubuntu 22.04+ LTS** or **Debian 11+**.

### Standard Installation (Latest Version)
Run the one-liner installer script:
```bash
curl -sSL https://raw.githubusercontent.com/webtechj/agilepanel/main/install.sh | sudo bash
```

### Installation from Specific Release (e.g. v0.8)
```bash
curl -sSL https://raw.githubusercontent.com/webtechj/agilepanel/v0.8/install.sh | AP_VERSION=v0.8 sudo -E bash
```

*The installer will ask for the administrator name and email to configure SSL registration and default admin details.*

---

## 🛠️ CLI Commands & Usage

AgilePanel is controlled entirely via the `ap` executable.

### 🌐 Site Management

#### Create a Site
Create a standard PHP or WordPress site with Redis cache enabled:
```bash
ap site create example.com --php=8.3 --wp
```
*If `--wp` is set, it will prompt for the WordPress admin details (Full Name, Username, Email) and output a secure password along with database credentials.*

#### Delete a Site
Deletes the database, system user/group, PHP configuration pools, public webroot directory, and Caddyfile bindings:
```bash
ap site delete example.com
```

#### List Sites
Renders a grid displaying all domain names, PHP versions, database namespaces, and locks:
```bash
ap site list
```

#### Site Information
Displays comprehensive settings, directory locations, database credentials, and Caddy ACME SSL certificate renew details:
```bash
ap site info example.com
```

#### Lock/Unlock a Site
Changes site folders to read-only/immutable (using system attributes) or unlocks them for updates:
```bash
ap site lock example.com
ap site unlock example.com
```

#### Cache Cleaning
Flushes static Caddy page caches, Redis object caches, bytecode OPcaches, and WordPress transient states:
```bash
ap site cache-clean example.com
```

#### Edit Configuration
Opens the site's PHP-FPM configuration pool inside the system's text editor (respecting `EDITOR` variable) and reloads FPM upon exit:
```bash
ap site edit example.com
```

#### Reinstall WordPress
Recreates database schemas, resets public directories, and reinstalls WordPress core files using the existing site config:
```bash
ap site reinstall example.com
```

#### SSL Force Renewal
Bypasses local certificates cache and forces Caddy to request a fresh SSL certificate:
```bash
ap site ssl-renew example.com
```

#### Backup Database
Exports a secure database SQL dump file into the site's private backup directory:
```bash
ap site backup-db example.com
```

#### Manual Site Backup
Generates separate manual ZIP backups of both public WordPress files and MariaDB database schemas:
```bash
ap site backup example.com
```
*These files are saved inside `/var/www/example.com/backup/` and can be immediately downloaded securely via any SFTP/FTP client using the system user credentials.*

---

### 🖥️ Server Administration

#### Status Monitoring
Monitor CPU, RAM usage, active website counts, and the operational status of Caddy, MariaDB, Redis, and active PHP-FPM pool versions:
```bash
ap server status
```

#### Configure Basic Authentication
Setup global Basic Auth credentials to secure phpMyAdmin and backend administrator utilities:
```bash
ap server auth [username] [password]
```

#### Service Management
Restart global services:
```bash
ap server restart caddy
ap server restart php8.3-fpm
ap server restart mariadb
ap server restart redis
ap server restart all
```

#### Optimization Tuning
Re-run resource audits to tune MySQL buffers, Redis sockets, and Swap:
```bash
ap server tune
```

---

### 🔧 Tools & Maintenance

#### Install phpMyAdmin
Downloads, configures, and secures phpMyAdmin dynamically:
```bash
ap tool install phpmyadmin
```
*Once installed, access it at `http://[your-server-ip]:8888` using the credentials configured via `ap server auth`.*

#### Repair Installation
Runs a non-destructive audit to rebuild all configurations, re-verify system optimizations, rewrite Caddyfile directories, re-create PHP pools, and reload services:
```bash
ap repair
```

#### System Update
Updates system apt repositories and self-updates the `ap` executable to the latest codebase:
```bash
ap update
```

#### System Upgrade
Upgrades all OS system packages and re-runs `ap repair` to ensure configurations align with upgraded software:
```bash
ap upgrade
```

#### Configuration Sync
Synchronizes all server services and scans `/var/www/` to auto-detect and import pre-existing site installations:
```bash
ap sync
```
