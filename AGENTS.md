# AGENTS.md

## Project Goal

snapTrans is a minimalist Windows screenshot translation tool. The intended UX is:

1. Press a global shortcut.
2. Freeze the current screen.
3. Drag a selection box.
4. Release the mouse.
5. OCR and stream translation in place over the selected region.

No confirm button should appear between mouse release and translation start.

## Tech Stack

- Desktop shell: Wails v2
- Backend: Go 1.21+
- Frontend: Vue 3, TypeScript, Vite, Tailwind CSS
- OCR: local `rapidocr_json.exe`
- Translation: DeepSeek-compatible OpenAI streaming API through `sashabaranov/go-openai`

## Development Rules

- Keep the immediate screenshot-to-translation flow as the top priority.
- Treat DPI scaling and multi-monitor coordinates as correctness-sensitive.
- Do not commit API keys, OCR binaries, generated build outputs, or local config files.
- Prefer small focused modules over a large `app.go`.
- Add or update tests for coordinate mapping, state transitions, OCR parsing, and config behavior.
- Use `frontend/src/services/backend.ts` as the frontend boundary for Wails calls and browser fallback behavior.
- Keep tray actions and global hotkey behavior aligned: Capture should enter selection mode, Settings should open a compact centered window, and Quit should terminate the app.

## Local Setup

1. Install Go 1.21+.
2. Install Wails v2 CLI.
3. Run `npm install` inside `frontend/`.
4. Copy `.env.sample` to `.env`.
5. Fill `DEEPSEEK_API_KEY`.
6. On Windows x64, `wails build` / the initial `wails dev` build automatically prepares RapidOCR and models. The first build needs GitHub Releases and npm registry access. To reuse local files without moving them, run `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/ensure-rapidocr.ps1 -SourceDirectory <OCR folder>` first.

## Verification

- Frontend unit tests: `cd frontend && npm test`
- Frontend type check: `cd frontend && npm run typecheck`
- Frontend build: `cd frontend && npm run build`
- Backend tests after Go is installed: `go test ./...`
- OCR dependency setup tests: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/ensure-rapidocr.test.ps1`
- Desktop dev run after Wails is installed: `wails dev`
