# Changelog v2.0.0 - Telegram Mini App Release

**Release Date**: May 6, 2026

## 🎨 Major Features

### Telegram Mini App (NEW!)
A visual web dashboard that opens directly inside Telegram for managing Docker containers with a modern interface.

**Features:**
- 📊 **Real-time Dashboard**
  - Auto-refresh every 5 seconds
  - Container list with live status indicators (🟢 running, 🔴 stopped)
  - Search and filter by name, image, or state
  - Running/stopped container counters

- ⚡ **Quick Actions**
  - Start, stop, restart containers with one tap
  - Responsive design for mobile, tablet, and desktop
  - Smooth animations and transitions

- 📈 **Live Stats**
  - CPU usage with visual progress bars
  - Memory usage in GB with percentage
  - Real-time refresh button
  - Color-coded performance indicators

- 📋 **Colorized Logs**
  - Automatic pattern detection:
    - 🔴 Red: errors, exceptions, failures
    - 🟡 Yellow: warnings, deprecated
    - 🟢 Green: success, started, ready
    - 🔵 Blue: info, debug
  - Last 100 lines with refresh
  - Monospace font for readability

- 🔒 **Security**
  - Telegram authentication with HMAC-SHA256
  - User whitelist via `ALLOWED_USERS`
  - No external authentication needed
  - Validates all requests against bot token

- 🌙 **Dark Theme**
  - Optimized for mobile viewing
  - Gradient backgrounds
  - High contrast for readability
  - Smooth color transitions

## 🏗️ Technical Implementation

### Backend (Go)
- REST API with Gorilla Mux router
- Endpoints:
  - `GET /api/containers` - List all containers
  - `GET /api/containers/{id}` - Get container details
  - `POST /api/containers/{id}/start` - Start container
  - `POST /api/containers/{id}/stop` - Stop container
  - `POST /api/containers/{id}/restart` - Restart container
  - `GET /api/containers/{id}/logs` - Get container logs
  - `GET /api/containers/{id}/stats` - Get container stats
- CORS middleware for cross-origin requests
- Telegram auth middleware with HMAC validation
- Docker API integration for container management

### Frontend (React 19 + TypeScript)
- Vite build system for fast development
- Tailwind CSS for styling
- Telegram WebApp SDK integration
- Auto-refresh with silent updates
- Responsive design with mobile-first approach
- Error handling and loading states

### Deployment
- Multi-stage Docker build
- Frontend built and served from `/webapp/dist`
- Backend serves static files and API
- Single container deployment
- Port 8080 for HTTP server

## 🐛 Bug Fixes
- Fixed Docker logs multiplexed stream parsing (8-byte headers)
- Fixed stats calculation for CPU percentage
- Fixed memory division by zero errors
- Separated logs and stats modal states to prevent conflicts

## 📦 Dependencies
- `github.com/gorilla/mux` - HTTP router
- `github.com/gorilla/websocket` - WebSocket support (for future features)
- React 19 - Frontend framework
- TypeScript - Type safety
- Tailwind CSS - Styling
- Vite - Build tool

## 🔄 Breaking Changes
None. The Mini App is completely optional and doesn't affect existing bot commands.

## 📝 Notes
- The Mini App requires the bot to be running with network access
- Access via Telegram: Open bot → Menu → 🐳 Dashboard
- All existing bot commands continue to work as before
- The Mini App is served on port 8080 (internal, proxied by Telegram)

## 🚀 What's Next (v2.1)
- Historical resource usage charts (24h, 7d, 30d)
- Per-container resource breakdown
- Configurable alerts (CPU/RAM thresholds)
- Export metrics as CSV/JSON
- WebSocket for real-time updates without polling

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v1.3.1...v2.0.0
