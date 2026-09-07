# Changelog v2.4.3 - Security Fix (Mini App API Authorization)

**Version**: 2.4.3
**Release Date:** September 4, 2026

## Summary

Botainer v2.4.3 is a **critical security release**. It fixes an authorization
bypass in the Mini App REST API (`api/server.go`) that allowed any Telegram
user who had opened the bot to access the full API — including `docker
inspect` on every container on the host (exposing environment variables such
as third-party API tokens and passwords), Docker Compose actions, container
creation, and container lifecycle operations — regardless of the
`ALLOWED_USERS` whitelist.

If you expose the Botainer Mini App (`nginx` + `webapp/dist`) to the public
internet, **upgrade immediately** and rotate any credentials that were
present in environment variables of containers running on the same host,
since they may have been exposed via `GET /api/containers/{id}`.

---

## 🔒 Security Fixes

### Critical: Mini App API authorization bypass
- **Root cause**: `authMiddleware` / `validateTelegramAuth` in `api/server.go`
  verified the Telegram WebApp `initData` HMAC-SHA256 signature, but:
  1. Never checked `auth_date`, so a signed `initData` never expired.
  2. Never checked the `ALLOWED_USERS` env var — that whitelist was only
     enforced in the Telegram bot's command handlers (`main.go`), **not** in
     the REST API used by the Mini App.
  - As a result, any user who sent `/start` to the bot (even users excluded
    by `ALLOWED_USERS`) could obtain a validly signed `initData` and use it
    to call the API directly, bypassing all access restrictions.
- **Fix**:
  - `validateTelegramAuth` now rejects `initData` whose `auth_date` is older
    than 24h, or set unreasonably in the future (clock-skew tolerance: 5m).
  - `validateTelegramAuth` now extracts the `user.id` from `initData` and
    enforces it against `ALLOWED_USERS` (same env var used by the bot), via
    the new `isUserAllowed()` helper. Behavior is unchanged (allow-all) when
    `ALLOWED_USERS` is unset, matching existing bot semantics.
- **Tests**: added `api/server_test.go` with coverage for expired
  `auth_date`, future `auth_date`, users outside `ALLOWED_USERS`, users
  inside `ALLOWED_USERS`, invalid HMAC signatures, and missing `user` field.

### Hardening: CORS wildcard on the API
- **Root cause**: `corsMiddleware` set `Access-Control-Allow-Origin: *`,
  which would allow any website to read API responses from a browser
  context if it ever obtained a valid `initData` token.
- **Fix**: the middleware now reflects the request's `Origin` header
  (with `Vary: Origin`) instead of a wildcard, and explicitly allows the
  `X-Telegram-Init-Data` header used for authentication.

---

## 🛡️ Deployment Recommendations

These are operational recommendations, not code changes in this release:

- Do not expose the Mini App (`botainer.yonier.com` or equivalent) directly
  to the public internet unless you have additional network-level
  restrictions in place. Consider limiting access to your LAN and/or VPN via
  your reverse proxy (`allow`/`deny` rules), since the API grants full
  control over Docker on the host.
- Avoid publishing the backend's port (`8080` by default) directly on the
  host if it is not required — access it only through your reverse proxy.
- Rotate any credentials (API tokens, database passwords, etc.) stored as
  environment variables in containers running on the same Docker host as
  Botainer if you suspect your instance was exposed before upgrading.

---

## 📝 Version History

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

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.4.2...v2.4.3
