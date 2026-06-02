
# Role: Expert Wails & Vue 3 Desktop Application Developer

## Project Overview
You are tasked with building a high-performance, minimalist Windows screenshot translation tool using Wails v2 (Go + Vue 3). 
The core UX bypasses traditional "confirm" steps: Shortcut -> Freeze Screen -> Draw Bounding Box -> Release Mouse -> Translate IN-PLACE over the original box -> Show "Copy/Restore" buttons.

## Tech Stack
- **Framework:** Wails v2 (Go backend, Web frontend)
- **Backend:** Go 1.21+
- **Frontend:** Vue 3 (Composition API, `<script setup>`), TypeScript, Tailwind CSS
- **Local OCR:** `RapidOCR-json.exe` (called via Go `os/exec`)
- **LLM API:** DeepSeek API (using `sashabaranov/go-openai` for streaming)

## Core Architecture & Wails Configuration
- **Window State:** Frameless, fully transparent background (`rgba(0,0,0,0)`), fullscreen state on activation, hidden on blur.
- **IPC Communication:** Use `wails.EventsEmit` to push streaming text and UI state changes from Go to Vue. Use bound methods for Vue to call Go.

## Development Sprints (Execute Sequentially)

### Sprint 1: Go Backend Foundation (Config, Hotkey, & Capture)
1. **Config Management:** Create a `config.json` local store to save `ShortcutKey` (default: `Alt+Q`) and `DeepSeekAPIKey`. Expose `SaveConfig` and `LoadConfig` methods to Vue.
2. **Global Hotkey:** Use `golang-design/hotkey` to register the global shortcut. When pressed, trigger the screenshot workflow.
3. **Screenshot:** Use `kbinani/screenshot` to capture all monitors. Convert the raw image to Base64.
4. **Trigger Frontend:** Emit an event `capture-start` with the Base64 image to wake up the Vue frontend.

### Sprint 2: Local OCR & DeepSeek API (Go Core Logic)
1. **Local OCR Wrapper:** Create a Go function `ExtractText(base64Image)`. 
   - Write the base64/temp image to disk.
   - Execute a local binary via `os/exec`: `./rapidocr_json.exe --image_dir temp.jpg`.
   - Parse the stdout JSON to extract the raw string. Delete the temp file.
2. **DeepSeek Streaming:** Create `TranslateText(text string)`.
   - Use `go-openai` with custom BaseURL `https://api.deepseek.com/v1`.
   - Set `Stream: true`. 
   - As chunks arrive, use `wails.EventsEmit("translation-token", chunk)` to push them instantly to the frontend.
3. **Pipeline Method:** Expose a bound method `ProcessImage(base64Crop)` that sequentially calls `ExtractText` and `TranslateText`.

### Sprint 3: Vue 3 Frontend (The In-Place UX)
1. **Capture Mask:** When receiving `capture-start`, render the full-screen Base64 image on a `<canvas>`. Overlay a `bg-black/40` mask.
2. **Bounding Box Interaction:** 
   - `mousedown`: Start drawing the transparent selection rectangle.
   - `mousemove`: Update dimensions.
   - `mouseup`: INSTANTLY stop. Do NOT show confirm buttons. 
   - Calculate the `x, y, width, height` of the drawn box. Crop this specific area from the original canvas to Base64 and pass it to Go's `ProcessImage`.
3. **In-Place Result Box:** 
   - Hide the drawing mask but keep the background frozen.
   - Render an absolute-positioned `div` exactly at the `left: x`, `top: y` coordinates of the bounding box. Make its `width` match the bounding box or slightly wider.
   - Style: `backdrop-blur-md bg-white/85 dark:bg-zinc-900/85 shadow-2xl rounded-lg p-4 transition-all`.
   - Show a loading spinner until the first `"translation-token"` arrives.
   - Append incoming tokens to a `ref` variable and render with a typewriter effect. Use `markdown-it` to render basic formatting.

### Sprint 4: Action Buttons & Settings
1. **Bottom Action Bar:** Below the translated text inside the result box, render a minimal flex container with two buttons:
   - **Copy:** Copies the final translated text to the clipboard.
   - **Restore (Close):** Destroys the bounding box, clears the state, hides the Wails window, and waits for the next global hotkey press.
2. **Settings Panel:** Add a hidden gear icon that opens a simple modal to input the DeepSeek API Key and configure the shortcut.

## Critical Constraints & Anti-Patterns (DO NOT DO THESE)
- **NO Extra Clicks:** Zero buttons between releasing the mouse and the translation starting. The UX must feel instantaneous.
- **DPI Scaling:** Windows UI scaling (125%, 150%) WILL distort coordinates. Ensure frontend mouse coordinates are properly scaled to match the physical image pixels when cropping.
- **Non-Blocking Go:** Execute `ProcessImage` (OCR and API calls) in a goroutine. Never block the Wails main thread.

Begin by generating the Go backend code for Sprint 1 and Sprint 2. Ensure the code is modular and handles errors gracefully.