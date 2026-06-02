# Graph Report - agilepanel  (2026-06-02)

## Corpus Check
- 40 files · ~51,413 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 370 nodes · 890 edges · 26 communities (19 shown, 7 thin omitted)
- Extraction: 70% EXTRACTED · 30% INFERRED · 0% AMBIGUOUS · INFERRED: 263 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `98e24d95`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- [[_COMMUNITY_Site Orchestration Core|Site Orchestration Core]]
- [[_COMMUNITY_Metrics & System Monitoring|Metrics & System Monitoring]]
- [[_COMMUNITY_Database Management|Database Management]]
- [[_COMMUNITY_Server CLI & UI Rendering|Server CLI & UI Rendering]]
- [[_COMMUNITY_Telemetry Service|Telemetry Service]]
- [[_COMMUNITY_Caddy Web Server Config|Caddy Web Server Config]]
- [[_COMMUNITY_Config State Management|Config State Management]]
- [[_COMMUNITY_S3 Backup Storage|S3 Backup Storage]]
- [[_COMMUNITY_Documentation & Commands|Documentation & Commands]]
- [[_COMMUNITY_Telemetry Ping Client|Telemetry Ping Client]]
- [[_COMMUNITY_PHP-FPM Pool Management|PHP-FPM Pool Management]]
- [[_COMMUNITY_CLI Prompt Helpers|CLI Prompt Helpers]]
- [[_COMMUNITY_Site Orchestration Tests|Site Orchestration Tests]]
- [[_COMMUNITY_CLI Entry Point|CLI Entry Point]]
- [[_COMMUNITY_Server Security Hardening|Server Security Hardening]]
- [[_COMMUNITY_Tool Installer (phpMyAdminGUI)|Tool Installer (phpMyAdmin/GUI)]]
- [[_COMMUNITY_WordPress Install Tests|WordPress Install Tests]]
- [[_COMMUNITY_Install Command|Install Command]]
- [[_COMMUNITY_Repair Command|Repair Command]]
- [[_COMMUNITY_Site Command|Site Command]]
- [[_COMMUNITY_Sync Command|Sync Command]]
- [[_COMMUNITY_Tool Command|Tool Command]]
- [[_COMMUNITY_Update Command|Update Command]]
- [[_COMMUNITY_Shell Installer Script|Shell Installer Script]]
- [[_COMMUNITY_Merge Script (Scratch)|Merge Script (Scratch)]]
- [[_COMMUNITY_Community 25|Community 25]]

## God Nodes (most connected - your core abstractions)
1. `Create()` - 36 edges
2. `ReadState()` - 30 edges
3. `GetStatePath()` - 25 edges
4. `Reinstall()` - 21 edges
5. `PrintInfo()` - 20 edges
6. `Sync()` - 19 edges
7. `PrintSuccess()` - 19 edges
8. `Delete()` - 18 edges
9. `Backup()` - 18 edges
10. `Divider()` - 18 edges

## Surprising Connections (you probably didn't know these)
- `AgilePanel` --references--> `AgilePanel Logo`  [EXTRACTED]
  README.md → agilepanel_logo.png
- `main()` --calls--> `Execute()`  [INFERRED]
  main.go → D:/repos/VPSops/cmd/root.go
- `SendTelemetryPing()` --references--> `State`  [EXTRACTED]
  D:/repos/VPSops/internal/config/telemetry.go → internal/config/telemetry.go
- `PingAsync()` --references--> `State`  [EXTRACTED]
  D:/repos/VPSops/internal/config/telemetry.go → internal/config/telemetry.go
- `TestSendTelemetryPing()` --references--> `T`  [EXTRACTED]
  D:/repos/VPSops/internal/config/telemetry_test.go → internal/config/telemetry_test.go

## Import Cycles
- None detected.

## Communities (26 total, 7 thin omitted)

### Community 0 - "Site Orchestration Core"
Cohesion: 0.15
Nodes (41): GetStatePath(), ReadState(), State, State, Time, State, State, Time (+33 more)

### Community 1 - "Metrics & System Monitoring"
Cohesion: 0.12
Nodes (30): Time, T, T, Duration, Time, T, T, ProcessStatus (+22 more)

### Community 2 - "Database Management"
Cohesion: 0.13
Nodes (25): T, T, T, T, CreateDatabase(), DeleteDatabase(), GenerateSecurePassword(), GenerateSecurePrefix() (+17 more)

### Community 3 - "Server CLI & UI Rendering"
Cohesion: 0.16
Nodes (28): init(), renderProgressBar(), Info(), TableColumn, Accent(), Banner(), Danger(), Header() (+20 more)

### Community 4 - "Telemetry Service"
Cohesion: 0.20
Nodes (22): Request, Time, T, ResponseWriter, BadgeMetrics, getClientIP(), getMetrics(), Request (+14 more)

### Community 5 - "Caddy Web Server Config"
Cohesion: 0.47
Nodes (9): installMetricsCron(), replaceLine(), setOrAppendRedisConfig(), SetupDefaultWebserver(), TuneDatabase(), TuneRedis(), TuneServer(), TuneSwap() (+1 more)

### Community 6 - "Config State Management"
Cohesion: 0.25
Nodes (16): GlobalConfig, SiteConfig, State, DefaultState(), GenerateUUID(), TestAdminFields(), TestConcurrentStateLocking(), TestDefaultState() (+8 more)

### Community 7 - "S3 Backup Storage"
Cohesion: 0.29
Nodes (16): Request, State, Request, State, Name, ReadSeeker, ListBucketResult, buildS3Request() (+8 more)

### Community 8 - "Documentation & Commands"
Cohesion: 0.18
Nodes (11): Diagnostics & Updates, AgilePanel Command Reference, Server Administration Command Set, Site Management Command Set, Server Tools Command Set, AgilePanel Logo, AgilePanel, Developer-Centric Operations (+3 more)

### Community 9 - "Telemetry Ping Client"
Cohesion: 0.27
Nodes (9): PingAsync(), SendTelemetryPing(), TestSendTelemetryPing(), TestSendTelemetryPingOptOut(), TelemetryPayload, State, T, State (+1 more)

### Community 10 - "PHP-FPM Pool Management"
Cohesion: 0.27
Nodes (8): T, T, DeletePHPPool(), GeneratePHPPool(), GetPHPPoolPath(), TestGeneratePHPPool(), WritePHPPool(), SiteConfig

### Community 11 - "CLI Prompt Helpers"
Cohesion: 0.61
Nodes (6): getDomainArg(), getServiceArg(), promptConfirm(), promptDoubleConfirm(), promptPassword(), promptString()

### Community 12 - "Site Orchestration Tests"
Cohesion: 0.57
Nodes (6): T, T, TestLaravelAndHTMLSiteOrchestration(), TestSanitizeUser(), TestSyncImport(), TestValidateDomain()

### Community 13 - "CLI Entry Point"
Cohesion: 0.29
Nodes (3): Execute(), main(), main()

### Community 14 - "Server Security Hardening"
Cohesion: 0.60
Nodes (3): CleanServer(), SecureServer(), UnlockGuiPanel()

### Community 15 - "Tool Installer (phpMyAdmin/GUI)"
Cohesion: 0.60
Nodes (3): FixPhpMyAdminConfig(), InstallGui(), InstallPhpMyAdmin()

### Community 16 - "WordPress Install Tests"
Cohesion: 0.06
Nodes (31): AgilePanel (ap) Command Reference, `ap install gui`, `ap repair`, `ap server auth [username] [password]`, `ap server restart [service]`, `ap server secure`, `ap server status`, `ap server tune` (+23 more)

### Community 24 - "Merge Script (Scratch)"
Cohesion: 0.12
Nodes (16): 🛠️ CLI Reference & Command Set, 🛠️ Developer-Centric Operations, 🚀 High-Performance Optimizations, 📥 Installation, Installation from Specific Release (e.g. v0.8), 🌟 Key Features, 📄 License, 🔧 Maintenance & Tools (+8 more)

### Community 25 - "Community 25"
Cohesion: 0.40
Nodes (3): T, T, TestGenerateCaddyfile()

## Knowledge Gaps
- **61 isolated node(s):** `Time`, `T`, `Time`, `Duration`, `T` (+56 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **7 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Create()` connect `Database Management` to `Site Orchestration Core`, `Server CLI & UI Rendering`, `Config State Management`, `S3 Backup Storage`, `Telemetry Ping Client`, `PHP-FPM Pool Management`, `Site Orchestration Tests`?**
  _High betweenness centrality (0.112) - this node is a cross-community bridge._
- **Why does `ReadState()` connect `Site Orchestration Core` to `Metrics & System Monitoring`, `Database Management`, `Server CLI & UI Rendering`, `Caddy Web Server Config`, `Config State Management`, `Site Orchestration Tests`?**
  _High betweenness centrality (0.085) - this node is a cross-community bridge._
- **Why does `GetStatePath()` connect `Site Orchestration Core` to `Metrics & System Monitoring`, `Database Management`, `Server CLI & UI Rendering`, `Caddy Web Server Config`, `Config State Management`?**
  _High betweenness centrality (0.079) - this node is a cross-community bridge._
- **Are the 30 inferred relationships involving `Create()` (e.g. with `DownloadFromS3()` and `GetStatePath()`) actually correct?**
  _`Create()` has 30 INFERRED edges - model-reasoned connections that need verification._
- **Are the 26 inferred relationships involving `ReadState()` (e.g. with `TestAdminFields()` and `TestConcurrentStateLocking()`) actually correct?**
  _`ReadState()` has 26 INFERRED edges - model-reasoned connections that need verification._
- **Are the 23 inferred relationships involving `GetStatePath()` (e.g. with `getMetricsPath()` and `RepairInstallation()`) actually correct?**
  _`GetStatePath()` has 23 INFERRED edges - model-reasoned connections that need verification._
- **Are the 18 inferred relationships involving `Reinstall()` (e.g. with `GetStatePath()` and `WithLockedState()`) actually correct?**
  _`Reinstall()` has 18 INFERRED edges - model-reasoned connections that need verification._