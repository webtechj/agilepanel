<p align="center">
  <img src="agilepanel_logo.png" alt="AgilePanel Logo" width="240" />
</p>

<h1 align="center">AgilePanel (ap)</h1>

<p align="center">
  <strong>A Secure, Lightweight, and Hyper-Fast CLI Control Panel for WordPress</strong>
</p>

<p align="center">
  <a href="https://github.com/webtechj/agilepanel/releases"><img src="https://img.shields.io/github/v/release/webtechj/agilepanel?color=blue&label=version&logo=github" alt="Release Version" /></a>
  <img src="https://img.shields.io/badge/Language-Go-00add8?logo=go&logoColor=white" alt="Go Language" />
  <img src="https://img.shields.io/badge/OS-Ubuntu%20%7C%20Debian-E95420?logo=ubuntu&logoColor=white" alt="OS Support" />
  <img src="https://img.shields.io/badge/Web_Server-Caddy-00a2db?logo=caddy&logoColor=white" alt="Caddy Web Server" />
  <img src="https://img.shields.io/badge/Database-MariaDB-003545?logo=mariadb&logoColor=white" alt="MariaDB Database" />
  <img src="https://img.shields.io/badge/Cache-Redis-dc382d?logo=redis&logoColor=white" alt="Redis Cache" />
  <img src="https://img.shields.io/badge/PHP-8.1%20%7C%208.2%20%7C%208.3-777bb4?logo=php&logoColor=white" alt="PHP Versions" />
  <a href="https://github.com/webtechj/agilepanel"><img src="https://img.shields.io/endpoint?url=https://telemetry.agilepanel.io/api/badge?metric=active" alt="Active Servers" /></a>
  <a href="https://github.com/webtechj/agilepanel"><img src="https://img.shields.io/endpoint?url=https://telemetry.agilepanel.io/api/badge?metric=sites" alt="WordPress Sites" /></a>
  <img src="https://img.shields.io/badge/License-MIT-green?logo=open-source-initiative&logoColor=white" alt="MIT License" />
</p>

---

## ⚡ What is AgilePanel?

**AgilePanel** is a modern, high-performance, and minimal WordPress VPS manager built in Go. It directly manages native server services—**Caddy**, **PHP-FPM**, **MariaDB**, and **Redis**—on bare-metal Linux (Ubuntu/Debian) via a fast, interactive command-line interface. 

By eliminating bloated web UIs and heavy daemon overhead (typical in platforms like cPanel or Plesk), AgilePanel secures your system, reduces memory footprints, and accelerates page load times out of the box.

---

## 🌟 Key Features

### 🔒 Security-First Architecture
*   **System User Isolation**: Automatically provisions each site under its own unprivileged system user (`wp_[sanitized_domain]`) to prevent cross-site security leaks.
*   **Hardened PHP Configurations**: Restricts directory access using `open_basedir` and disables dangerous system execution commands (like `exec`, `shell_exec`, `system`) by default.
*   **Automatic Namespace Obfuscation**: Generates randomized prefixes (e.g. `db_a1b2c3_`) for database namespaces, user credentials, and WordPress table prefixes to block generic SQL injection and automated path guessing.
*   **Caddy Security Filters**: Out-of-the-box filtering to block dotfiles (e.g. `.git`, `.env`), deny PHP execution recursively inside `/wp-content/uploads/`, and shield sensitive scripts (`xmlrpc.php`, `install.php`).

### 🚀 High-Performance Optimizations
*   **Hardware-Aware Tuning**: The `ap server tune` command automatically scales the MariaDB InnoDB buffer pool (allocated to 30% of system RAM), configures high-speed transaction flushes, and disables unnecessary replication logging.
*   **Built-in Compression**: Delivers resources instantly using automated Gzip and Zstd compression engines (`encode gzip zstd`).
*   **UNIX Socket Coupling**: Connects PHP-FPM, WordPress Redis object caches, and Caddy database queries directly through high-speed local UNIX sockets instead of slower TCP ports.
*   **Swap File Allocator**: Automatically provisions a persistent 2GB `/swapfile` and tweaks kernel swappiness boundaries to keep low-resource VPS nodes running smoothly under high traffic.

### 🛠️ Developer-Centric Operations
*   **On-Demand phpMyAdmin**: Install phpMyAdmin with one command. Access is hidden behind a dedicated global port (`IP:8888`) and shielded behind bcrypt-based HTTP Basic Authentication.
*   **Non-Destructive Repair System**: Run `ap repair` to completely recreate all configuration files, verify swap buffers, re-generate PHP pools, and reload services without touching databases, site files, or data.
*   **Automatic Directory Synchronization**: The `ap sync` command scans `/var/www/` for pre-existing website folders, reads their database settings directly from `wp-config.php`, and automatically registers them back into the panel's locked `state.json` file.
*   **Zero-Dependency Deployment**: Compiled as a standalone static Go binary with no Python or third-party runtime package dependencies required.

---

## 📥 Installation

Ensure your server is running a clean install of **Ubuntu 22.04+ LTS** or **Debian 11+**.

### Standard Installation (Latest Version)
Run the automated one-liner installer:
```bash
curl -sSL https://raw.githubusercontent.com/webtechj/agilepanel/main/install.sh | sudo bash
```

### Installation from Specific Release (e.g. v0.8)
You can choose to install specific tagged releases:
```bash
curl -sSL https://raw.githubusercontent.com/webtechj/agilepanel/v0.8/install.sh | AP_VERSION=v0.8 sudo -E bash
```

*The installer will prompt you for the server administrator's Name and Email. This email will be registered with Let's Encrypt / ZeroSSL for SSL certificate renewals.*

---

## 🛠️ CLI Reference & Command Set

AgilePanel is managed entirely via the `ap` command. It is designed to be friendly for newbies and powerful for CLI experts.

### 🌐 Site Management

| Command | Description |
| :--- | :--- |
| `ap site create [domain] --wp` | Creates a new PHP/WordPress site with automated system users, databases, and SSL. |
| `ap site delete [domain]` | Completely decommissions a website, drops its database, and cleans directory files. |
| `ap site list` | Renders a clean, responsive, stacked list of all hosted sites and their active status. |
| `ap site info [domain]` | Queries directories, database credentials, active FPM versions, and SSL certificates. |
| `ap site lock [domain]` | Sets site files to immutable/read-only to protect against write exploits. |
| `ap site unlock [domain]` | Restores standard write access to install plugins or run core upgrades. |
| `ap site cache-clean [domain]` | Flushes WordPress transients, Redis caches, PHP OPcache, and Caddy edge caches. |
| `ap site backup [domain]` | Generates separate manual ZIP backups of both web files and database schemas. |
| `ap site backup-db [domain]` | Creates a raw SQL database backup inside the site's secure `/backup` folder. |
| `ap site edit [domain]` | Opens the site's PHP-FPM configuration pool in your system's text editor. |
| `ap site reinstall [domain]` | Freshly reinstalls WordPress core files and database schemas under existing configs. |
| `ap site ssl-renew [domain]` | Clears the local Caddy certificates cache and forces Caddy to request a fresh SSL. |
| `ap site fix-permissions [domain]` | Recursively restores correct file (0644) and folder (0755) permissions. |

---

### 🖥️ Server Administration

*   **Monitor Status**: Check system CPU load, memory usage, site count, and active services:
    ```bash
    ap server status
    ```
*   **Configure Administrator Auth**: Configure basic auth credentials used to secure phpMyAdmin and backend tools:
    ```bash
    ap server auth [username] [password]
    ```
*   **Hardware Optimization**: Audit resources to re-tune buffers, Redis sockets, and Swap memory:
    ```bash
    ap server tune
    ```
*   **Service Restarts**: Restart web server stack components:
    ```bash
    ap server restart [caddy|mariadb|redis|php8.3-fpm|all]
    ```

---

### 🔧 Maintenance & Tools

*   **Install phpMyAdmin**: Install phpMyAdmin on-demand:
    ```bash
    ap tool install phpmyadmin
    ```
    *Access securely at `http://[your-server-ip]:8888` using the credentials configured in `ap server auth`.*
*   **Repair Configurations**: Verify and rebuild your server configs and PHP pools instantly:
    ```bash
    ap repair
    ```
*   **System Update**: Refresh apt repositories and self-update the `ap` executable to the latest build:
    ```bash
    ap update
    ```
*   **System Upgrade**: Upgrade all OS packages and run `ap repair` to ensure compatibility:
    ```bash
    ap upgrade
    ```
*   **Configuration Sync**: Synchronize configuration files and auto-import untracked site folders:
    ```bash
    ap sync
    ```

---

## 📊 Privacy & Telemetry

AgilePanel collects anonymous usage telemetry to track active installations and display live status badges on this repository.

### What is collected?
- A randomly generated unique server ID (`uuid` stored in `/etc/agilepanel/state.json`)
- System OS and Architecture (e.g. `linux`, `amd64`)
- Current AgilePanel version (e.g. `0.8.0`)
- The number of WordPress sites hosted on the server

> [!NOTE]
> We **never** collect, transmit, or store server IPs, site domains, administrator credentials, database configurations, or files. The telemetry system is fully anonymous.

### Opting Out
Telemetry runs asynchronously and will never block or slow down your commands. However, you can opt out of telemetry completely at any time by setting the `AGILEPANEL_TELEMETRY_URL` environment variable to `"none"`. E.g.:
```bash
export AGILEPANEL_TELEMETRY_URL="none"
```

---

## 📄 License

AgilePanel is open-source software licensed under the [MIT License](LICENSE).

