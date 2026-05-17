# Changelog v2.4.0 - Logs Fix & Update Reliability

**Version**: 2.4.0
**Release Date:** May 17, 2026

## Summary

Botainer v2.4.0 focuses on reliability: fixes the `/logs` command that was returning garbled output for non-TTY containers, improves the `/updateall` flow, fixes a deadlock in the webapp API, and introduces a new compact notification format with Compose project context.

---

## 🐛 Bug Fixes

### `/logs` and `/logfile` — Garbled Output Fixed
- **Root cause**: Docker non-TTY containers use a multiplexed stream with 8-byte frame headers. Reading the raw stream with `io.ReadAll` included those headers as garbage characters.
- **Fix**: Introduced `readContainerLogs()` helper using `stdcopy.StdCopy` from the Docker SDK, with automatic TTY detection.
- **Also fixed** in `api/handlers.go` (`handleContainerLogs`) for the webapp log viewer.
- Added UTF-8 sanitization to prevent invalid characters reaching Telegram.

### `/updateall` — Compose Project Lookup
- Fixed the pre-validation step that looked up the compose file for each container before updating.
- Now uses the unified `resolveComposeFile()` helper (see v2.4.1).

### Webapp API — Deadlock in Alert Config
- Fixed a potential deadlock in `api/alerts.go` where `save()` was called while holding the mutex.
- `save()` correctly does NOT re-acquire the mutex — it is called while `a.mu` is already held in `SetConfig()`/`DeleteConfig()`.

### Telegram Markdown Fallback
- `bot.Send` now retries without `ParseMode` when Telegram rejects a message due to Markdown formatting errors.
- Prevents silent message delivery failures.

---

## ✨ New Features

### Redesigned Notification Format
New compact notification format for image update alerts, with Compose project context:

```
🔔 Actualización disponible
`image:tag`
━━━━━━━━━━━━━━━━━━
📦 antes  `...abc123`
✅ ahora  `...def456`
💾 82.1 MB · 🐳 container_name
🗂 Proyecto: my_project
```

Both `runImageUpdateCheck()` and `checkTrackedImages()` use the new format.

---

## 🔧 Technical

- `readContainerLogs(ctx, cli, containerID, tail)` helper centralizes log reading with `stdcopy.StdCopy`
- All 5+ raw log reading sites in `main.go` replaced with the helper
- `api/handlers.go` uses the same pattern with TTY detection

---

## 📝 Version History

- **v2.4.0** (May 17, 2026) - Logs fix, update reliability, new notification format
- **v2.3.1** (May 7, 2026) - Compact UI Redesign
- **v2.3.0** (May 7, 2026) - Collaboration Suite
- **v2.2.1** (May 6, 2026) - Complete Management Suite
- **v2.2.0** (May 6, 2026) - Advanced Management Release
- **v2.0.0** (May 6, 2026) - Telegram Mini App Release

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.3.1...v2.4.0
