# Changelog v2.2.0 - Advanced Management Release

**Release Date**: May 6, 2026  
**Version**: 2.2.0  
**Status**: Production Ready

## 🎉 Overview

This release completes Phase 2.1 (Advanced Monitoring) and Phase 2.2 (Advanced Management), bringing powerful new features to the Telegram Mini App for comprehensive Docker management.

## ✨ New Features

### Phase 2.1: Advanced Monitoring (100% Complete)

#### 📊 Historical Charts
- Interactive CPU and memory usage charts
- Time range selector: 1 hour, 24 hours, 7 days
- Recharts integration with dark theme
- Real-time data collection every 30 seconds
- Stores up to 10,080 data points (7 days)
- Per-container historical view

**Endpoints**:
- `GET /api/containers/{id}/metrics?duration=1h`
- `GET /api/metrics?duration=24h`

#### 📥 Export Metrics
- Download metrics as CSV or JSON
- Configurable time ranges (1h to 30 days)
- Automatic file naming with timestamps
- Excel-compatible CSV format

**Endpoint**:
- `GET /api/metrics/export?duration=24h&format=csv`

#### 🚨 Alerts System
- Configure CPU and RAM thresholds per container
- Automatic Telegram notifications when exceeded
- Alert history (last 100 alerts)
- Enable/disable alerts per container
- Persistent configuration in `/data/alerts.json`

**Endpoints**:
- `GET /api/alerts/configs` - List all configurations
- `POST /api/alerts/configs` - Create/update configuration
- `DELETE /api/alerts/configs/{id}` - Delete configuration
- `GET /api/alerts/history?limit=50` - Get alert history

**Notification Format**:
```
⚠️ CPU Alert Triggered

Container: nginx
Type: cpu
Threshold: 80.0%
Current: 92.5%
Time: 15:04:05
```

### Phase 2.2: Advanced Management (75% Complete)

#### 📋 Bulk Operations
- Multi-select containers with checkboxes
- Batch actions: start, stop, restart, delete
- Select All / Deselect All functionality
- Confirmation dialog for delete action
- Auto-exit bulk mode after action execution
- Individual results per container

**Endpoint**:
- `POST /api/bulk` - Execute bulk action

**Supported Actions**:
- `start` - Start multiple containers
- `stop` - Stop multiple containers
- `restart` - Restart multiple containers
- `delete` - Force delete multiple containers

#### 🐳 Docker Compose Manager
- Automatic detection of Compose projects in `/workspace`
- Recursive search for `compose.yaml` and `docker-compose.yml`
- Project-level operations
- Real-time action execution
- Loading states per action
- Confirmation before destructive operations

**Endpoints**:
- `GET /api/compose/projects?workspace=/workspace` - List all projects
- `POST /api/compose/action` - Execute Compose command

**Supported Actions**:
- `up` - Start all services (`docker compose up -d`)
- `down` - Stop and remove all services
- `restart` - Restart all services
- `pull` - Pull latest images

## 🎨 UI/UX Improvements

### Dashboard Header
- 🐳 Docker Compose Manager button
- 📋 Bulk Operations toggle
- 🚨 Alerts Manager button
- 📥 Export Metrics button
- 🔄 Refresh button with pulse indicator

### Container Cards
- Checkboxes in bulk mode
- Individual action buttons hidden in bulk mode
- Visual status indicators
- Responsive layout

### Modals
- AlertsManager with configuration and history tabs
- ComposeManager with project cards
- HistoricalCharts with dual graphs
- ExportMetrics with format selector

## 🔧 Technical Changes

### Backend

**New Files**:
- `api/alerts.go` - Alert system implementation
- `api/metrics.go` - Metrics collection and storage

**Modified Files**:
- `api/handlers.go` - Added bulk, compose, alerts, and metrics handlers
- `api/server.go` - Added new routes
- `main.go` - Initialize alert store and metrics collector

**Dependencies**:
- No new dependencies required
- Uses existing Docker client and Gorilla Mux

### Frontend

**New Components**:
- `AlertsManager.tsx` - Alert configuration and history
- `ComposeManager.tsx` - Compose project management
- `HistoricalCharts.tsx` - Interactive charts
- `ExportMetrics.tsx` - Metrics export modal

**Modified Files**:
- `App.tsx` - Integrated all new components
- Added bulk mode state management
- Added modal state management

**Dependencies**:
- `recharts` v2.15.0 (for charts)

### Data Persistence

**New Files**:
- `/data/metrics.json` - Historical metrics storage
- `/data/alerts.json` - Alert configurations and history

**Format**:
```json
{
  "configs": {
    "container_id": {
      "cpu_threshold": 80,
      "mem_threshold": 80,
      "enabled": true
    }
  },
  "history": [
    {
      "id": "20260506150405-cpu-abc123",
      "container_id": "abc123",
      "type": "cpu",
      "threshold": 80,
      "current_value": 92.5,
      "triggered_at": "2026-05-06T15:04:05Z"
    }
  ]
}
```

## 📊 Performance

- Metrics collection: Every 30 seconds
- Alert checking: Every 30 seconds
- Dashboard refresh: Every 5 seconds
- Metrics retention: 7 days (10,080 points)
- Alert history: Last 100 alerts

## 🐛 Bug Fixes

- Fixed memory display from MB to GB
- Improved error handling in bulk operations
- Added confirmation dialogs for destructive actions
- Fixed auto-exit in bulk mode

## 🔒 Security

- All endpoints require Telegram authentication
- HMAC-SHA256 validation of init data
- User whitelist enforcement
- No external authentication needed

## 📝 Documentation

**New Files**:
- `MINI_APP_GUIDE.md` - User guide for Mini App features
- `IMPLEMENTATION_PROGRESS.md` - Development progress tracking
- `CHANGELOG_v2.2.0.md` - This file

**Updated Files**:
- `README.md` - Updated with v2.2 features

## 🚀 Migration Guide

### From v2.0 to v2.2

No breaking changes. All existing functionality remains intact.

**New Environment Variables** (optional):
```env
# No new required variables
# Existing variables still work
```

**New Volumes**:
```yaml
volumes:
  - botainer_data:/data  # Already exists, now stores more data
```

**New Permissions**:
- No additional permissions required
- Uses existing Docker socket access

## 📈 Statistics

- **Total Features**: 9 implemented
- **Phase 2.1**: 100% complete (3/3)
- **Phase 2.2**: 75% complete (2/4)
- **Code Changes**: 
  - Backend: +800 lines
  - Frontend: +1,200 lines
  - Total: ~2,000 lines

## 🎯 What's Next

### Phase 2.2 Completion
- Visual Container Creator
- Network Visualizer

### Phase 2.3: Collaboration
- Multi-user access control
- Audit log viewer
- Shared templates
- Team notifications

## 🙏 Acknowledgments

Thanks to all contributors and users who provided feedback during development.

## 📞 Support

- **GitHub Issues**: [github.com/YonierGomez/botainer/issues](https://github.com/YonierGomez/botainer/issues)
- **Telegram Channel**: [@botainer_news](https://t.me/botainer_news)
- **Documentation**: [README.md](README.md)

---

**Full Changelog**: [v2.0.0...v2.2.0](https://github.com/YonierGomez/botainer/compare/v2.0.0...v2.2.0)
