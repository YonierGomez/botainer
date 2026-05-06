# Botainer Mini App - Changelog

## v2.0.1 - Mini App Enhancements (2026-05-06)

### 🎨 New Features

#### Container Stats
- ✅ Real-time CPU and memory usage display
- ✅ Beautiful gradient progress bars (blue-purple for CPU, green-teal for RAM)
- ✅ Shows MB used/limit and percentages
- ✅ Refresh button for live updates
- ✅ Only available for running containers

#### Search & Filters (Enhanced)
- ✅ Search bar to filter by container name or image
- ✅ Filter buttons: All, Running, Stopped
- ✅ Live counter updates on filter buttons
- ✅ Clear filters button when no results

#### Logs Viewer (Enhanced)
- ✅ View last 100 lines of container logs
- ✅ Full-screen modal with syntax highlighting
- ✅ Refresh button to reload logs
- ✅ Works for both running and stopped containers

#### Auto-Refresh
- ✅ Container list updates every 5 seconds automatically
- ✅ Silent background updates (no loading flicker)
- ✅ Visual indicator (pulsing green dot)
- ✅ Manual refresh button available

### 🔒 Security Improvements

- ✅ Fixed JSON content-type in auth middleware
- ✅ Better error handling for non-Telegram access
- ✅ No-cache headers to prevent stale content
- ✅ Stops auto-refresh when not in Telegram

### 🐛 Bug Fixes

- Fixed "Unexpected token" JSON parse errors
- Fixed auto-refresh causing errors outside Telegram
- Fixed cache issues in Telegram WebView
- Removed HTML parse mode from config
- Better error messages

### 📝 Documentation

- Added note that Mini App is completely optional
- Updated README with clearer instructions
- Added security documentation

### 📊 Stats

- **New endpoints:** 1 (`/api/containers/:id/stats`)
- **New features:** 4 major
- **Bug fixes:** 5
- **Commits:** 15+

## v2.0.0 - Telegram Mini App Release (2026-05-06)

### 🎨 New Features

#### Visual Dashboard
- ✅ Modern dark theme with gradient backgrounds
- ✅ Real-time container status display
- ✅ Stats cards showing running/stopped counts
- ✅ Responsive design optimized for mobile

#### Search & Filters
- ✅ Search bar to filter by container name or image
- ✅ Filter buttons: All, Running, Stopped
- ✅ Live counter updates on filter buttons
- ✅ Clear filters button when no results

#### Logs Viewer
- ✅ View last 100 lines of container logs
- ✅ Full-screen modal with syntax highlighting
- ✅ Refresh button to reload logs
- ✅ Works for both running and stopped containers

#### Auto-Refresh
- ✅ Container list updates every 5 seconds
- ✅ Visual indicator (pulsing green dot)
- ✅ Manual refresh button available

#### Container Actions
- ✅ Start stopped containers
- ✅ Stop running containers
- ✅ Restart containers
- ✅ View logs for any container

### 🔒 Security

#### Telegram WebApp Authentication
- ✅ HMAC-SHA256 signature validation
- ✅ Only accessible from Telegram bot
- ✅ No direct URL access without auth
- ✅ Automatic token expiration (24h)

### 🛠️ Technical Stack

**Frontend:**
- React 19.0.0
- TypeScript 5.7.2
- Vite 6.0.7
- Tailwind CSS 4.0.0

**Backend:**
- Go with gorilla/mux
- REST API with JSON responses
- WebSocket support (ready for real-time)
- Docker API integration

**Infrastructure:**
- Nginx reverse proxy
- Cloudflare HTTPS
- Docker Compose deployment

### 📦 Deployment

```bash
# Pull latest image
docker pull yoniergomez/botainer:latest

# Update and restart
cd /home/ubuntu/chips_all
docker compose up -d --build botainer
docker compose restart nginx
```

### 🎯 Access

1. Open [@botainerbot](https://t.me/botainerbot) on Telegram
2. Click "🐳 Dashboard" button
3. Manage containers visually!

### 📊 Stats

- **Lines of code:** ~400 (frontend) + ~300 (backend)
- **Build size:** 197KB JS + 18KB CSS (gzipped)
- **API endpoints:** 7
- **Features:** 10+

### 🐛 Known Issues

None reported yet!

### 🚀 Coming Soon

**Phase 1 (Next):**
- Real-time stats with CPU/RAM graphs
- Live log streaming (WebSocket)
- Docker Compose project management
- Push notifications

**Phase 2:**
- Container creation wizard
- Image management
- Volume management
- Network visualizer

**Phase 3:**
- Templates system
- Backup/restore
- Historical monitoring
- Multi-server support

### 📝 Notes

- Mini App only works inside Telegram
- Requires valid Telegram authentication
- Auto-refresh can be disabled in future versions
- Logs limited to last 100 lines (configurable)

### 🔗 Links

- **GitHub:** https://github.com/YonierGomez/botainer
- **Docker Hub:** https://hub.docker.com/r/yoniergomez/botainer
- **Telegram Bot:** https://t.me/botainerbot
- **News Channel:** https://t.me/botainer_news
