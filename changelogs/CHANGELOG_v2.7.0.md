
# Changelog v2.7.0 - Pin ImagePull to the daemon's native platform

**Version**: 2.7.0
**Release Date:** September 16, 2026

## Summary

All `ImagePull` calls now explicitly pin the target `Platform` to the
architecture reported by the connected Docker daemon (e.g. `linux/arm64`),
instead of leaving it empty and relying on default registry/daemon
negotiation.

## Why

The periodic auto-update check (`ENABLE_AUTO_CHECK`, every
`CHECK_UPDATES_INTERVAL` hours) calls `cli.ImagePull` for every image
currently in use, purely to compare digests — even when no auto-update is
configured for that container, and even when the result is only a
notification. With `image.PullOptions{}` left empty, the daemon decides
which manifest to pull from a multi-arch image index, which is not
guaranteed to always resolve to the host's native architecture (e.g. when
qemu/binfmt emulation for other platforms is registered). A mismatched or
corrupted layer pulled this way sits cached locally and only surfaces as a
failure later, when something else (a manual `docker compose up -d`, an
auto-update rule, a container recreate) actually uses that image — at which
point the container fails to start with `exec /entrypoint: exec format
error`, with no indication that a stale background pull was the cause.

This was observed in production with `aiogram/telegram-bot-api:latest`,
which is republished frequently upstream: the image was silently re-pulled
every 6 hours by the auto-check, and a later `docker compose up -d` picked
up a cached image that could not execute on the host, causing the
container to crash-loop.

## What changed

- Added `detectDaemonPlatform()`, called once in `main()` right after the
  Docker client is initialized. It queries `cli.Info(ctx)` and caches the
  daemon's `OSType`/`Architecture` as `daemonPullPlatform` (e.g.
  `"linux/arm64"`), normalizing kernel-style arch names
  (`x86_64`→`amd64`, `aarch64`→`arm64`, `armv7l`→`arm/v7`,
  `armv6l`→`arm/v6`) to the identifiers Docker registries expect.
- Added `pullOpts()`, a small helper returning
  `image.PullOptions{Platform: daemonPullPlatform}`.
- Replaced all 9 call sites that built `image.PullOptions{}` inline
  (auto-update check, tracked image checks, manual container/template
  pulls, rollback, recreate) with `pullOpts()`, so every pull — automatic
  or manual — always requests an image matching the daemon's own
  architecture.
- If platform detection fails for any reason (e.g. `cli.Info` errors), the
  bot logs a warning and falls back to the previous behavior (empty
  `Platform`, default negotiation) rather than failing to start.

## Not changed

- This does **not** change whether Botainer pulls images automatically at
  all — that's controlled independently by `ENABLE_AUTO_CHECK` (see
  README §7, Configuration). This release only makes every pull, automatic
  or manual, target the correct architecture.

## Verification

- `go build ./...` and `go vet ./...` pass.
- `docker compose build botainer` succeeds using the project's existing
  multi-stage `Dockerfile` (`golang:alpine` builder).

---

## 📝 Version History

- **v2.7.0** (Sep 16, 2026) - Pin ImagePull to the daemon's native platform
- **v2.6.1** (Sep 7, 2026) - Fix ~120 remaining hardcoded Spanish strings
- **v2.6.0** (Sep 7, 2026) - Full English/Spanish support (i18n)
- **v2.5.0** (Sep 7, 2026) - Mini App removed by default (breaking change)
- **v2.4.3** (Sep 4, 2026) - Security fix: Mini App API authorization bypass, CORS hardening
- **v2.4.2** (May 2026) - Fix compose update recreate, runComposeCmd with correct workdir
- **v2.4.1** (May 17, 2026) - Code quality, panic fixes, resolveComposeFile helper
- **v2.4.0** (May 17, 2026) - Logs fix, update reliability, new notification format

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.6.1...v2.7.0
