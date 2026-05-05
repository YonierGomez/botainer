# Botainer — Docker Management via Telegram

## Overview

Botainer is an open-source Telegram bot written in Go that enables remote Docker container management from any device with Telegram. It provides 25+ commands, real-time notifications, Docker Compose support, remote image tracking, and Helm chart monitoring.

**Repository**: https://github.com/YonierGomez/botainer  
**Landing Page**: https://yoniergomez.github.io/botainer/  
**Docker Hub**: https://hub.docker.com/r/yoniergomez/botainer  
**Version**: 1.2.0  
**Author**: Yonier Gomez (https://yonier.com)

---

## Core Features

### Container Management
- **List & Monitor**: View all containers with status indicators (🟢🔴🟡), CPU/RAM usage
- **Control**: Start, stop, restart, pause, unpause containers
- **Logs**: Real-time logs with error highlighting, filters, download as .log file
- **Exec**: Execute commands inside containers
- **Inspect**: Detailed container, image, volume, and network inspection
- **Create**: Step-by-step wizard for Docker Run and Compose

### Docker Compose
- Full project management: up, down, restart, pull, ps
- Auto-detection of compose projects from workspace
- Project-based update notifications

### Notifications & Monitoring
- Real-time alerts for container events (start, stop, crash, restart, pause, remove)
- Automatic image update detection (every 6 hours)
- System diagnostics (stopped containers, high resource usage, unhealthy status)
- Resource monitoring (CPU, RAM, disk usage)

### Remote Tracking (v1.2.0)
- **Image Tracking** (`/trackimage`): Monitor Docker images from any registry (Docker Hub, GHCR, private registries) for version updates
- **Helm Chart Tracking** (`/trackchart`): Monitor Helm charts from Artifact Hub, supports URL or `repo/chart` format
- Automatic checks every 6 hours with notifications

### Security & Utilities
- User whitelist (ALLOWED_USERS environment variable)
- Favorites system (per-user persistent)
- Command history tracking
- Universal search across containers, images, volumes
- Resource cleanup (prune unused images, volumes, networks)
- Volume backup to .tar.gz

---

## Commands Reference

### Basic Commands
- `/start` - Main menu with interactive buttons
- `/list` - All containers with status in single message
- `/ps` - Running containers with CPU/RAM stats
- `/running` - All containers with action buttons
- `/stats` - System dashboard (CPU, RAM, disk)
- `/uptime` - System uptime information

### Container Management
- `/create` - Container creation wizard (Docker Run or Compose)
- `/restart` - Restart a container
- `/stop` - Stop a container
- `/start_container` - Start a stopped container
- `/pause` / `/unpause` - Pause/resume a container
- `/exec` - Execute command inside container
- `/logs` - View real-time logs
- `/logfile` - Download logs as .log file
- `/inspect` - Inspect containers, images, volumes, networks
- `/env` - View container environment variables

### Images & Updates
- `/images` - List local images
- `/checkupdates` - Manually check for image updates
- `/updateall` - Update all images and recreate containers
- `/autoupdate` - Configure automatic container updates
- `/trackimage` - Track remote Docker images for updates
- `/trackchart` - Track Helm charts from Artifact Hub

### Docker Compose
- `/compose` - Manage Compose projects (up, down, restart, pull, ps)

### Resources
- `/volumes` - List volumes with backup option
- `/networks` - List networks
- `/prune` - Clean up unused resources (images, volumes, networks)

### Utilities
- `/search` - Search across containers, images, volumes
- `/diagnose` - Auto diagnostics (stopped containers, high resource usage)
- `/favorites` - View favorite containers
- `/addfav` - Add container to favorites
- `/history` - Command execution history
- `/backup` - Backup volume to .tar.gz
- `/version` - Bot version information

---

## Architecture & Tech Stack

### Technology
- **Language**: Go (golang)
- **Bot Framework**: go-telegram-bot-api v5
- **Docker SDK**: github.com/docker/docker/client
- **Base Image**: Alpine Linux (multi-stage build)
- **Registries**: Docker Hub, GitHub Container Registry
- **CI/CD**: GitHub Actions (multi-arch: amd64 + arm64)

### Project Structure
```
botainer/
├── main.go                    # Single-file architecture (all bot logic)
├── go.mod / go.sum           # Go dependencies
├── Dockerfile                # Multi-stage build
├── docker-compose.yml        # Local deployment
├── .env / .env.example       # Configuration
├── locale/                   # Translations (es, en)
│   ├── es.json
│   └── en.json
├── docs/                     # GitHub Pages landing
│   ├── index.html
│   ├── bot-image.png         # OG preview (1200×630)
│   └── botainer-image.png    # Logo
└── .github/workflows/
    └── docker-multiarch.yml  # CI/CD pipeline
```

### Configuration
Environment variables in `.env`:
```env
# Required
TELEGRAM_BOT_TOKEN=your_bot_token

# Optional
ALLOWED_USERS=123456789,987654321  # Comma-separated User IDs
NOTIFY_CHAT_ID=123456789           # Chat ID for notifications
CHECK_UPDATES_INTERVAL=6           # Hours between update checks
ENABLE_AUTO_CHECK=true             # Enable automatic update checks
ENABLE_STARTUP_NOTIFICATION=true   # Send notification on bot start
LANGUAGE=es                        # Bot language (es/en)
HOST_HOME=/home/ubuntu             # Host home directory
WORKSPACE=/workspace               # Docker Compose projects directory
```

### Persistence
- Configuration stored in `/data/config.json` (Docker volume: `botainer_data`)
- Persists: auto-update settings, tracked images, tracked charts
- Survives container restarts

---

## Remote Tracking Features (v1.2.0)

### Image Tracking
**Command**: `/trackimage`

**Supported Formats**:
- `nginx:latest`
- `ghcr.io/user/app:main`
- `docker.io/library/redis:alpine`
- `registry.example.com/image:tag`

**How it works**:
1. Bot pulls the image to get current digest
2. Stores image:digest mapping in config
3. Every 6 hours, pulls again and compares digests
4. Sends notification if digest changed (new version)

**Notification includes**:
- Image name and tag
- Old vs new digest (last 19 chars)
- Image size
- Close button

### Helm Chart Tracking
**Command**: `/trackchart`

**Supported Formats**:
- `repo/chart` (e.g., `bitnami/nginx`, `argo/argo-cd`)
- Full URL: `https://artifacthub.io/packages/helm/argo/argo-cd`

**How it works**:
1. Bot queries Artifact Hub API: `GET https://artifacthub.io/api/v1/packages/helm/{repo}/{chart}`
2. Stores chart:version mapping in config
3. Every 6 hours, queries API again and compares versions
4. Sends notification if version changed

**Notification includes**:
- Chart name (repo/chart)
- Repository name
- Old vs new version
- App version (if available)
- Link to Artifact Hub

**API Response Structure**:
```json
{
  "version": "15.0.2",
  "app_version": "1.25.3",
  "repository": {
    "name": "bitnami"
  }
}
```

---

## Container Icons

Botainer uses contextual icons for better UX:
- 👑 botainer (self-reference)
- 🐘 postgres, mysql, mariadb
- 🍃 mongo
- ⚡ redis
- 🌐 nginx
- 🪶 apache
- 💚 node
- 🐍 python
- 🐘 php
- ☕ java
- 🐹 golang
- ☁️ nextcloud
- 🎬 radarr, plex
- 📺 sonarr, jellyfin, emby
- 🏠 heimdall, homarr
- 🔒 wireguard
- 🛡️ pihole, adguard
- 🔀 traefik
- 🐳 portainer
- 🗼 watchtower
- 📊 grafana
- 📈 prometheus

---

## CI/CD Pipeline

### Workflow: `docker-multiarch.yml`

**Triggers**:
- Push to `main` (paths: main.go, go.mod, go.sum, Dockerfile)
- Schedule: Every Monday at 08:00 UTC
- Manual dispatch with `force_build` option

**Phases**:
1. **check-version**: Detects Alpine, Go, and tgapi versions from Dockerfile/go.mod
2. **build**: Builds native images (amd64 on ubuntu-latest, arm64 on ubuntu-24.04-arm)
3. **build-and-push**: Merges digests into multi-arch manifest, pushes to Docker Hub
4. **release**: Creates GitHub Release with changelog and Docker pull instructions

**Version Format**: `{alpine}-go{go}-tgapi{tgapi}`  
Example: `3.21.3-go1.24.2-tgapiv5.5.1`

**Outputs**:
- Docker Hub: `yoniergomez/botainer:latest` + versioned tags
- GitHub Container Registry: `ghcr.io/yoniergomez/botainer:latest`
- GitHub Release with changelog

---

## Deployment

### Production Deployment
The bot runs as the `botainer` service inside `/home/ubuntu/chips_all/compose.yaml`:

```yaml
services:
  botainer:
    image: yoniergomez/botainer:latest
    container_name: botainer
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /home/ubuntu:/workspace:ro
      - botainer_data:/data
    env_file:
      - /home/ubuntu/botainer/.env
    environment:
      - HOST_HOME=/home/ubuntu
      - WORKSPACE=/workspace
    network_mode: host

volumes:
  botainer_data:
```

**After code changes**:
```bash
cd /home/ubuntu/botainer
docker compose -f /home/ubuntu/chips_all/compose.yaml up -d --build botainer
docker logs --tail 5 botainer  # Verify: "Bot iniciado: @botainerbot"
```

### Local Development
```bash
git clone https://github.com/YonierGomez/botainer.git
cd botainer
cp .env.example .env
# Edit .env with your token
docker compose up -d --build
docker logs -f botainer
```

---

## Branch Protection & Workflow

### Branch Rules
- **main is protected** — direct pushes blocked
- All changes via Pull Request
- At least 1 approving review required
- Stale reviews dismissed on new commits
- Force pushes and branch deletion disabled

### Development Workflow
```bash
git checkout -b feat/my-feature
# Make changes
git add -A && git commit -m "feat: description"
git push origin feat/my-feature
gh pr create --title "feat: description" --body "What and why"
```

---

## Code Conventions

### Architecture
- **Single-file**: All code in `main.go` (no packages)
- **Error handling**: Standard Go error handling, no panic in handlers
- **Concurrency**: Goroutines for all handlers, notifications, and checks
- **State management**: In-memory maps for user state, favorites, history

### Key Functions
- `handleStart()` - Main menu
- `handleCallback()` - All button interactions
- `monitorEvents()` - Docker event stream for notifications
- `checkUpdates()` - Periodic image update checks (6h interval)
- `runImageUpdateCheck()` - Check local container images
- `checkTrackedImages()` - Check remote tracked images
- `checkTrackedCharts()` - Check Helm charts from Artifact Hub
- `recreateContainer()` - Pull new image and recreate container
- `getIcon()` - Get contextual icon for container name

### Translations
- Stored in `locale/{lang}.json`
- Loaded at startup via `getText(key)` function
- Supports `$1`, `$2` placeholders for dynamic values

---

## Common Tasks

### Add a New Command
1. Add to `commands` slice in `main()` with `getText("menu_commandname")`
2. Add translation keys to `locale/es.json` and `locale/en.json`
3. Add case in `switch update.Message.Command()` section
4. Create handler function `handleCommandName(chatID int64)`
5. Test and verify command appears in Telegram

### Add a New Callback Action
1. Add case in `handleCallback()` switch statement
2. Parse `action` and `target` from callback data format `action:target`
3. Implement logic and send response
4. Call `bot.Request(tgbotapi.NewCallback(query.ID, "message"))` to acknowledge

### Modify Notifications
- Edit `monitorEvents()` function
- Docker events come from `cli.Events()` stream
- Filter by `event.Type` and `event.Action`
- Use `event.Actor.Attributes` for container details

### Update Landing Page
- Edit `docs/index.html`
- Update meta tags (title, description, OG, Twitter)
- Update JSON-LD structured data
- Test social preview with https://www.opengraph.xyz/

---

## Troubleshooting

### Bot Not Responding
```bash
docker ps | grep botainer
docker logs --tail 50 botainer
docker compose -f /home/ubuntu/chips_all/compose.yaml restart botainer
```

### Commands Not Showing in Telegram
- Commands auto-register on startup via `setMyCommands`
- Wait 1-2 minutes after restart
- Type `/` in chat to trigger command list refresh

### Docker Permission Error
```bash
sudo usermod -aG docker $USER
newgrp docker
```

### Tracked Images Not Updating
- Check `enableAutoCheck` is true
- Verify `notifyChatID` is set (send any message to bot)
- Check logs for pull errors
- Manually trigger with `/checkupdates`

### Helm Chart Tracking Fails
- Verify chart exists on Artifact Hub
- Check format: `repo/chart` or full URL
- Test API manually: `wget -qO- https://artifacthub.io/api/v1/packages/helm/bitnami/nginx`
- Ensure `wget` is available in container (included in Alpine)

---

## Links & Resources

- **GitHub**: https://github.com/YonierGomez/botainer
- **Landing Page**: https://yoniergomez.github.io/botainer/
- **Docker Hub**: https://hub.docker.com/r/yoniergomez/botainer
- **GHCR**: https://github.com/YonierGomez/botainer/pkgs/container/botainer
- **Telegram News**: https://t.me/botainer_news
- **Author**: https://yonier.com
- **License**: MIT

---

## Version History

### v1.2.0 (Current)
- ✨ Remote Docker image tracking (`/trackimage`)
- ✨ Helm chart monitoring from Artifact Hub (`/trackchart`)
- ✨ Support for Artifact Hub URLs
- 🔧 Container force delete with confirmation
- 📝 Updated documentation and landing page

### v1.1.0
- 🌐 Multi-language support (Spanish, English)
- 🔧 Improved error handling
- 📊 Enhanced statistics display

### v1.0.0
- 🎉 Initial release
- 🐳 25+ Docker management commands
- 🔔 Real-time notifications
- 📁 Docker Compose support
- 🔍 Auto diagnostics
