# Graph Report - D:/repos/VPSops  (2026-06-01)

## Corpus Check
- 42 files · ~51,521 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 263 nodes · 607 edges · 25 communities (22 shown, 3 thin omitted)
- Extraction: 57% EXTRACTED · 43% INFERRED · 0% AMBIGUOUS · INFERRED: 262 edges (avg confidence: 0.8)
- Token cost: 1,200 input · 450 output

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
- [[_COMMUNITY_WordPress Install Tests|WordPress Install Tests]]
- [[_COMMUNITY_Shell Installer Script|Shell Installer Script]]

## God Nodes (most connected - your core abstractions)
1. `Create()` - 35 edges
2. `ReadState()` - 29 edges
3. `GetStatePath()` - 24 edges
4. `Reinstall()` - 20 edges
5. `PrintInfo()` - 19 edges
6. `Sync()` - 18 edges
7. `PrintSuccess()` - 18 edges
8. `Delete()` - 17 edges
9. `Backup()` - 17 edges
10. `Divider()` - 17 edges

## Surprising Connections (you probably didn't know these)
- `AgilePanel` --references--> `AgilePanel Logo`  [EXTRACTED]
  README.md → agilepanel_logo.png
- `main()` --calls--> `Execute()`  [INFERRED]
  main.go → cmd/root.go
- `renderProgressBar()` --calls--> `Muted()`  [INFERRED]
  cmd/server.go → internal/ui/ui.go
- `renderProgressBar()` --calls--> `repeat()`  [INFERRED]
  cmd/server.go → internal/ui/ui.go
- `List()` --calls--> `Accent()`  [INFERRED]
  internal/site/orchestrator.go → internal/ui/ui.go

## Import Cycles
- None detected.

## Communities (25 total, 3 thin omitted)

### Community 0 - "Site Orchestration Core"
Cohesion: 0.19
Nodes (33): GetStatePath(), State, ReloadCaddy(), ReloadPHP(), RepairInstallation(), InstallWordPress(), RunAsUser(), Backup() (+25 more)

### Community 1 - "Metrics & System Monitoring"
Cohesion: 0.12
Nodes (27): Duration, Time, T, T, ProcessStatus, cpuStats, HistoricalStats, GetHistoricalMetrics() (+19 more)

### Community 2 - "Database Management"
Cohesion: 0.13
Nodes (25): T, Time, CreateDatabase(), DeleteDatabase(), GenerateSecurePassword(), GenerateSecurePrefix(), TestCreateAndDeleteDatabase(), TestGenerateSecurePassword() (+17 more)

### Community 3 - "Server CLI & UI Rendering"
Cohesion: 0.11
Nodes (14): renderProgressBar(), TableColumn, Accent(), Banner(), Muted(), padRight(), PrintTable(), repeat() (+6 more)

### Community 4 - "Telemetry Service"
Cohesion: 0.19
Nodes (18): ResponseWriter, BadgeMetrics, getClientIP(), getMetrics(), Request, Time, handleHome(), handleJSONBadge() (+10 more)

### Community 5 - "Caddy Web Server Config"
Cohesion: 0.19
Nodes (14): State, T, GenerateCaddyfile(), TestGenerateCaddyfile(), WriteCaddyfile(), installMetricsCron(), replaceLine(), setOrAppendRedisConfig() (+6 more)

### Community 6 - "Config State Management"
Cohesion: 0.27
Nodes (15): GlobalConfig, SiteConfig, State, DefaultState(), GenerateUUID(), ReadState(), TestAdminFields(), TestConcurrentStateLocking() (+7 more)

### Community 7 - "S3 Backup Storage"
Cohesion: 0.28
Nodes (14): Request, State, Name, ReadSeeker, ListBucketResult, buildS3Request(), DeleteFromS3(), DownloadFromS3() (+6 more)

### Community 8 - "Documentation & Commands"
Cohesion: 0.18
Nodes (11): Diagnostics & Updates, AgilePanel Command Reference, Server Administration Command Set, Site Management Command Set, Server Tools Command Set, AgilePanel Logo, AgilePanel, Developer-Centric Operations (+3 more)

### Community 9 - "Telemetry Ping Client"
Cohesion: 0.33
Nodes (7): PingAsync(), SendTelemetryPing(), TestSendTelemetryPing(), TestSendTelemetryPingOptOut(), TelemetryPayload, State, T

### Community 10 - "PHP-FPM Pool Management"
Cohesion: 0.33
Nodes (7): T, DeletePHPPool(), GeneratePHPPool(), GetPHPPoolPath(), TestGeneratePHPPool(), WritePHPPool(), SiteConfig

### Community 11 - "CLI Prompt Helpers"
Cohesion: 0.52
Nodes (5): getDomainArg(), getServiceArg(), promptConfirm(), promptDoubleConfirm(), promptString()

### Community 12 - "Site Orchestration Tests"
Cohesion: 0.53
Nodes (5): T, TestLaravelAndHTMLSiteOrchestration(), TestSanitizeUser(), TestSyncImport(), TestValidateDomain()

## Knowledge Gaps
- **20 isolated node(s):** `install.sh script`, `GlobalConfig`, `Time`, `TelemetryPayload`, `T` (+15 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **3 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Create()` connect `Database Management` to `Site Orchestration Core`, `Server CLI & UI Rendering`, `Caddy Web Server Config`, `Config State Management`, `S3 Backup Storage`, `Telemetry Ping Client`, `PHP-FPM Pool Management`, `Site Orchestration Tests`?**
  _High betweenness centrality (0.148) - this node is a cross-community bridge._
- **Why does `ReadState()` connect `Config State Management` to `Site Orchestration Core`, `Metrics & System Monitoring`, `Database Management`, `Caddy Web Server Config`, `Site Orchestration Tests`?**
  _High betweenness centrality (0.115) - this node is a cross-community bridge._
- **Why does `GetStatePath()` connect `Site Orchestration Core` to `Metrics & System Monitoring`, `Database Management`, `Caddy Web Server Config`, `Config State Management`?**
  _High betweenness centrality (0.107) - this node is a cross-community bridge._
- **Are the 30 inferred relationships involving `Create()` (e.g. with `DownloadFromS3()` and `GetStatePath()`) actually correct?**
  _`Create()` has 30 INFERRED edges - model-reasoned connections that need verification._
- **Are the 26 inferred relationships involving `ReadState()` (e.g. with `TestAdminFields()` and `TestConcurrentStateLocking()`) actually correct?**
  _`ReadState()` has 26 INFERRED edges - model-reasoned connections that need verification._
- **Are the 23 inferred relationships involving `GetStatePath()` (e.g. with `getMetricsPath()` and `RepairInstallation()`) actually correct?**
  _`GetStatePath()` has 23 INFERRED edges - model-reasoned connections that need verification._
- **Are the 18 inferred relationships involving `Reinstall()` (e.g. with `GetStatePath()` and `WithLockedState()`) actually correct?**
  _`Reinstall()` has 18 INFERRED edges - model-reasoned connections that need verification._