
# Changelog v2.11.0 - Compose progress bar + app version in update notifications

**Version**: 2.11.0
**Release Date:** September 28, 2026

## Summary

Two related improvements to update notifications:

1. The remaining Docker Compose-based update paths now show the same live
   progress bar as the rest of the app while pulling.
2. Update notifications now show the app's actual version label (e.g.
   `v1.11.2-ls145`) next to the before/now hashes, when the image provides
   one — instead of just two meaningless hex fragments.

```
🔔 Update available

`lscr.io/linuxserver/pairdrop:latest`
━━━━━━━━━━━━━━━━
📦 before  `9f75eb18b5390fa25d1 (v1.11.1-ls144)`
✅ now    `e6c61ebbb39d1f80a9f (v1.11.2-ls145)`
💾 — · 📦 pairdrop
🗂 Project: work_pro
```

## Part 1 — Progress bar for Compose-based updates

Most real-world containers are managed via Docker Compose, so the "🔄 Pull
& Up" button on an update notification (`compose_pullup_service`) is the
one most users actually click — and it was still missing the byte-level
progress bar added in v2.9.0-v2.9.2, because it ran through
`docker compose pull`/`up --pull always`, which doesn't expose the
structured per-layer stream `cli.ImagePull` does.

- `compose_pullup_service`: inspects the running container to get its
  current image tag, pulls it via `pullImageWithProgress` (live bar on the
  same message), then runs `docker compose up -d --no-deps <service>`
  without `--pull`. If the pull fails (e.g. a locally built image with
  nothing to pull from a registry), it logs the error and still runs
  `up -d --no-deps`, falling back to whatever is cached.
- `newtag_update` for Compose services (semver tag bump): same swap —
  pulls the new tag via the API with a progress bar instead of
  `docker compose pull`.
- `/updateall`'s confirm step, Compose containers: same swap inside the
  per-container retry loop, reusing the batch's shared progress message.
- Standalone containers were already covered since v2.9.0/v2.9.1 and are
  unchanged.

## Part 2 — App version label next to the hash

`before`/`now` in update notifications were always short hex fragments of
an image ID or manifest digest — accurate for detecting a change, but
meaningless to read. Most container images set an
`org.opencontainers.image.version` (or similar) label with the actual app
version, so we now show it when available.

- Added `versionFromLabels()` (checks
  `org.opencontainers.image.version`, `org.label-schema.version`, plain
  `version`) and `localImageVersion()` (reads it off an already-local
  image — free, no network call).
- For **local running containers** (`runImageUpdateCheck`): the old
  image's version comes from the local label (always available, no
  network call). The new image's version comes from its just-pulled
  labels in full-pull mode, or from a **lightweight registry fetch** (see
  below) in no-pull/lightweight mode — no image layers downloaded either
  way.
- For **`/trackimage`** (`checkTrackedImages`): both the old and new
  version are resolved via the lightweight registry fetch — old by its
  stored digest (registries let you fetch a manifest by digest as well as
  by tag), new by the current tag.
- Added `fetchRemoteImageVersionAtRef()` / `fetchRemoteImageVersion()`:
  fetch the manifest (following a multi-arch index/manifest-list down to
  the entry matching the daemon's own platform) and then the tiny image
  config blob (a few KB of JSON, not the actual image layers) directly
  from the registry, and pull the version label out of it.
- **Generalized registry auth** (`fetchRegistryToken`): previously only
  Docker Hub and GHCR were hardcoded; every other registry now falls back
  to discovering the realm/service via the standard
  `WWW-Authenticate: Bearer realm="...",service="..."` challenge on the
  registry's `/v2/` endpoint. This is what makes the label lookup actually
  work for `lscr.io` (linuxserver.io's registry, which delegates auth to
  `ghcr.io` — verified manually end-to-end against
  `lscr.io/linuxserver/pairdrop`) and should work for most other
  standards-compliant registries too, not just the two previously
  hardcoded ones. `findNewerTag()` (existing semver-tag-bump feature)
  benefits from this too, for free.
- If no version label is found (or the registry lookup fails for any
  reason), the line falls back to just the hash, exactly as before —
  this is purely additive.

## Verification

- `go build main.go`, `go vet ./...`, `go test ./...` pass.
- Manually verified the registry auth discovery + manifest + config blob
  fetch end-to-end against `lscr.io/linuxserver/pairdrop` (the reported
  case): confirmed it resolves `org.opencontainers.image.version =
  v1.11.2-ls145` without pulling any image layers.

---

## 📝 Version History

- **v2.11.0** (Sep 28, 2026) - Compose progress bar + app version in notifications
- **v2.9.2** (Sep 22, 2026) - Progress bar for /trackimage's manual check
- **v2.9.1** (Sep 22, 2026) - Fix update_recreate button skipping the pull
- **v2.9.0** (Sep 22, 2026) - Live progress bar while pulling images
- **v2.8.0** (Sep 22, 2026) - Notify without pulling when ENABLE_AUTO_CHECK=false

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.9.2...v2.11.0
