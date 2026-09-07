# Changelog v2.6.0 - Full English/Spanish Support

**Version**: 2.6.0
**Release Date:** September 7, 2026

## Summary

Botainer is now fully bilingual. Every user-facing message — command
outputs, button labels, confirmations, error messages, notifications, and
menu descriptions — is routed through the existing `getText()` locale
system, backed by `locale/es.json` and `locale/en.json`.

Closes #25.

## What changed

- Extracted **~300 previously hardcoded Spanish strings** across all of
  `main.go` into `locale/es.json` / `locale/en.json`, using the existing
  `getText(key, args...)` helper with `$1`, `$2`, ... placeholders.
- Both locale files now have **364 keys each**, fully in sync (verified: no
  missing keys on either side).
- Covers every command and callback flow: container list/inspect/actions,
  Docker Compose management, image/Helm chart tracking, auto-update,
  rollback, templates, maintenance mode, alerts, health checks, scheduled
  reports, audit log, vulnerability scanning, webhooks, update policies,
  registries, cleanup, and port management.
- Fixed a `go vet` warning (non-constant format string) introduced while
  wrapping a few `fmt.Errorf` calls with `getText()`.

## Not changed (by design)

- Two internal error strings that are only ever written to logs / wrapped
  Go errors (never shown directly to the Telegram user) were intentionally
  left as-is, since translating internal diagnostics has no user-facing
  value.
- Telegram's `setMyCommands` (the `/` command menu) still reflects a single
  language — whichever `LANGUAGE` the bot was started with — since Telegram
  requires per-`language_code` registration for true per-user localization,
  which is out of scope for this change.

## How to use

Set `LANGUAGE=en` (or `LANGUAGE=es`, the default) in your `.env`. All
command output, buttons, and notifications will follow that language.

---

## 📝 Version History

- **v2.6.0** (Sep 7, 2026) - Full English/Spanish support (i18n)
- **v2.5.0** (Sep 7, 2026) - Mini App removed by default (breaking change)
- **v2.4.3** (Sep 4, 2026) - Security fix: Mini App API authorization bypass, CORS hardening
- **v2.4.2** (May 2026) - Fix compose update recreate, runComposeCmd with correct workdir
- **v2.4.1** (May 17, 2026) - Code quality, panic fixes, resolveComposeFile helper
- **v2.4.0** (May 17, 2026) - Logs fix, update reliability, new notification format

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.5.0...v2.6.0
