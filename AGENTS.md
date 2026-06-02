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

## Local Setup

1. Install Go 1.21+.
2. Install Wails v2 CLI.
3. Run `npm install` inside `frontend/`.
4. Copy `.env.sample` to `.env`.
5. Fill `DEEPSEEK_API_KEY`.
6. Put `rapidocr_json.exe` at the project root or set `RAPIDOCR_EXE_PATH`.

## Verification

- Frontend unit tests: `cd frontend && npm test`
- Frontend type check: `cd frontend && npm run typecheck`
- Frontend build: `cd frontend && npm run build`
- Backend tests after Go is installed: `go test ./...`
- Desktop dev run after Wails is installed: `wails dev`

