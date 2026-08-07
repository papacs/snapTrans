# Contributing

Thanks for considering contributing to snapTrans.

## Development Setup

1. Install Go 1.21+, the Wails v2 CLI, and Node.js 22+.
2. `cd frontend && npm install`
3. Copy `.env.sample` to `.env` and fill in `LLM_API_KEY`.
4. Unzip `RapidOCR-json_v0.2.0` next to `build/bin/snapTrans.exe`, or set `RAPIDOCR_EXE_PATH`.

## Commands

- Frontend tests: `cd frontend && npm test`
- Frontend type check: `cd frontend && npm run typecheck`
- Frontend build: `cd frontend && npm run build`
- Backend tests: `go test ./...`
- Desktop dev run: `wails dev`
- Desktop build: `wails build`

## Guidelines

- Keep the immediate screenshot-to-translation flow as the top priority.
- Treat DPI scaling and multi-monitor coordinates as correctness-sensitive.
- Do not commit API keys, OCR binaries, generated build outputs, or local config files.
- Prefer small focused modules over a large `app.go`.
- Add or update tests for coordinate mapping, state transitions, OCR parsing, and config behavior.
- Use `frontend/src/services/backend.ts` as the frontend boundary for Wails calls and browser fallback behavior.
- Keep tray actions and global hotkey behavior aligned: Capture enters selection mode, Settings opens the compact centered window, Quit terminates the app.

## Release Checklist

1. Run the full verification suite (frontend tests, type check, build; `go test ./...`).
2. Bump `appVersion` in `app.go` and note changes in `docs/RELEASE-NOTES.md`.
3. Run `wails build` and smoke-test the packaged exe: capture, translate, settings, tray, quit.
