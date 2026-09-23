
# Changelog v2.9.1 - Fix update_recreate button skipping the pull

**Version**: 2.9.1
**Release Date:** September 22, 2026

## Summary

The "🔄 Update" button shown on a "🔔 Update available" notification for a
standalone container (`update_recreate` callback) now actually pulls the new
image (with the v2.9.0 progress bar) before recreating the container. It
previously recreated using whatever image was already cached locally.

## Why

`update_recreate` called `recreateWithNewImage()`, which recreates a
container using its currently configured image tag without pulling —
correct only under the assumption that the new image had already been
pulled during the check that triggered the notification. That assumption
held before v2.8.0, when `runImageUpdateCheck()` always pulled the image to
detect the digest change in the first place.

Since v2.8.0, when `ENABLE_AUTO_CHECK=false`, that same detection is done
with a lightweight registry digest check that pulls nothing. So the
notification could now fire without the new image ever being downloaded,
and clicking "Update" would recreate the container with the same (old)
image — a functional regression introduced by v2.8.0, not just a missing
progress bar.

## What changed

- `update_recreate` now calls `recreateContainer(target, chatID,
  messageID)` (pull with progress + recreate) instead of
  `recreateWithNewImage(target)` (recreate only, no pull).
- In full-pull mode (`ENABLE_AUTO_CHECK=true`) this just re-pulls an image
  that's already cached — Docker reports every layer as "Already exists"
  almost instantly, so the extra pull is effectively a fast no-op.
- `diagnose_recreate` (unrelated: just restarts a container with its
  current image, not tied to an update notification) is unchanged.

## Verification

- `go build main.go`, `go vet ./...`, `go test ./...` pass.

---

## 📝 Version History

- **v2.9.1** (Sep 22, 2026) - Fix update_recreate button skipping the pull
- **v2.9.0** (Sep 22, 2026) - Live progress bar while pulling images
- **v2.8.0** (Sep 22, 2026) - Notify without pulling when ENABLE_AUTO_CHECK=false
- **v2.7.0** (Sep 16, 2026) - Pin ImagePull to the daemon's native platform

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.9.0...v2.9.1
