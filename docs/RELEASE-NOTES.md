# snapTrans v0.2.1

发布日期：2026-09-17

这是一个以翻译响应速度和 Windows 构建可靠性为重点的维护版本。下载
`snapTrans-v0.2.1-windows-x64.zip` 后完整解压，直接运行 `snapTrans.exe`；
RapidOCR-json 和所需模型已包含在压缩包内。

此版本尚未进行代码签名，Windows SmartScreen 可能在首次运行时显示未知发布者提示。

### Changed

- DeepSeek V4 翻译请求默认关闭 thinking，兼容
  `deepseek/deepseek-v4-flash` 等带供应商前缀的模型 ID
- 按下翻译快捷键时预热 LLM 的 TLS / HTTP 连接，与截屏、拖选和 OCR 并行执行
- 预热请求使用 `/models`，不调用模型生成；设置 3 秒超时和 60 秒节流
- Windows 构建会自动准备并校验固定版本的 RapidOCR-json 与模型文件
- 桌面应用代码整理到 `internal/desktop`，保持 Wails 绑定入口精简
- 项目网站与中英文说明更新

### Fixed

- 修复兼容网关未关闭 DeepSeek V4 thinking，导致正文前出现数秒隐藏推理等待的问题
- 修复冷 TLS / HTTP 连接占用首次翻译关键路径的问题
- 修复从 `build/bin` 运行时相对 RapidOCR 路径解析不稳定的问题
- OCR 依赖准备脚本增加固定哈希校验、安全解压和本地目录复用

### Verification

- [x] `go test -count=1 ./...` passes
- [x] `cd frontend && npm test` passes
- [x] `cd frontend && npm run typecheck` passes
- [x] `cd frontend && npm run build` passes
- [x] OCR dependency setup tests pass
- [x] `wails build` completes on Windows

### Package contents

- `snapTrans.exe`
- RapidOCR-json v0.2.0 executable and models
- Project README and setup instructions
- snapTrans MIT license
- RapidOCR-json third-party notice and MIT license
