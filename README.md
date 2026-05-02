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

### Tech Stack

![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white)
![Alpine Linux](https://img.shields.io/badge/Alpine_Linux-0D597F?logo=alpinelinux&logoColor=white)
![Telegram](https://img.shields.io/badge/Telegram-26A5E4?logo=telegram&logoColor=white)
![Docker Compose](https://img.shields.io/badge/Docker_Compose-2496ED?logo=docker&logoColor=white)

Telegram bot written in Go to manage Docker from your phone. 25+ commands, real-time notifications, automatic image update detection, and an interactive button interface.

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

### Option A — Pre-built image from Docker Hub (recommended)

```bash
docker pull yoniergomez/botainer:latest
```

Edit `docker-compose.yml` to use the image instead of building:

```yaml
services:
  botainer:
    image: yoniergomez/botainer:latest
    container_name: botainer
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /home/ubuntu:/workspace:ro
    env_file:
      - .env
    environment:
      - HOST_HOME=/home/ubuntu
      - WORKSPACE=/workspace
    network_mode: host
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
    env_file:
      - .env
    network_mode: host
```

The `/var/run/docker.sock` volume gives access to the host Docker daemon. The `/workspace` volume points to the directory where your Docker Compose projects live — adjust it to match your server.

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

When an update is detected, it sends a notification with buttons:

- If the container belongs to a Docker Compose project → **🔄 Pull & Up: \<project\>** button that runs `pull` + `up -d`
- If it's a standalone container → **🔄 Recreate: \<name\>** button

---

## 7. Security

Restrict access by adding your User ID to `.env`:

```env
ALLOWED_USERS=123456789
```

Additional recommendations:

- Rotate the token periodically via @BotFather (`/revoke`)
- Never commit the `.env` file (it's already in `.gitignore`)
- Use a VPN for remote server access

---

## 8. Update the bot

```bash
cd botainer
git pull
docker compose up -d --build
```

---

## 9. Troubleshooting

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
