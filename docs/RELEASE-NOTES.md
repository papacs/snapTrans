# snapTrans v0.2.0

发布日期：2026-09-04

这是 snapTrans 的首个公开 Windows 便携版。下载
`snapTrans-v0.2.0-windows-x64.zip` 后完整解压，直接运行 `snapTrans.exe`；
RapidOCR-json 和所需模型已包含在压缩包内。

此版本尚未进行代码签名，Windows SmartScreen 可能在首次运行时显示未知发布者提示。

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
- Screenshot annotations: rectangle, ellipse, arrow, pen, mosaic, text and numbered callouts
- Manual scrolling capture with a stitched preview
- Rule-based table extraction with editable TSV / Markdown export

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

- [x] `go test ./...` passes
- [x] `cd frontend && npm test && npm run typecheck && npm run build` passes
- [x] `wails build` completes on Windows

### Package contents

- `snapTrans.exe`
- RapidOCR-json v0.2.0 executable and models
- Project README and setup instructions
- snapTrans MIT license
- RapidOCR-json third-party notice and MIT license
