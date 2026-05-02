# 🐳 Botainer

**Manage Docker from Telegram.** Open-source bot written in Go — 25+ commands, real-time notifications, Docker Compose support, multi-arch image (amd64 + arm64).

🔗 [GitHub](https://github.com/YonierGomez/botainer) · [Landing](https://yoniergomez.github.io/botainer/) · [Issues](https://github.com/YonierGomez/botainer/issues)

---

## Quick Start

```bash
docker run -d \
  --name botainer \
  --restart unless-stopped \
  -e TELEGRAM_BOT_TOKEN="your_token_here" \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  yoniergomez/botainer:latest
```

> Get your token from [@BotFather](https://t.me/botfather) on Telegram.

---

## With Docker Compose

```yaml
services:
  botainer:
    image: yoniergomez/botainer:latest
    container_name: botainer
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /home/ubuntu:/workspace:ro
    environment:
      - TELEGRAM_BOT_TOKEN=your_token_here
      - ALLOWED_USERS=123456789        # optional: restrict access by Telegram User ID
      - HOST_HOME=/home/ubuntu         # optional: for Compose project detection
      - WORKSPACE=/workspace           # optional: path to your Compose projects
    network_mode: host
```

```bash
docker compose up -d
docker logs -f botainer
# → Bot started: @your_bot
```

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `TELEGRAM_BOT_TOKEN` | ✅ | Bot token from @BotFather |
| `ALLOWED_USERS` | ❌ | Comma-separated Telegram User IDs. Empty = allow all |
| `HOST_HOME` | ❌ | Host home path (e.g. `/home/ubuntu`) |
| `WORKSPACE` | ❌ | Path to Docker Compose projects directory |

---

## Available Commands

| Category | Commands |
|---|---|
| Status | `/start` `/list` `/ps` `/running` `/stats` |
| Management | `/restart` `/stop` `/pause` `/unpause` `/create` |
| Logs | `/logs` `/logfile` |
| Images | `/images` `/checkupdates` `/updateall` |
| Compose | `/compose` |
| Resources | `/volumes` `/networks` `/prune` `/search` |
| Utilities | `/exec` `/inspect` `/env` `/diagnose` `/favorites` `/history` |

Commands are **auto-registered** in BotFather when the bot starts.

---

## Automatic Notifications

Once active, the bot sends alerts for:

- 🟢 Container started
- 🔴 Container stopped
- 💥 Container crashed
- 🔄 Container restarted
- ⏸️ Container paused / ▶️ resumed
- 🗑️ Container removed
- 🆕 New image version available (with one-tap update button)

Image updates are checked automatically every **6 hours**.

---

## Architectures

| Architecture | Tag |
|---|---|
| `linux/amd64` | `latest` |
| `linux/arm64` | `latest` |

Built natively on GitHub Actions — no QEMU emulation.

---

## Security

Restrict access to specific users by setting `ALLOWED_USERS`:

```bash
-e ALLOWED_USERS=123456789,987654321
```

Get your Telegram User ID from [@userinfobot](https://t.me/userinfobot).

---

## Tags

| Tag | Description |
|---|---|
| `latest` | Latest stable build |
| `{alpine}-go{go}-tgapi{tgapi}` | Versioned build (e.g. `3.21.3-go1.24.2-tgapiv5.5.1`) |

---

## License

MIT — [view license](https://github.com/YonierGomez/botainer/blob/main/LICENSE)
