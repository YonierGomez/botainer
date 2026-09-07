# Changelog v2.5.0 - Mini App Removed by Default (Breaking Change)

**Version**: 2.5.0
**Release Date:** September 7, 2026

## Summary

Botainer v2.5.0 removes the Telegram Mini App (visual web dashboard) by
default. This is a **breaking change** for anyone relying on the Mini
App/dashboard — Botainer is now a pure command-based Telegram bot, with no
web dashboard exposed unless explicitly enabled.

This follows the security incident documented in
[CHANGELOG_v2.4.3.md](CHANGELOG_v2.4.3.md), where the Mini App's REST API
had an authorization bypass that allowed unauthorized access to Docker
control and container environment variables on affected instances. While
v2.4.3 fixed the underlying bug, exposing a Docker-control API over HTTP —
even a correctly authenticated one — is a broader attack surface than most
self-hosted setups need. Removing it by default reduces that surface for
everyone, while keeping the option available for users who explicitly want
it and understand the tradeoffs.

---

## 💥 Breaking Change

### Mini App API disabled by default
- The Mini App REST API server (`api/server.go`, port 8080) **no longer
  starts automatically**. It now requires `ENABLE_MINI_APP=true` in your
  environment to start.
- If you were using the Mini App/dashboard, it will stop working after
  upgrading unless you set `ENABLE_MINI_APP=true` **and** take the
  additional steps below.
- The Telegram bot's command interface (`/list`, `/ps`, `/stats`, `/logs`,
  etc. — all 25+ commands) is **unaffected** and continues to work exactly
  as before.

### Migration guide

If you want to keep using the Mini App:
1. Set `ENABLE_MINI_APP=true` in your `.env`
2. Restrict network access to the Mini App at your reverse proxy (e.g.
   `allow`/`deny` rules limiting access to your LAN/VPN) — do not expose it
   directly to the public internet
3. Keep `ALLOWED_USERS` set — it is now enforced in the API as well (fixed
   in v2.4.3)
4. Review [docs/security.md](../docs/security.md) for the current
   authentication model before re-enabling

If you don't need the Mini App, no action is required — this is the new
default behavior.

---

## 🗑️ Removed

- `docs/index.html` (landing page): removed the "Mini App" section and its
  nav link
- `README.md`: removed the "Mini App Features" section, the full "Mini App"
  usage/roadmap/architecture section, and related badges (React, TypeScript)
- Nothing was removed from the Go backend or `webapp/` source code — the
  Mini App remains available as an opt-in feature via `ENABLE_MINI_APP=true`

---

## 📝 Version History

- **v2.5.0** (Sep 7, 2026) - Mini App removed by default (breaking change)
- **v2.4.3** (Sep 4, 2026) - Security fix: Mini App API authorization bypass, CORS hardening
- **v2.4.2** (May 2026) - Fix compose update recreate, runComposeCmd with correct workdir
- **v2.4.1** (May 17, 2026) - Code quality, panic fixes, resolveComposeFile helper
- **v2.4.0** (May 17, 2026) - Logs fix, update reliability, new notification format
- **v2.3.1** (May 7, 2026) - Compact UI Redesign
- **v2.3.0** (May 7, 2026) - Collaboration Suite
- **v2.2.1** (May 6, 2026) - Complete Management Suite
- **v2.2.0** (May 6, 2026) - Advanced Management Release
- **v2.0.0** (May 6, 2026) - Telegram Mini App Release

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.4.3...v2.5.0
