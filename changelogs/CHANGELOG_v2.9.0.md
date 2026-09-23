
# Changelog v2.9.0 - Live progress bar while pulling images

**Version**: 2.9.0
**Release Date:** September 22, 2026

## Summary

Interactive commands that pull an image via the Docker API now show a live
text progress bar in the same Telegram message, instead of a static
"⏳ ..." message with no feedback until the pull finishes.

```
📥 Descargando imagen
`nginx:1.27`

[████████░░░░] 67%
📦 142.3 MB / 210.5 MB · capas 4/7
```

## What changed

- Added `pullImageWithProgress(ctx, imageTag, chatID, messageID)`, which
  decodes the JSON stream returned by `cli.ImagePull` (one event per layer:
  `status`, `id`, `progressDetail.current/total`), aggregates bytes across
  all layers currently known, and edits the given Telegram message roughly
  every 1.2s with a rendered progress bar, percentage, downloaded/total
  size and completed/total layer count. Throttled and de-duplicated so it
  doesn't hit Telegram's edit-rate limits. When `chatID`/`messageID` are
  `0` (no UI to update — e.g. the background auto-update loop) it just
  drains the pull like the previous `io.Copy(io.Discard, reader)`.
- Wired into every interactive command that pulls a single image and
  already has a message to edit:
  - `recreate` (manual container recreate/pull)
  - `newtag_update` for standalone containers (semver tag bump)
  - `rollback_do` (rollback to a previous image)
  - `tpl_deploy` (deploy a saved template)
- `recreateContainer()`, `doRollback()` and `deployTemplate()` now take
  `chatID`/`messageID` parameters; the one background caller
  (auto-update loop in `runImageUpdateCheck`) passes `0, 0` and keeps the
  old silent behavior.
- New locale key `image_pull_progress` (en/es).

## Not covered by this release

- Docker Compose-based pulls (`compose_pullup_service`, the compose branch
  of `newtag_update`, and `/updateall`'s compose path) shell out to
  `docker compose pull`/`up`, which doesn't expose the same structured
  per-layer JSON stream, so they still show a single "updating..." status
  message rather than a byte-level progress bar.
- The periodic background auto-check and `/trackimage` checks (pulls only
  happen there when `ENABLE_AUTO_CHECK=true`, see v2.8.0) remain silent —
  there's no chat actively watching a specific message for those.

## Verification

- `go build main.go`, `go vet ./...`, `go test ./...` pass (via
  `golang:alpine`, matching the Dockerfile builder).

---

## 📝 Version History

- **v2.9.0** (Sep 22, 2026) - Live progress bar while pulling images
- **v2.8.0** (Sep 22, 2026) - Notify without pulling when ENABLE_AUTO_CHECK=false
- **v2.7.0** (Sep 16, 2026) - Pin ImagePull to the daemon's native platform
- **v2.6.1** (Sep 7, 2026) - Fix ~120 remaining hardcoded Spanish strings
- **v2.6.0** (Sep 7, 2026) - Full English/Spanish support (i18n)
- **v2.5.0** (Sep 7, 2026) - Mini App removed by default (breaking change)
- **v2.4.3** (Sep 4, 2026) - Security fix: Mini App API authorization bypass, CORS hardening

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.8.0...v2.9.0
