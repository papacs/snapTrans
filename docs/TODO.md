# Engineering TODO

This file records the practical details that should not be lost while the MVP is being built.

## P0 - Must Validate Before First Release

- Verify Wails v2 transparent, frameless, always-on-top fullscreen behavior on Windows 10 and Windows 11.
- Verify the app hides correctly after `Restore` and wakes correctly after the global shortcut.
- Validate global hotkey registration on Windows with `Alt+Q`, changed shortcuts, and duplicate shortcuts used by other software.
- Validate DPI mapping at 100%, 125%, 150%, and mixed-DPI multi-monitor setups.
- Validate multi-monitor capture when monitors have negative coordinates.
- Confirm `rapidocr_json.exe` CLI argument shape and exact JSON schema.
- Add cancellation so closing/restoring a result stops OCR and translation work.
- Confirm DeepSeek stream behavior, timeout handling, and empty OCR text handling with a real API key.
- Ensure API keys are never logged, emitted to frontend events, or committed.

## P1 - UX and Reliability

- Add a small tray entry so users can open settings when the window is hidden.
- Add clear error states inside the in-place result box for missing OCR binary, missing API key, OCR failure, and network failure.
- Add retry from the result box without requiring a new screenshot.
- Improve shortcut parser coverage and validation messages.
- Persist window/runtime state defensively so stale translation tokens cannot update a new selection.
- Add markdown sanitization review for rendered translation content.
- Add telemetry-free crash/error logging to a local file.

## P2 - Packaging and Open Source Polish

- Add app icon and Windows installer metadata.
- Add GitHub Actions for frontend tests, Go tests, and Wails build smoke checks.
- Add `LICENSE` after choosing the license.
- Add screenshots/GIFs to README.
- Add contribution guidelines.
- Add release notes template.

## Environment Status

Go and Wails are installed on this machine. `go test ./...` and `wails build` have been verified locally.
