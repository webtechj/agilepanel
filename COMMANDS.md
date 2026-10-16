# AgilePanel (ap) Command Reference

This file documents all the commands, subcommands, syntax options, and functionality available in the **AgilePanel (ap)** CLI command set.

---

## 🌐 Site Management (`ap site`)
Manage websites, configurations, and WordPress installations.

### `ap site create [domain]`
Creates a new website on the server.
*   **Description**: Provisions a new isolated system user (`wp_[sanitized_domain]`), configures the database, and deploys the site environment.
*   **Flags**:
    *   `--php [version]`: Specify the PHP version to use (e.g., `8.2`, `8.3`). If omitted, prompts interactively.
    *   `--wp`: Automatically installs WordPress (alias for `--type=wp`).
    *   `--type [type]`: Specify the type of site to create: `wp` (WordPress), `woocommerce` (optimized WooCommerce/caching setup), `laravel`, `php` (Custom PHP), or `html` (Static HTML). Defaults to `wp`.
    *   `--heavy-plugins`: Optimizes PHP memory (256MB), `max_input_vars` (5000), and execution timeout (300s) for heavy page builders like Elementor or WooCommerce. Prompts interactively during creation if omitted.
    *   `--memory [size]`: Explicitly define custom PHP memory limit (e.g., `128M`, `256M`, `512M`).

### `ap site delete [domain]`
Completely deletes a website.
*   **Description**: Drops the site's database, removes the associated system user, and deletes the files in its web directory.
*   **Security**: Requires dual confirmation (typing the domain name) to prevent accidental deletions.

### `ap site list`
Lists all websites.
*   **Description**: Renders a clean, structured table listing all websites hosted on the server along with their active PHP version and site type.

### `ap site info [domain]`
Displays detailed website information.
*   **Description**: Queries and shows the directory paths, database credentials, active FPM pool details, configuration state, and SSL status.

### `ap site lock [domain]`
Locks the website directory.
*   **Description**: Sets the site's directory permissions and attributes to read-only/immutable to protect against files being injected or modified in write exploits. Requires dual confirmation.

### `ap site unlock [domain]`
Unlocks the website directory.
*   **Description**: Restores standard write access, allowing plugin installations, theme updates, or core upgrades.

### `ap site cache-clean [domain]`
Clears website caching layers.
*   **Description**: Flushes transients, Redis object cache, PHP OPcache, or Caddy edge cache to reflect changes instantly.
*   **Flags** (If none are specified, all caches are flushed):
    *   `--wp`: Clean WordPress internal transients and query caches.
    *   `--redis`: Clean Redis Object Cache.
    *   `--opcache`: Reload PHP-FPM pool to flush OPcache.
    *   `--caddy`: Flush Caddy edge/HTTP cache.

### `ap site reinstall [domain]`
Reinstalls WordPress.
*   **Description**: Non-destructively downloads clean WordPress core files and regenerates database tables under existing credentials.

### `ap site ssl-renew [domain]`
Renews SSL certificate.
*   **Description**: Clears Caddy's local certificate cache and forces Caddy to request a fresh Let's Encrypt / ZeroSSL certificate.

### `ap site fix-permissions [domain]`
Fixes site permissions.
*   **Description**: Recursively restores standard directory (0755) and file (0644) permissions, ensuring correct unprivileged user ownership.

### `ap site backup [domain]`
Creates separate manual ZIP backups.
*   **Description**: Backs up website files and database schemas into separate ZIP archives located under the site's secure `/backup` directory.

### `ap site backup-db [domain]`
Backs up only the database.
*   **Description**: Dumps a raw SQL export of the website's database inside the secure backup folder.

### `ap site edit [domain]`
Edits the site configuration.
*   **Description**: Opens the PHP-FPM configuration pool file for the site in the system's default text editor.

### `ap site doctor [domain]`
Runs automated diagnostics and pre-flight health checks.
*   **Description**: Audits DNS reachability, SSL certificate validity, PHP-FPM service state, MariaDB connectivity, WordPress core files, and recent fatal errors in logs. Outputs a clear health checklist with actionable remedies.

### `ap site restore [domain] [backup-target]`
Restores website files and/or MariaDB database from local backups.
*   **Description**: Automatically unzips and rolls back site files (`htdocs`) and/or imports the database dump from the site's secure `/backup` folder. Supports `latest` or specific backup timestamps. Requires confirmation (or `-y`).

### `ap site reset-pass [domain] [username] [password]`
Resets WordPress administrator password.
*   **Description**: Uses WP-CLI to update the administrator password in the database directly. If credentials are omitted, prompts or auto-generates a secure password.

### `ap site safe-mode [domain] [on|off]`
Toggles emergency safe mode to recover from White Screen of Death (WSOD).
*   **Description**: Temporarily deactivates all active plugins via WP-CLI and snapshots their state to allow admin access during 500 errors. Passing `off` restores all previously active plugins.

### `ap site update [domain] [flags]`
Updates PHP version, memory limits, and performance parameters for an existing website.
*   **Description**: Updates PHP runtime version and/or PHP memory allocation with seamless zero-downtime reloads. For WordPress sites, automatically updates `WP_MEMORY_LIMIT` and `WP_MAX_MEMORY_LIMIT` inside `wp-config.php`.
*   **Flags**:
    *   `--php83`: Switch website to PHP 8.3 immediately (e.g. `ap site update sitename.com --php83`).
    *   `--php82`: Switch website to PHP 8.2 immediately.
    *   `--php81`: Switch website to PHP 8.1 immediately.
    *   `--php [version]`: Specify target PHP version (e.g. `8.3`, `8.2`).
    *   `--memory [size]`: Set PHP memory limit (e.g., `256M`, `512M`, `128M`).
    *   `--heavy-plugins`: Apply high-performance profile (256MB memory, 5000 max_input_vars, 300s timeout) for Elementor, WooCommerce, etc.

### `ap site php-switch [domain] [version]`
Switches the active PHP version for a website.
*   **Description**: Rebuilds the site's PHP-FPM pool for the new PHP version (e.g. `8.1`, `8.2`, `8.3`), seamlessly updates Caddyfile fastcgi routing sockets, and reloads services without downtime.

### `ap site optimize [domain]`
Optimizes database tables and clears expired cache/spam.
*   **Description**: Defragments MariaDB tables (`wp db optimize`), removes expired transients, and deletes spam/trash comments to accelerate page generation.

---

## 🖥️ Server Administration (`ap server`)
Monitor system health, manage users, restart services, and tune hardware.

### `ap server status`
Displays the real-time status of server resources.
*   **Description**: Displays service statuses (Caddy, PHP-FPM, MariaDB, Redis), CPU load averages, RAM/Swap usage bars, active TCP connections, top resource-consuming processes, and historical peak performance logs.

### `ap server auth [username] [password]`
Configures administrator credentials.
*   **Description**: Configures basic authentication credentials used to secure phpMyAdmin and other backend administrative tools. Prompts interactively if credentials are omitted.

### `ap server restart [service]`
Restarts services.
*   **Description**: Restarts backend system services to apply configurations cleanly.
*   **Arguments**: `caddy`, `mariadb`, `redis`, `php-fpm` (or specific versions e.g. `php8.3-fpm`), or `all`.

### `ap server tune`
Optimizes server hardware resources.
*   **Description**: Audits VPS RAM capacity and configures optimized database buffers (30% RAM), Redis UNIX socket connections, swap file allocation, and kernel network sysctl parameters.
*   **Flags**:
    *   `--admin-name [name]`: Update the administrator's name.
    *   `--admin-email [email]`: Update the administrator's email.

### `ap server secure`
Hardens system security policies.
*   **Description**: Restricts UFW firewall rules, disables root password logins in SSH configuration (relying on SSH keys), and enforces a strict 30-day root password rotation rule.

---

## 🔧 Server Tools (`ap tool`)
Install and maintain helper utilities.

### `ap tool install [tool]`
Installs server tools.
*   **Description**: Downloads and installs extra utilities on the server.
*   **Arguments**: `phpmyadmin` (Installs phpMyAdmin globally on port 8888, secured behind Basic Auth).

### `ap tool fix-phpmyadmin`
Fixes phpMyAdmin config errors.
*   **Description**: Re-generates phpMyAdmin's `config.inc.php` file with a fresh, secure blowfish secret.

---

## 📦 Extensions & Addons (`ap install`)
Install extension packages.

### `ap install gui`
Installs the Web GUI Dashboard companion.
*   **Description**: Downloads the pre-compiled AgilePanel Web GUI binary, provisions a systemd daemon service (`agilepanel-gui.service`) running on port 8889, configures UFW firewall rules, and starts the dashboard.

---


## 🛠️ Diagnostics & Updates
Manage CLI and package dependencies.

### `ap repair`
Runs a non-destructive configuration repair.
*   **Description**: Scans and rebuilds all core PHP-FPM pools, Caddy configurations, and server settings. Realigns directories without touching site files, databases, or user credentials.

### `ap sync`
Synchronizes panel configurations.
*   **Description**: Syncs active states and scans `/var/www/` directories to auto-detect and register pre-existing site folders back into the AgilePanel system database (`state.json`).

### `ap update`
Updates system package lists and CLI.
*   **Description**: Runs `apt-get update` to refresh OS repositories, checks for a new AgilePanel release version on GitHub, and self-updates the `/usr/local/bin/ap` binary.

### `ap upgrade`
Upgrades system packages.
*   **Description**: Non-interactively runs `apt-get upgrade` to apply system security updates and triggers `ap repair` to ensure Caddy and PHP configs remain compatible.
