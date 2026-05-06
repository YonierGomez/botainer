<p align="center">
  <img src="https://raw.githubusercontent.com/YonierGomez/botainer/main/docs/botainer-image.png" alt="Botainer" width="300" />
</p>

<h1 align="center">Botainer</h1>

[![GitHub Stars](https://img.shields.io/github/stars/YonierGomez/botainer?style=flat&logo=github&label=Stars)](https://github.com/YonierGomez/botainer/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/YonierGomez/botainer?style=flat&logo=github&label=Forks)](https://github.com/YonierGomez/botainer/network/members)
[![GitHub Issues](https://img.shields.io/github/issues/YonierGomez/botainer?logo=github&label=Issues)](https://github.com/YonierGomez/botainer/issues)
[![GitHub License](https://img.shields.io/github/license/YonierGomez/botainer?logo=opensourceinitiative&label=License)](https://github.com/YonierGomez/botainer/blob/main/LICENSE)
[![Last Commit](https://img.shields.io/github/last-commit/YonierGomez/botainer?logo=github&label=Last%20Commit)](https://github.com/YonierGomez/botainer/commits/main)
[![Docker Pulls](https://img.shields.io/docker/pulls/yoniergomez/botainer?logo=docker&label=Docker%20Pulls)](https://hub.docker.com/r/yoniergomez/botainer)
[![Docker Image Size](https://img.shields.io/docker/image-size/yoniergomez/botainer/latest?logo=docker&label=Image%20Size)](https://hub.docker.com/r/yoniergomez/botainer)
[![GitHub Release](https://img.shields.io/github/v/release/YonierGomez/botainer?logo=github&label=Release)](https://github.com/YonierGomez/botainer/releases)
[![CI Status](https://img.shields.io/github/actions/workflow/status/YonierGomez/botainer/docker-multiarch.yml?logo=githubactions&label=CI)](https://github.com/YonierGomez/botainer/actions)
[![Telegram Channel](https://img.shields.io/badge/Telegram-News%20Channel-26A5E4?logo=telegram)](https://t.me/botainer_news)

### Tech Stack

![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white)
![Alpine Linux](https://img.shields.io/badge/Alpine_Linux-0D597F?logo=alpinelinux&logoColor=white)
![Telegram](https://img.shields.io/badge/Telegram-26A5E4?logo=telegram&logoColor=white)
![Docker Compose](https://img.shields.io/badge/Docker_Compose-2496ED?logo=docker&logoColor=white)
![React](https://img.shields.io/badge/React_19-61DAFB?logo=react&logoColor=black)
![TypeScript](https://img.shields.io/badge/TypeScript-3178C6?logo=typescript&logoColor=white)

**Telegram bot + Mini App** to manage Docker from your phone. 25+ commands, real-time notifications, automatic image update detection, remote image tracking, Helm chart monitoring, and a **visual dashboard** with dark theme.

🎨 **NEW in v2.0:** [Telegram Mini App](https://t.me/botainerbot) - Visual dashboard with real-time container management!

📢 **Stay updated:** Join our [Telegram News Channel](https://t.me/botainer_news) for updates and new features!

> **Note:** The Mini App (visual dashboard) is **completely optional**. The bot works perfectly without it using text commands only.

---

## Requirements

- Linux server with Docker and Docker Compose installed
- Telegram bot token (see next section)

---

## 1. Create the bot on Telegram

1. Open Telegram and search for [@BotFather](https://t.me/botfather)
2. Send `/newbot`
3. Choose a name for the bot (e.g. `My Docker Bot`)
4. Choose a username ending in `bot` (e.g. `mydocker_bot`)
5. BotFather will give you a token in this format:

```
123456789:ABCdefGHIjklMNOpqrsTUVwxyz
```

Save that token — you'll need it in the next step.

> To get your Telegram User ID (needed to restrict access), send a message to [@userinfobot](https://t.me/userinfobot).

---

## 2. Installation

```bash
git clone https://github.com/YonierGomez/botainer.git
cd botainer
cp .env.example .env
```

Edit `.env` with your token:

```bash
nano .env
```

```env
# Required
TELEGRAM_BOT_TOKEN=123456789:ABCdefGHIjklMNOpqrsTUVwxyz

# Optional: restrict access to these User IDs (comma-separated)
# If left empty, any user can interact with the bot
ALLOWED_USERS=123456789,987654321
```

---

## 3. Run with Docker

### Option A — Pre-built image (recommended)

Pull from Docker Hub:
```bash
docker pull yoniergomez/botainer:latest
```

Or from GitHub Container Registry:
```bash
docker pull ghcr.io/yoniergomez/botainer:latest
```

Edit `docker-compose.yml` to use the pre-built image:

```yaml
services:
  botainer:
    image: yoniergomez/botainer:latest  # or ghcr.io/yoniergomez/botainer:latest
    container_name: botainer
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /home/ubuntu:/workspace:ro
      - botainer_data:/data
    env_file:
      - .env
    environment:
      - HOST_HOME=/home/ubuntu
      - WORKSPACE=/workspace
    network_mode: host

volumes:
  botainer_data:
```

### Option B — Local build from source

```bash
docker compose up -d --build
```

Verify it's running:

```bash
docker logs -f botainer
```

You should see: `Bot started: @your_bot`

To stop it:

```bash
docker compose down
```

---

## 4. docker-compose.yml configuration

```yaml
services:
  botainer:
    build: .
    container_name: botainer
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /home/ubuntu:/workspace:ro
      - botainer_data:/data
    env_file:
      - .env
    network_mode: host

volumes:
  botainer_data:
```

The `/var/run/docker.sock` volume gives access to the host Docker daemon. The `/workspace` volume points to the directory where your Docker Compose projects live — adjust it to match your server.

> **Note**
> 
> The `botainer_data` volume is required to persist configuration (auto-update settings, tracked images, tracked charts). Without it, your settings will be lost when the bot restarts.

---

## 5. Available commands

### Menu & status

| Command | Description |
|---|---|
| `/start` | Main menu with buttons |
| `/list` | All containers with status (🟢🔴🟡) in a single message |
| `/ps` | Running containers with CPU and RAM |
| `/running` | All containers with action buttons |
| `/stats` | System dashboard (CPU, RAM, disk) |

### Container management

| Command | Description |
|---|---|
| `/create` | Wizard to create a container (Docker Run or Compose) |
| `/restart` | Restart a container |
| `/stop` | Stop a container |
| `/start_container` | Start a stopped container |
| `/pause` / `/unpause` | Pause / resume a container |
| `/exec` | Execute a command inside a container |
| `/logs` | View logs in real time |
| `/logfile` | Download logs as a `.log` file |
| `/inspect` | Inspect containers, images, volumes and networks |

### Images & updates

| Command | Description |
|---|---|
| `/checkupdates` | Manually check for image updates |
| `/updateall` | Update all images and recreate containers |
| `/images` | List local images |
| `/trackimage` | Track remote Docker images for updates |
| `/trackchart` | Track Helm charts from Artifact Hub |

### Docker Compose

| Command | Description |
|---|---|
| `/compose` | Manage Compose projects (up, down, restart, pull, ps) |

### Resources

| Command | Description |
|---|---|
| `/volumes` | List volumes |
| `/networks` | List networks |
| `/prune` | Clean up unused resources |
| `/search` | Search across containers, images and volumes |

### Utilities

| Command | Description |
|---|---|
| `/diagnose` | Auto diagnostics (stopped containers, high resource usage) |
| `/favorites` | View favorite containers |
| `/addfav` | Add a container to favorites |
| `/env` | View environment variables of a container |
| `/history` | Command execution history |

### Advanced Features

| Command | Description |
|---|---|
| `/rollback` | Rollback container to previous image version |
| `/templates` | Save and deploy container configurations |
| `/maintenance` | Maintenance mode (pause all except critical) |
| `/alerts` | Configure resource alerts (CPU/RAM thresholds) |
| `/healthchecks` | Configure HTTP/TCP health checks |
| `/reports` | Schedule daily/weekly system reports |
| `/audit` | View command execution audit log |
| `/scan` | Scan images for vulnerabilities (Trivy) |
| `/webhooks` | Configure webhooks for external notifications |
| `/policies` | Auto-update policies (schedule, conditions) |
| `/networks` | Manage Docker networks |
| `/registries` | Connect to private registries |
| `/cleanup` | Intelligent cleanup of orphaned images |
| `/ports` | Port management and conflict detection |

---

## 6. Automatic notifications

Notifications are activated when you send any message to the bot. You'll receive alerts for:

- 🟢 Container started
- 🔴 Container stopped
- 💥 Container crashed unexpectedly
- 🔄 Container restarted
- ⏸️ Container paused / ▶️ resumed
- 🗑️ Container removed
- 🆕 New image version available (with an update button)

### Image updates

The bot automatically checks for new image versions every 6 hours (first check 5 minutes after startup). You can also trigger it manually with `/checkupdates` or from the main menu.

**What it checks:**
- **Digest updates**: Same tag, new version (e.g., `nginx:latest` updated)
- **Newer tags**: Semver-based newer versions (e.g., `alpine:3.18` → `alpine:3.23` available)

When an update is detected, it sends a notification with buttons:

- **🔄 Actualizar: \<container\>** button that automatically updates the container:
  - For Compose services: Edits `compose.yaml` and runs `docker compose up -d <service>`
  - For standalone containers: Recreates container with new image tag

**Smart detection:**
- Automatically detects when multiple containers use the same image
- Only checks each unique image once (efficient)
- Shows update button for each container using that image
- Supports semver tags (3.18, 2.5, 1.25) and detects newer versions
- Skips floating tags (latest, alpine, stable) for newer tag detection
- Parallel checking with 10-second timeout per image (fast and reliable)

### Remote image & Helm chart tracking

Track updates for images and charts that aren't running locally:

**Track Docker images** (`/trackimage`):
- Monitor any Docker image from Docker Hub, GHCR, or private registries
- Supports formats: `nginx:latest`, `ghcr.io/user/app:main`, `registry.io/image:tag`
- Get notifications when new versions are available

**Track Helm charts** (`/trackchart`):
- Monitor Helm charts from Artifact Hub
- Paste the chart URL or use `repo/chart` format
- Examples: `https://artifacthub.io/packages/helm/argo/argo-cd` or `bitnami/nginx`
- Notifications include chart version and app version

Both tracking features check for updates every 6 hours automatically.

---

## 7. Configuration

Botainer can be configured through environment variables in `.env`:

### Basic Configuration

```env
# Required
TELEGRAM_BOT_TOKEN=your_bot_token

# Optional: restrict access (comma-separated User IDs)
ALLOWED_USERS=123456789,987654321

# Optional: chat ID for notifications
NOTIFY_CHAT_ID=123456789
```

### Advanced Configuration

```env
# Update check interval (hours, default: 6)
CHECK_UPDATES_INTERVAL=6

# Enable/disable automatic update checks (default: true)
ENABLE_AUTO_CHECK=true

# Enable/disable startup notification (default: true)
ENABLE_STARTUP_NOTIFICATION=true

# Bot language (default: es)
# Options: es (Spanish), en (English)
LANGUAGE=es
```

### Volumes

| Volume | Path | Purpose |
|---|---|---|
| Docker socket | `/var/run/docker.sock` | Access to Docker daemon (read-only) |
| Workspace | `/workspace` | Docker Compose projects directory (read-only) |
| Data | `/data` | Persistent storage for bot configuration |

The `botainer_data` volume stores:
- Auto-update settings per container
- Tracked remote images and their digests
- Tracked Helm charts with versions and metadata
- Last check timestamps

### Persistence

Bot configuration is automatically saved to `/data/config.json` and persists across restarts thanks to the `botainer_data` Docker volume. This includes:
- Auto-update settings per container
- Tracked remote images with digests
- Tracked Helm charts with versions, app versions, repos, and container images
- Last check timestamps

---

## 8. Security

Restrict access by adding your User ID to `.env`:

```env
ALLOWED_USERS=123456789
```

Additional recommendations:

- Rotate the token periodically via @BotFather (`/revoke`)
- Never commit the `.env` file (it's already in `.gitignore`)
- Use a VPN for remote server access

---

## 9. Update the bot

```bash
cd botainer
git pull
docker compose up -d --build
```

---

## 10. Troubleshooting

**Bot not responding**
```bash
docker ps | grep botainer
docker logs --tail 50 botainer
docker compose restart
```

**Docker permission error**
```bash
sudo usermod -aG docker $USER
newgrp docker
```

**Commands not showing in Telegram**

Commands are registered automatically on startup. If they don't appear, restart the bot, wait 1–2 minutes, then type `/` in the chat.

---

## 11. Roadmap: Telegram Mini App (Coming Soon)

We're planning to add a **Telegram Mini App** — a visual web interface that opens directly inside Telegram for a richer user experience.

### What is a Mini App?

A Mini App is a web application (HTML/CSS/JavaScript) that runs inside Telegram with access to special APIs. Think of it as having a full dashboard in your phone without leaving the chat.

### Planned Features

#### 📊 **Visual Dashboard**
- Real-time container status with live updates
- CPU, RAM, and disk usage graphs
- Container health indicators and alerts
- System overview with interactive charts

#### 🎛️ **Advanced Container Management**
- Drag-and-drop to reorder containers
- Bulk operations (start/stop/restart multiple containers)
- Quick filters (running, stopped, by image, by project)
- Container grouping by Docker Compose project

#### 📝 **Interactive Logs Viewer**
- Live log streaming with syntax highlighting
- Search and filter logs in real-time
- Download logs with date range selection
- Multi-container log aggregation

#### 🔧 **Visual Container Creation**
- Form-based container creation (no YAML needed)
- Port mapping with conflict detection
- Volume mounting with file browser
- Environment variable editor with validation
- Network selection with visual diagram

#### 📈 **Resource Monitoring**
- Historical resource usage charts (last 24h, 7d, 30d)
- Per-container resource breakdown
- Alerts configuration with visual thresholds
- Export metrics as CSV/JSON

#### 🔄 **Update Management**
- Visual diff of image changes
- Batch update with preview
- Rollback history with one-click restore
- Update scheduling (maintenance windows)

#### 🗂️ **Template Library**
- Browse and deploy pre-configured stacks
- Save your own templates with screenshots
- Share templates via link
- One-click deployment from template

#### 🌐 **Network Visualizer**
- Interactive network topology diagram
- Container connections and port mappings
- Network creation and management
- DNS resolution testing

### Technical Architecture

```
┌─────────────────────────────────────────┐
│   Telegram Mini App (Frontend)         │
│   - React + TypeScript                  │
│   - Telegram WebApp SDK                 │
│   - Real-time updates via WebSocket    │
└──────────────┬──────────────────────────┘
               │ HTTPS
               ▼
┌─────────────────────────────────────────┐
│   Botainer Backend (Go)                 │
│   - REST API for Mini App               │
│   - WebSocket for live updates          │
│   - Telegram auth validation            │
│   - Docker API integration              │
└──────────────┬──────────────────────────┘
               │ Unix Socket
               ▼
┌─────────────────────────────────────────┐
│   Docker Engine                         │
└─────────────────────────────────────────┘
```

### Why a Mini App?

**Current (Bot Commands)**
- ✅ Works everywhere (phone, desktop, web)
- ✅ No installation needed
- ✅ Simple text-based interface
- ❌ Limited interactivity
- ❌ No real-time updates
- ❌ Hard to visualize complex data

**Future (Mini App)**
- ✅ All benefits of bot commands
- ✅ Rich visual interface
- ✅ Real-time updates
- ✅ Interactive charts and graphs
- ✅ Better for complex operations
- ✅ Still inside Telegram (no external apps)

### Access Methods

The Mini App will be accessible via:
1. **Menu button** — Quick access from chat menu
2. **Inline button** — Launch from bot messages
3. **Direct link** — `https://t.me/botainerbot/app`
4. **Attachment menu** — Available in any chat

### Development Phases

**Phase 1: Foundation** (v2.0)
- Basic dashboard with container list
- Start/stop/restart actions
- Real-time status updates
- Logs viewer

**Phase 2: Monitoring** (v2.1)
- Resource usage charts
- Historical data
- Alerts configuration
- Export metrics

**Phase 3: Advanced Management** (v2.2)
- Visual container creation
- Bulk operations
- Template library
- Network visualizer

**Phase 4: Collaboration** (v2.3)
- Multi-user access control
- Audit log viewer
- Shared templates
- Team notifications

### Timeline

- **Q2 2026**: Planning and design
- **Q3 2026**: Phase 1 development
- **Q4 2026**: Beta testing
- **Q1 2027**: Public release

### Feedback Welcome

Have ideas for the Mini App? Open an issue on GitHub or join our [Telegram channel](https://t.me/botainer_news) to share your thoughts!

---

## Contributing

1. Create a branch from `main`
2. Make your changes and commit
3. Push the branch and open a Pull Request

```bash
git checkout -b my-feature
git add -A && git commit -m "feat: my change"
git push origin my-feature
```

---

## Support the project

If you find it useful, consider supporting development:

[![Buy Me A Coffee](https://img.shields.io/badge/Buy_Me_A_Coffee-FFDD00?logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/yoniergomez)
[![GitHub Sponsors](https://img.shields.io/badge/GitHub_Sponsors-EA4AAA?logo=githubsponsors&logoColor=white)](https://github.com/sponsors/YonierGomez)

---

## Links

- [GitHub](https://github.com/YonierGomez/botainer)
- [Author's website](https://www.yonier.com)

---

## License

MIT — see the [LICENSE](LICENSE) file.
