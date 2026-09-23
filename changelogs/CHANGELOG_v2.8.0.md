
# Changelog v2.8.0 - Notify without pulling when ENABLE_AUTO_CHECK=false

**Version**: 2.8.0
**Release Date:** September 22, 2026

## Summary

Update-check notifications for running containers and `/trackimage`-tracked
images no longer require `ENABLE_AUTO_CHECK=true` to fire. With the flag
disabled, Botainer now does a lightweight, no-pull registry digest check and
still notifies you when a new version is available — it just won't download
anything or auto-recreate containers until you act on it.

## Why

`ENABLE_AUTO_CHECK=false` was being used to stop Botainer from pulling
images on its own (see v2.7.0's rationale and the `.env` comment added
during the security remediation). But the periodic scan
(`checkUpdates()`) had a single gate around the *entire* check:

```go
if enableAutoCheck && notifyChatID != 0 {
    runImageUpdateCheck()
    checkTrackedImages(notifyChatID, false)
    checkTrackedCharts(notifyChatID, false)
}
```

Turning off automatic pulling also silently turned off *all* update
notifications — including for explicitly `/trackimage`/`/trackchart`-tracked
images and Helm charts, and for locally running containers — even though
`checkTrackedCharts` never pulled anything in the first place (it only calls
the Artifact Hub API). Users who disabled auto-pull for bandwidth/security
reasons stopped getting notified entirely, instead of just losing the
auto-pull behavior.

## What changed

- Added `getRemoteImageDigest()`, which calls the daemon's
  `DistributionInspect` API (`GET /distribution/{image}/json`) to fetch an
  image's current manifest digest straight from the registry, **without
  pulling any layers**.
- Added `getLocalImageDigest()`, which reads the digest Docker already has
  on record for a running container's image from `RepoDigests` (populated
  the last time that image was actually pulled) — also with no pull.
- `runImageUpdateCheck()` (checks images used by running containers) now
  branches on `ENABLE_AUTO_CHECK`:
  - `true` (unchanged): pulls each image, compares local image IDs, and
    auto-recreates containers enabled via the auto-update selector.
  - `false` (new): compares the local recorded digest against the
    registry's current digest with no pull; if they differ, sends the same
    notification (without a known size, and without auto-recreating
    anything) with manual update buttons.
- `checkTrackedImages()` (`/trackimage`) now stores and compares the
  registry manifest digest instead of the local Docker image ID:
  - Detecting a change never requires a pull.
  - When `ENABLE_AUTO_CHECK=true`, it additionally pre-pulls the new image
    so it's cached locally and reports its size, same as before.
  - When `false`, it just notifies, with no pull.
- `checkUpdates()`'s periodic loop no longer gates the whole scan behind
  `enableAutoCheck` — it always runs every `CHECK_UPDATES_INTERVAL` hours
  (as long as `NOTIFY_CHAT_ID` is set); each check function now decides
  internally whether it's allowed to pull.
- `checkTrackedCharts()` (`/trackchart`, Artifact Hub) is unaffected — it
  never pulled and always ran independently of this flag already.

## Not changed

- Manual per-container "update"/"recreate" buttons shown on a notification
  still pull that specific image when clicked, regardless of
  `ENABLE_AUTO_CHECK`.
- Registries not supported by `DistributionInspect` (none known — it goes
  through the daemon itself, so it works with any registry the daemon can
  already reach) are not a concern here, unlike the earlier
  `findNewerTag()` semver-tag check which is limited to Docker Hub/GHCR.

## Verification

- `go build main.go` and `go test ./...` pass (via `golang:alpine`, matching
  the project's Dockerfile builder).
- Manually verified against the `argo/argo-cd` tracked Helm chart and the
  production `botainer` container's own running-image check with
  `ENABLE_AUTO_CHECK=false`.

---

## 📝 Version History

- **v2.8.0** (Sep 22, 2026) - Notify without pulling when ENABLE_AUTO_CHECK=false
- **v2.7.0** (Sep 16, 2026) - Pin ImagePull to the daemon's native platform
- **v2.6.1** (Sep 7, 2026) - Fix ~120 remaining hardcoded Spanish strings
- **v2.6.0** (Sep 7, 2026) - Full English/Spanish support (i18n)
- **v2.5.0** (Sep 7, 2026) - Mini App removed by default (breaking change)
- **v2.4.3** (Sep 4, 2026) - Security fix: Mini App API authorization bypass, CORS hardening
- **v2.4.2** (May 2026) - Fix compose update recreate, runComposeCmd with correct workdir
- **v2.4.1** (May 17, 2026) - Code quality, panic fixes, resolveComposeFile helper
- **v2.4.0** (May 17, 2026) - Logs fix, update reliability, new notification format

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.7.0...v2.8.0
