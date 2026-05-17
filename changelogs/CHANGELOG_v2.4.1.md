# Changelog v2.4.1 - Code Quality & Panic Fixes

**Version**: 2.4.1
**Release Date:** May 17, 2026

## Summary

Botainer v2.4.1 is a code quality and stability release. It fixes a runtime panic in the webapp metrics API, removes duplicate UI code, and introduces a shared `resolveComposeFile()` helper that eliminates repeated patterns across all update flows.

---

## 🐛 Bug Fixes

### `api/metrics.go` — Runtime Panic on Empty Container Names
- **Root cause**: `c.Names[0]` panics when a container has no names (e.g., during creation or in some edge states).
- **Fix**: Replaced all `c.Names[0]` accesses in `api/metrics.go` with `containerFirstName(c)`, which safely returns an empty string.

### `handleStart` — Duplicate Keyboard
- Removed an unnecessary `if/else` block that rendered the same keyboard twice.
- Now always renders a single, consistent main menu keyboard.

---

## ♻️ Refactor

### `resolveComposeFile()` Helper
- New helper function: `resolveComposeFile(project string) (workDir, composeFile string, err error)`
- Wraps the repeated `getComposeWorkDir(project)` + `findComposeFile(workDir)` pattern.
- Replaced 4 independent repetitions across:
  - `/updateall` pre-validation
  - `newtag_update` callback
  - `compose_pullup_service` callback
  - Auto-update inside `runImageUpdateCheck()`
- Returns a descriptive error if the project workdir or compose file is not found.

---

## 📝 Version History

- **v2.4.1** (May 17, 2026) - Code quality, panic fixes, resolveComposeFile helper
- **v2.4.0** (May 17, 2026) - Logs fix, update reliability, new notification format
- **v2.3.1** (May 7, 2026) - Compact UI Redesign
- **v2.3.0** (May 7, 2026) - Collaboration Suite
- **v2.2.1** (May 6, 2026) - Complete Management Suite
- **v2.2.0** (May 6, 2026) - Advanced Management Release
- **v2.0.0** (May 6, 2026) - Telegram Mini App Release

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.4.0...v2.4.1
