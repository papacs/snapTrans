# snapTrans

snapTrans is a small Windows desktop tool for instant screenshot translation.

The target experience is intentionally direct: press the shortcut, draw a box, release the mouse, and watch the translation stream over the original screen area.

## Current Status

This repository contains the MVP skeleton:

- Wails v2 app structure.
- Vue 3 capture overlay and in-place translation UI.
- Config loading from local `config.json` plus `.env` fallback.
- Local RapidOCR command wrapper.
- DeepSeek streaming translator boundary.
- Global shortcut plus Windows tray menu for capture, settings, and quit.
- Browser fallback for frontend-only development.
- Engineering TODO list for the details that still need tightening.
- Verified local Go/Wails build output at `build/bin/snapTrans.exe`.

## Requirements

- Windows 10/11
- Go 1.21+
- Wails v2 CLI
- Node.js 22+ and npm
- `rapidocr_json.exe`
- DeepSeek API key

## Quick Start

```powershell
cd E:\workspace\snapTrans
Copy-Item .env.sample .env
```

Edit `.env` and fill:

```dotenv
DEEPSEEK_API_KEY=your_deepseek_key
```

Install frontend dependencies:

```powershell
cd frontend
npm install
npm run build
```

Run backend tests and build the desktop app:

```powershell
cd E:\workspace\snapTrans
go test ./...
wails build
```

For live desktop development:

```powershell
wails dev
```

## Frontend-Only Preview

If Wails is not installed yet, the frontend can still be previewed with a simulated capture and streamed response:

```powershell
cd frontend
npm run dev
```

Open the Vite URL and use the small capture button in the top-right corner.

## Environment

`.env` is intentionally ignored by git.

```dotenv
DEEPSEEK_API_KEY=
DEEPSEEK_BASE_URL=https://api.deepseek.com
DEEPSEEK_MODEL=deepseek-chat
RAPIDOCR_EXE_PATH=./RapidOCR-json_v0.2.0
RAPIDOCR_TIMEOUT_SECONDS=15
SNAPTRANS_SHORTCUT=Alt+Q
```

## Shortcut

The MVP default shortcut is `Alt+Q`. The settings modal can save a shortcut string, and the backend contains the parser for common combinations such as `Alt+Q`, `Ctrl+Shift+S`, and `Alt+Space`.

## OCR Binary

Unzip the RapidOCR release so `RapidOCR-json.exe` and the `models` folder stay together. The recommended Windows package layout is:

```text
build/bin/
  snapTrans.exe
  RapidOCR-json_v0.2.0/
    RapidOCR-json.exe
    models/
```

Then set `RapidOCR Path` to either the folder or the executable:

```text
E:/workspace/snapTrans/build/bin/RapidOCR-json_v0.2.0
E:/workspace/snapTrans/build/bin/RapidOCR-json_v0.2.0/RapidOCR-json.exe
```

Relative paths are also supported, so `./RapidOCR-json_v0.2.0` works when the folder is next to `snapTrans.exe`.

When the configured OCR path is relative, snapTrans checks these locations in order:

- Current working directory.
- The directory containing `snapTrans.exe`.
- The project root when running from `build/bin`.

The OCR wrapper runs `RapidOCR-json.exe --image=<temp_png>` from the RapidOCR executable directory, so the bundled `models` folder can be found. It expects JSON output and extracts text from common fields such as `text`, `rec_text`, and `content`.

## Tray and Cancellation

The app starts hidden and adds a Windows tray menu with Capture, Settings, and Quit actions. During a capture, press `Esc` or right-click to cancel and return to the hidden tray state.

## Architecture

- `app.go`: Wails-bound application methods and workflow events.
- `internal/config`: local config and `.env` fallback.
- `internal/capture`: screen capture and PNG data URL encoding.
- `internal/hotkeys`: global shortcut registration.
- `internal/ocr`: RapidOCR process wrapper and JSON text extraction.
- `internal/translator`: DeepSeek streaming translation.
- `frontend/src/services/backend.ts`: Wails bridge plus browser fallback.
- `frontend/src/utils/selection.ts`: CSS-to-image coordinate mapping and crop helpers.

## Development TODOs

See `docs/TODO.md`.
