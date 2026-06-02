# snapTrans MVP Design

## Goal

Build an open-source MVP skeleton for a minimalist Windows screenshot translation app: shortcut, frozen screen, drag selection, mouse release, OCR, streaming translation, and in-place controls.

## Scope

The MVP must provide a usable project structure and the first end-to-end workflow. It should be testable as a frontend-only preview without Wails and testable as a desktop app once Go and Wails are installed.

## Architecture

The backend is a Wails v2 Go app with focused internal packages:

- `internal/config`: load/save local settings and parse `.env` fallback.
- `internal/capture`: capture all monitors, merge them into one PNG, and return a data URL.
- `internal/hotkeys`: register the global shortcut.
- `internal/ocr`: call `rapidocr_json.exe`, parse JSON output, and return extracted text.
- `internal/translator`: stream DeepSeek translation tokens.
- `app.go`: expose Wails methods and emit workflow events.

The frontend is Vue 3 with TypeScript:

- The canvas renders the frozen screen data URL.
- A selection rectangle is tracked in CSS pixels.
- `selection.ts` maps CSS pixels to source image pixels for DPI-safe cropping.
- The selected crop is sent to `ProcessImage` immediately on mouse release.
- Events append tokens into an absolute-positioned result box.
- Browser fallback simulates capture and streaming translation when Wails is absent.

## Event Contract

The backend emits:

- `capture-start`: full screen image and dimensions.
- `ocr-start`: crop accepted and OCR is starting.
- `translation-start`: OCR text exists and translation is starting.
- `translation-token`: streamed token text.
- `translation-done`: stream completed.
- `workflow-error`: recoverable workflow error.

## Configuration

User config is stored outside the repo by default, under the OS app data directory. `.env` is used only as a development fallback. Required keys:

- `DEEPSEEK_API_KEY`
- `DEEPSEEK_BASE_URL`
- `DEEPSEEK_MODEL`
- `RAPIDOCR_EXE_PATH`
- `RAPIDOCR_TIMEOUT_SECONDS`
- `SNAPTRANS_SHORTCUT`

## Testing

Frontend tests cover coordinate normalization and DPI-safe mapping. Backend tests cover config loading and OCR JSON text extraction. Browser fallback allows visual inspection without Wails.

## Constraints

- No extra confirm click after mouse release.
- No API keys committed to the repository.
- No blocking OCR/API work on the Wails main thread.
- Go/Wails verification requires installing missing local tooling.

