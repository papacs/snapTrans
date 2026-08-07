# Engineering TODO

This file records the practical details that should not be lost while the MVP is being built.

## P0 - Must Validate Before First Release

- [ ] Verify Wails v2 transparent, frameless, always-on-top fullscreen behavior on Windows 10 and Windows 11.
- [ ] Verify the app hides correctly after `Restore` and wakes correctly after the global shortcut.
- [ ] Validate global hotkey registration on Windows with `Alt+Q`, changed shortcuts, and duplicate shortcuts used by other software.
- [ ] Validate DPI mapping at 100%, 125%, 150%, and mixed-DPI multi-monitor setups.
- [ ] Validate multi-monitor capture when monitors have negative coordinates.
- [ ] Confirm `rapidocr_json.exe` CLI argument shape and exact JSON schema.
- [ ] Confirm DeepSeek stream behavior, timeout handling, and empty OCR text handling with a real API key.
- [ ] Review the default DeepSeek model before `deepseek-chat` deprecation on 2026-07-24.
- [ ] Ensure API keys are never logged, emitted to frontend events, or committed.

## P1 - UX and Reliability

- [x] Add clear error states inside the in-place result box for missing OCR binary, missing API key, OCR failure, and network failure.
- [x] Add retry from the result box without requiring a new screenshot.
- [x] Improve shortcut parser coverage and validation messages.
- [x] Persist window/runtime state defensively so stale translation tokens cannot update a new selection.
- [x] Add markdown sanitization review for rendered translation content.
- [x] Add telemetry-free crash/error logging to a local file.
- [ ] Improve inline replacement quality by sampling per-block background more precisely and supporting multi-line OCR block layout without oversized labels.
- [x] Profile end-to-end latency from mouse release to first translated character, including capture crop, RapidOCR startup, OCR runtime, DeepSeek first token, and frontend paint.
- [x] Investigate a persistent OCR worker/server mode to avoid RapidOCR process cold-start latency after every selection.

## P2 - Packaging and Open Source Polish

- [x] Add app icon and Windows installer metadata.
- [x] Add GitHub Actions for frontend tests, Go tests, and Wails build smoke checks.
- [x] Add `LICENSE` after choosing the license.
- [ ] Add screenshots/GIFs to README.
- [x] Add contribution guidelines.
- [x] Add release notes template.

## Environment Status

Go and Wails are installed on this machine. `go test ./...` and `wails build` have been verified locally.

## Completed During MVP Hardening

- Added Windows tray actions for Capture, Settings, and Quit.
- Added right-click and `Esc` cancellation during capture selection.
- Resolved relative RapidOCR paths from the working directory, the executable directory, and the project root when running from `build/bin`.
- Single instance guard, workflow generation ids, per-display DPI coordinate mapping.
- Persistent RapidOCR worker with one-shot fallback and residency toggle.
- Auto translation direction detection, translation history, connection test, shortcut recorder, environment status badges, autostart, custom prompt/glossary, auto-copy, local logs, CI.
