# AgilePanel (`ap`) Complete CLI Command Reference

This document provides an exhaustive, production-grade reference for all commands, subcommands, arguments, syntax patterns, and flags available in the **AgilePanel (`ap`)** command set.

---

## 📑 Table of Contents
- [🌐 Site Management (`ap site`)](#-site-management-ap-site)
- [🖥️ Server Administration (`ap server`)](#️-server-administration-ap-server)
- [📊 Information & Global Diagnostics](#-information--global-diagnostics)
- [🔧 Server Tools & Extensions (`ap tool` / `ap install` / `ap gui`)](#-server-tools--extensions-ap-tool--ap-install--ap-gui)

---

## 🌐 Site Management (`ap site`)
Manage websites, configurations, and WordPress installations.

### `ap site create [domain] [flags]`
Provisions a new isolated website environment.
*   **Description**: Creates an unprivileged system user (`wp_[sanitized_domain]`), provisions an isolated PHP-FPM FastCGI pool, configures MariaDB credentials, secures Caddyfile routing, and installs the application.
*   **Flags**:
    *   `--php [version]`: Specify PHP version (`8.1`, `8.2`, `8.3`). Defaults to `8.3`.
    *   `--wp`: Automatically installs WordPress (alias for `--type=wp`).
    *   `--type [type]`: Specify site preset: `wp` (WordPress), `woocommerce` (optimized caching & memory), `laravel`, `php` (Custom PHP), or `html` (Static HTML). Defaults to `wp`.
    *   `--heavy-plugins`: Allocates 256MB PHP memory, 5000 `max_input_vars`, and 300s timeout for heavy page builders (Elementor, Divi, WooCommerce).
    *   `--memory [size]`: Explicit custom PHP memory limit (e.g. `128M`, `256M`, `512M`, `1024M`).
    *   `--db [default|true|false]`: Explicitly enable or disable MariaDB database provisioning (useful for static HTML sites).
    *   `--import-files [path]`: Path to local ZIP archive to extract into web root during creation.
    *   `--import-db [path]`: Path to local SQL or ZIP dump to import into database during creation.
    *   `--wp-user [username]`: WordPress administrative username.
    *   `--wp-pass [password]`: WordPress administrative password.
    *   `--wp-email [email]`: WordPress administrator email.
    *   `--wp-name [name]`: WordPress administrator display name.

### `ap site update [domain] [flags]`
Updates PHP version, memory limits, and performance parameters for an existing website with zero downtime.
*   **Description**: Updates PHP runtime version and/or PHP memory allocation with seamless zero-downtime reloads. For WordPress sites, automatically synchronizes `WP_MEMORY_LIMIT` and `WP_MAX_MEMORY_LIMIT` inside `wp-config.php`.
*   **Flags**:
    *   `--php83`: Switch website to PHP 8.3 immediately.
    *   `--php82`: Switch website to PHP 8.2 immediately.
    *   `--php81`: Switch website to PHP 8.1 immediately.
    *   `--php [version]`: Specify target PHP version (e.g. `8.3`, `8.2`, `8.1`).
    *   `--memory [size]`: Set PHP memory limit (e.g. `256M`, `512M`, `128M`).
    *   `--heavy-plugins`: Apply high-performance profile (256MB memory, 5000 max_input_vars, 300s timeout) for Elementor, WooCommerce, etc.

### `ap site delete [domain] [-y|--yes]`
Completely tears down a website.
*   **Description**: Drops the site's database, removes the associated system user, and deletes the files in its web directory.
*   **Flags**:
    *   `-y`, `--yes`: Bypass dual confirmation prompt.

### `ap site list`
Lists all hosted websites.
*   **Description**: Renders a clean, structured table listing all websites hosted on the server along with their active PHP version and site type.

### `ap site info [domain]`
Displays detailed website information.
*   **Description**: Queries and shows the directory paths, database credentials, active FPM pool details, configuration state, and SSL status.

### `ap site edit [domain]`
Edits the site configuration.
*   **Description**: Opens the PHP-FPM configuration pool file for the site in the system's default text editor.

### `ap site lock [domain] [-y|--yes]`
Locks the website directory to read-only/immutable attributes (`chattr +i`).
*   **Description**: Protects against files being injected, modified, or written during active exploits.
*   **Flags**:
    *   `-y`, `--yes`: Bypass confirmation prompt.

### `ap site unlock [domain]`
Unlocks the website directory.
*   **Description**: Restores standard write access, allowing plugin installations, theme updates, or core upgrades.

### `ap site cache-clean [domain] [flags]`
Clears website caching layers.
*   **Description**: Flushes transients, Redis object cache, PHP OPcache, or Caddy edge cache. If no flags are passed, flushes all caches.
*   **Flags**:
    *   `--wp`: Clean WordPress internal transients and query caches.
    *   `--redis`: Clean Redis Object Cache.
    *   `--opcache`: Reload PHP-FPM pool to flush OPcache.
    *   `--caddy`: Flush Caddy edge/HTTP cache.

### `ap site reinstall [domain]`
Reinstalls WordPress core files.
*   **Description**: Non-destructively downloads clean WordPress core files and regenerates database tables under existing credentials.

### `ap site ssl-renew [domain]`
Renews SSL certificate.
*   **Description**: Clears Caddy's local certificate cache and forces Caddy to request a fresh Let's Encrypt / ZeroSSL certificate.

### `ap site fix-permissions [domain]`
Fixes site permissions.
*   **Description**: Recursively restores standard directory (0755) and file (0644) permissions, ensuring correct unprivileged user ownership.

### `ap site backup [domain]`
Creates separate manual ZIP backups.
*   **Description**: Backs up website files and database schemas into separate ZIP archives located under the site's secure `/backup` directory. Automatically prunes older local archives, retaining the 2 most recent backups.

### `ap site backup-db [domain]`
Backs up only the database.
*   **Description**: Dumps a raw SQL export of the website's database inside the secure backup folder.

### `ap site restore [domain] [backup-target] [-y|--yes]`
Restores website files and/or MariaDB database from local backups.
*   **Description**: Automatically unzips and rolls back site files (`htdocs`) and/or imports the database dump from the site's secure `/backup` folder. Supports `latest` or specific backup timestamps.
*   **Flags**:
    *   `-y`, `--yes`: Bypass confirmation prompt.

### `ap site doctor [domain]`
Runs automated diagnostics and pre-flight health checks.
*   **Description**: Audits DNS reachability (and Cloudflare proxy / IP mismatches), SSL certificate validity (<14 days warning), PHP-FPM service state, MariaDB connectivity, WordPress core files, `wp-config.php` placement, and recent fatal errors in `wp-content/debug.log`. Outputs a clear health checklist with actionable remedies.

### `ap site reset-pass [domain] [username] [password]`
Resets WordPress administrator password.
*   **Description**: Uses WP-CLI to update the administrator password in the database directly. If credentials are omitted, prompts or auto-generates a secure password.

### `ap site safe-mode [domain] [on|off]`
Toggles emergency safe mode to recover from White Screen of Death (WSOD).
*   **Description**: Passing `on` temporarily deactivates all active plugins via WP-CLI and snapshots their state into `conf/safemode_plugins.txt` to allow admin access during 500 errors. Passing `off` reads the snapshot and restores all previously active plugins.

### `ap site php-switch [domain] [version]`
Switches the active PHP version for a website.
*   **Description**: Rebuilds the site's PHP-FPM pool for the new PHP version (e.g. `8.1`, `8.2`, `8.3`), seamlessly updates Caddyfile fastcgi routing sockets, and reloads services without downtime.

### `ap site optimize [domain]`
Optimizes database tables and clears expired cache/spam.
*   **Description**: Defragments MariaDB tables (`wp db optimize`), removes expired transients, and deletes spam/trash comments to accelerate page generation.

### `ap site s3-list` / `s3-download` / `s3-delete`
Manages offsite cloud backups stored on S3-compatible cloud storage (AWS S3, Wasabi, Backblaze B2, Cloudflare R2, MinIO).
*   `ap site s3-list [domain] [--json]`: Lists recent S3 Cloud backup versions and timestamps. Use `--json` for machine-readable output.
*   `ap site s3-download [domain] [timestamp]`: Downloads a specific S3 backup archive to local storage.
*   `ap site s3-delete [domain] [timestamp]`: Deletes a specific S3 backup archive from cloud storage.

---

## 🖥️ Server Administration (`ap server`)
Monitor system health, manage credentials, restart services, tune hardware, and harden host security.

### `ap server status`
Displays the real-time status of server resources.
*   **Description**: Displays service statuses (Caddy, PHP-FPM, MariaDB, Redis), CPU load averages, visual RAM/Swap/Disk progress bars, active TCP connections, top resource-consuming processes, and historical peak performance logs (last 24 hours).

### `ap server auth [username] [password]`
Configures administrator credentials.
*   **Description**: Configures bcrypt-hashed HTTP Basic Authentication credentials used to secure phpMyAdmin and backend administrative tools. Prompts interactively if credentials are omitted.

### `ap server tune [flags]`
Optimizes server hardware resources.
*   **Description**: Audits VPS RAM capacity, CPU cores, filesystem, and virtualization type. Configures optimized MariaDB InnoDB buffer pool (30% RAM), Redis UNIX socket connections (`0770`), persistent swap file allocation, and kernel cache sysctl parameters (`vm.swappiness=10`, `vm.vfs_cache_pressure=50`).
*   **Flags**:
    *   `--admin-name [name]`: Update the administrator's name.
    *   `--admin-email [email]`: Update the administrator's email.

### `ap server secure`
Hardens system security policies.
*   **Description**: Restricts UFW firewall rules (whitelisting 22, 80, 443, 8888, 8889 while automatically detecting and preserving custom SSH ports), disables root password logins in SSH configuration (`PermitRootLogin prohibit-password` to enforce SSH keys), and enforces a strict 30-day root password rotation rule (`chage -M 30 root`).

### `ap server clean`
Purges expired logs, backups, and caches.
*   **Description**: Safely frees disk space by removing rotated/expired logs, vacuuming systemd journal logs down to 7 days, deleting backup zip files older than 3 days, and running `apt-get clean` and `apt-get autoremove`.

### `ap server restart [service|all]`
Restarts services.
*   **Description**: Restarts backend system services to apply configurations cleanly.
*   **Arguments**: `caddy`, `mariadb`, `redis`, `php-fpm` (or specific versions e.g. `php8.3-fpm`), or `all`.

### `ap server unlock-gui`
Disables secondary GUI dashboard session security PIN locks in the event of administrative lockout.

---

## 📊 Information & Global Diagnostics

### `ap info`
Shows a detailed visual dashboard of AgilePanel installation.
*   **Description**: Renders an ASCII banner and displays CLI version, binary path, config state file path, host OS and CPU architecture, server public IP, hosted site counts, Web GUI Addon status, and running stack services with their socket and config file paths.

### `ap repair [flags]`
Runs a non-destructive configuration repair.
*   **Description**: Scans and rebuilds all core PHP-FPM pools, Caddy configurations, and server settings. Realigns directories without touching site files, databases, or user credentials.
*   **Flags**:
    *   `--ssh-password-recovery`: Temporarily enables SSH root password login during repair (recovery mode).

### `ap sync`
Synchronizes panel configurations.
*   **Description**: Syncs active states and scans `/var/www/` directories to auto-detect and register pre-existing site folders back into the AgilePanel system database (`state.json`).

### `ap update`
Updates system package lists and CLI.
*   **Description**: Runs `apt-get update` to refresh OS repositories, checks for a new AgilePanel release version on GitHub, and self-updates the `/usr/local/bin/ap` binary.

### `ap upgrade`
Upgrades system packages.
*   **Description**: Non-interactively runs `apt-get upgrade` to apply system security updates and triggers `ap repair` to ensure Caddy and PHP configs remain compatible.

---

## 🔧 Server Tools & Extensions (`ap tool` / `ap install` / `ap gui`)

### `ap tool install phpmyadmin`
Installs phpMyAdmin globally on port `8888`, secured behind bcrypt HTTP Basic Auth.

### `ap tool fix-phpmyadmin`
Regenerates phpMyAdmin's `config.inc.php` file with a fresh, secure Blowfish secret.

### `ap install gui`
Downloads the pre-compiled AgilePanel Web GUI binary, provisions a systemd daemon service (`agilepanel-gui.service`) running on port `8889`, configures UFW firewall rules, and starts the dashboard.

### `ap gui enable` / `ap gui disable` / `ap gui update`
*   `ap gui enable`: Starts and enables the `agilepanel-gui` daemon service and opens port `8889`.
*   `ap gui disable`: Stops and disables the `agilepanel-gui` daemon service and blocks port `8889` access.
*   `ap gui update`: Downloads the latest compiled `agilepanel-gui` binary release and restarts the daemon.
