# Changelog v1.3.0

## 🎯 Major Feature Release

This is the biggest update to Botainer yet, adding **12 new major features** across monitoring, security, and infrastructure management.

---

## 📊 Phase 1: Alerts & Monitoring

### Resource Alerts (`/alerts`)
- **Configure CPU/RAM/Disk thresholds** per container
- **Real-time monitoring** with 30-second checks
- **Instant notifications** when thresholds are exceeded
- **Persistent configuration** across restarts

### Health Checks (`/healthchecks`)
- **HTTP health checks** - Monitor web services
- **TCP health checks** - Monitor any TCP service
- **Configurable intervals** per container
- **Automatic notifications** on failure

### Scheduled Reports (`/reports`)
- **Daily or weekly reports** delivered automatically
- **System overview** with container counts and status
- **Manual trigger** option for on-demand reports
- **Configurable schedule** (daily/weekly/disabled)

---

## 🔐 Phase 3: Security & Audit

### Audit Logs (`/audit`)
- **Complete command history** with timestamps
- **User tracking** - who executed what
- **Success/failure tracking** for all operations
- **Export capability** - download as JSON
- **Automatic cleanup** - keeps last 1000 entries

### Vulnerability Scanning (`/scan`)
- **Trivy integration** for CVE detection
- **Scan any container image** with one command
- **Critical and High severity** focus
- **Instant results** with vulnerability counts

### Webhooks (`/webhooks`)
- **External notifications** to any HTTP endpoint
- **Event filtering** - choose which events to send
- **Custom headers** support
- **Multiple webhooks** with individual enable/disable

### Auto-Update Policies (`/policies`)
- **Schedule-based updates** (cron format)
- **Resource-aware updates** (min RAM/disk requirements)
- **Auto-approve mode** for hands-free updates
- **Per-container configuration**

---

## 🌐 Phase 4: Networking & Infrastructure

### Network Management (`/networks`)
- **List all Docker networks** with details
- **View containers per network**
- **Create and delete networks**
- **Network inspection** with full metadata

### Port Management (`/ports`)
- **View all exposed ports** across containers
- **Detect port conflicts** automatically
- **Port-to-container mapping**
- **Protocol information** (TCP/UDP)

### Private Registry Support (`/registries`)
- **Connect to private registries**
- **Authentication support** (username/password)
- **Multiple registry management**
- **Enable/disable per registry**

### Intelligent Cleanup (`/cleanup`)
- **Detect orphaned images** automatically
- **Calculate space savings** before cleanup
- **Preview images** to be removed
- **One-click cleanup** with confirmation

---

## ↩️ Phase 2: Advanced Container Management (Already Implemented)

### Rollback System (`/rollback`)
- **Save previous image versions** automatically
- **One-click rollback** to any previous version
- **Version history** (up to 5 entries per container)
- **Automatic cleanup** of old entries

### Container Templates (`/templates`)
- **Save container configurations** as templates
- **Deploy from templates** with one click
- **Full config preservation** (env, ports, volumes, labels)
- **Template management** (view, deploy, delete)

### Advanced Search (`/search`)
- **Filter by labels** - `label:key=value`
- **Filter by environment** - `env:VAR`
- **Filter by status** - `status:running`
- **Free text search** across names and images

### Maintenance Mode (`/maintenance`)
- **Pause all containers** except critical ones
- **Protected containers** (botainer never paused)
- **One-click activation/deactivation**
- **State persistence** across restarts

---

## 🔧 Technical Improvements

### New Data Structures
```go
type ResourceAlert struct {
    CPUThreshold  float64
    RAMThreshold  float64
    DiskThreshold float64
    Enabled       bool
}

type HealthCheck struct {
    Type     string // http, tcp
    Target   string
    Interval int
    Enabled  bool
}

type AuditEntry struct {
    Timestamp time.Time
    UserID    int64
    Command   string
    Target    string
    Success   bool
}

type Webhook struct {
    URL     string
    Events  []string
    Headers map[string]string
    Enabled bool
}

type UpdatePolicy struct {
    Schedule      string
    MinFreeRAM    int
    MinFreeDisk   int
    AutoApprove   bool
    Enabled       bool
}

type Registry struct {
    URL      string
    Username string
    Password string
    Enabled  bool
}
```

### Background Monitoring
- **3 new goroutines** for continuous monitoring:
  - `monitorResources()` - CPU/RAM alerts every 30s
  - `runHealthChecks()` - HTTP/TCP checks every 60s
  - `sendScheduledReports()` - Daily/weekly reports

### Persistence
- **All new features persist** to `/data/config.json`
- **Automatic migration** from older config versions
- **Backward compatible** with existing installations

---

## 📊 Statistics

- **12 new commands** added
- **24 total features** (up from 12)
- **40+ commands** available
- **~1500 lines** of new code
- **3 monitoring goroutines** running in background
- **100% backward compatible**

---

## 📚 Documentation Updates

### README.md & README_EN.md
- **New "Advanced Features" section** with all new commands
- **Updated command tables** with descriptions
- **Persistence section** expanded

### Landing Page (docs/index.html)
- **24 feature cards** (up from 12)
- **40+ commands** listed (up from 25)
- **New command groups**: Monitoring, Security, Advanced
- **Updated hero section** with new capabilities

---

## 🚀 Upgrade Instructions

```bash
cd botainer
git pull
docker compose up -d --build
```

**Note**: All existing configurations are preserved. New features are opt-in and require manual configuration via their respective commands.

---

## 🔮 What's Next

Potential features for v1.4.0:
- Multi-server management
- Backup automation
- Container migration tools
- Performance analytics
- Custom dashboards

---

**Version**: 1.3.0  
**Date**: 2026-05-05  
**Type**: Major Feature Release  
**Breaking Changes**: None  
**Migration Required**: No
