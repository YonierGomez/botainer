# Botainer — Project Instructions

## What is Botainer

Botainer is an open-source Telegram bot written in Go that allows managing Docker containers remotely from a phone or any Telegram client. It supports 25+ commands, real-time notifications, Docker Compose management, automatic image update detection, and multi-arch Docker images (amd64 + arm64).

- **Repo**: https://github.com/YonierGomez/botainer
- **Landing**: https://yoniergomez.github.io/botainer/
- **Docker Hub**: https://hub.docker.com/r/yoniergomez/botainer
- **Author**: Yonier Gomez — https://yonier.com

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go (golang) |
| Bot framework | go-telegram-bot-api v5 |
| Base image | Alpine Linux (latest) |
| Container | Docker + Docker Compose |
| CI/CD | GitHub Actions (docker-multiarch.yml) |
| Registry | Docker Hub — yoniergomez/botainer |
| Landing | GitHub Pages — docs/index.html |

---

## Project Structure

```
botainer/
├── main.go                          # Entry point and all bot logic
├── go.mod / go.sum                  # Go module dependencies
├── Dockerfile                       # Multi-stage build (golang:alpine → alpine)
├── docker-compose.yml               # Local dev / production compose
├── .env / .env.example              # Bot token and allowed users config
├── docs/
│   ├── index.html                   # Landing page (GitHub Pages)
│   ├── og-preview.html              # Social preview card source (1200×630)
│   ├── bot-image.png                # OG/social preview image
│   └── botainer-image.png           # Official logo
└── .github/
    └── workflows/
        └── docker-multiarch.yml     # CI: build amd64+arm64, push to Docker Hub, GitHub Release
```

---

## Branch Protection Rules

- **main is protected** — direct pushes are blocked
- All changes must go through a Pull Request
- At least 1 approving review required before merge
- Stale reviews are dismissed on new commits
- Force pushes and branch deletion are disabled

### Workflow for changes

```bash
git checkout -b feat/my-feature
# make changes
git add -A && git commit -m "feat: description"
git push origin feat/my-feature
gh pr create --title "feat: description" --body "What and why"
```

---

## CI/CD Pipeline (docker-multiarch.yml)

The workflow triggers on:
- Push to `main` (paths: main.go, go.mod, go.sum, Dockerfile)
- Every Monday at 08:00 UTC (scheduled)
- Manual dispatch with `force_build` option

**Phases:**
1. `check-version` — detects Alpine, Go and tgapi versions, decides whether to build
2. `build` — builds native amd64 (ubuntu-latest) and arm64 (ubuntu-24.04-arm) images
3. `build-and-push` — merges digests into a multi-arch manifest, pushes to Docker Hub
4. `release` — creates a GitHub Release with changelog and Docker pull instructions

**Version format:** `{alpine}-go{go}-tgapi{tgapi}` e.g. `3.21.3-go1.24.2-tgapiv5.5.1`

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `TELEGRAM_BOT_TOKEN` | ✅ | Bot token from @BotFather |
| `ALLOWED_USERS` | ❌ | Comma-separated Telegram User IDs. Empty = allow all |
| `HOST_HOME` | ❌ | Host home directory (for Compose project detection) |
| `WORKSPACE` | ❌ | Path to Docker Compose projects directory |

---

## Coding Conventions

- All code is in `main.go` (single-file architecture)
- Go standard error handling — no panic in handlers
- Telegram commands are registered automatically via `setMyCommands` on startup
- Notifications run in a goroutine watching Docker events
- Image update checks run every 6 hours in a separate goroutine

---

## Landing & SEO

- Landing is at `docs/index.html` — served via GitHub Pages
- Language: **English**
- Full SEO: title, description, keywords, canonical, robots
- Open Graph + Twitter Card with absolute URLs pointing to `bot-image.png`
- JSON-LD structured data (SoftwareApplication schema)
- Favicon: `docs/botainer-image.png`
- Social preview image: `docs/bot-image.png` (1200×630px)

---

## Commit Convention

Use conventional commits:

```
feat: add new command /portforward
fix: handle empty container list in /ps
docs: update README installation steps
chore: bump actions to latest versions
ci: add arm64 native runner
refactor: extract notification handler
```
