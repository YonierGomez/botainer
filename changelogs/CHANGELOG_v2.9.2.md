
# Changelog v2.9.2 - Progress bar for /trackimage's manual check

**Version**: 2.9.2
**Release Date:** September 22, 2026

## Summary

`checkTrackedImages()` now shows the same live progress bar as the other
pull commands when triggered manually (the "🔄 Check now" button under
`/trackimage`) with `ENABLE_AUTO_CHECK=true`.

## What changed

- Added a loading message (`sendLoading`) before the check loop, only when
  `manual && enableAutoCheck` (there's something to actually pull and a
  message to update).
- Each tracked image's pull now goes through `pullImageWithProgress`
  instead of a plain `cli.ImagePull` + `io.Copy(io.Discard, ...)`, reusing
  the same loading message sequentially across however many tracked images
  turn out to have updates.
- New locale key `checking_tracked_images` (en/es).
- The periodic background run (`manual=false`) and lightweight/no-pull mode
  (`ENABLE_AUTO_CHECK=false`) are unaffected — no message is created, so
  `pullImageWithProgress` takes its existing silent path.

## Audit of every image-pulling command (why some aren't covered)

| Command / callback | Pull mechanism | Progress bar? |
|---|---|---|
| `recreate` | Docker API | ✅ (v2.9.0) |
| `update_recreate` (🔔 notification button) | Docker API | ✅ (v2.9.1) |
| `newtag_update` (standalone container) | Docker API | ✅ (v2.9.0) |
| `rollback_do` | Docker API | ✅ (v2.9.0) |
| `tpl_deploy` | Docker API | ✅ (v2.9.0) |
| `/trackimage` → check now (manual) | Docker API | ✅ (this release) |
| `newtag_update` (compose service) | `docker compose pull` (CLI) | ❌ no structured stream |
| `compose_pullup_service` | `docker compose up -d --pull always` (CLI) | ❌ no structured stream |
| `/updateall` confirm, compose containers | `docker compose pull` (CLI) | ❌ no structured stream |
| `/updateall` confirm, standalone containers | none (image already pulled during scan) | n/a |
| `/updateall` / `/checkupdates` scan phase | Docker API, up to 10 images in parallel | count-based ("X/Y checked"), not byte-level (parallel goroutines sharing one message can't render one coherent bar) |
| Periodic background check (`checkUpdates`) | Docker API (or no-pull digest check) | intentionally silent, no chat is watching a message |
| `validatePreUpdate`'s image pull | Docker API | dead code path — always called with `newImage=""` in the current codebase, never actually pulls |

The Compose-CLI rows are the only real gap: `docker compose pull`/`up
--pull` don't expose the same per-layer JSON events `cli.ImagePull` does, so
a byte-level bar there would require parsing Compose's textual CLI output
(fragile across Compose versions) or switching those specific update paths
to pull via the Docker API first and then `docker compose up -d --no-deps`
without `--pull` (bigger, riskier change to established update paths) —
not done here.

## Verification

- `go build main.go`, `go vet ./...`, `go test ./...` pass.

---

## 📝 Version History

- **v2.9.2** (Sep 22, 2026) - Progress bar for /trackimage's manual check
- **v2.9.1** (Sep 22, 2026) - Fix update_recreate button skipping the pull
- **v2.9.0** (Sep 22, 2026) - Live progress bar while pulling images
- **v2.8.0** (Sep 22, 2026) - Notify without pulling when ENABLE_AUTO_CHECK=false

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.9.1...v2.9.2
