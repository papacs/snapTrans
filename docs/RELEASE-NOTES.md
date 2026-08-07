# Release Notes Template

Copy this template for each release and fill in the details.

## v0.2.0 - 2026-08-04

### Added

- Persistent RapidOCR worker (residency toggle in Settings)
- Auto translation direction detection from OCR text
- Translation history (recent 50, copy/clear)
- Test Connection button in Settings
- Shortcut recorder in Settings
- Environment status badges (OCR / API key)
- Start with Windows (registry autostart)
- Custom translation prompt and glossary
- Auto-copy translation result
- Retry and Copy OCR actions in the result box
- Local diagnostic logs (`logs/snaptrans.log`)
- GitHub Actions CI

### Changed

- Settings window is resizable-height, draggable, and shows version
- Tray Capture menu item reflects the configured shortcut
- Faster startup: OCR engine warms up in the background

### Fixed

- Single-instance guard (named mutex)
- Stale translation events can no longer corrupt a newer selection (generation ids)
- Mixed-DPI / multi-monitor coordinate mapping
- Shortcut changes validate before saving and surface conflicts
- One-shot OCR retries with the documented `--image_path=` argument shape
- Worker process no longer killed by warm-up context cancellation

### Verification

- [ ] `go test ./...` passes
- [ ] `cd frontend && npm test && npm run typecheck && npm run build` passes
- [ ] `wails build` smoke-tested on Windows 10/11
