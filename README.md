# snapTrans

snapTrans 是一个极简 Windows 截图翻译工具。
snapTrans is a minimalist Windows screenshot translation tool.

目标体验很直接：按下全局快捷键，拖出选区，松开鼠标，然后在原屏幕区域上直接流式显示翻译结果。鼠标松开后不再出现确认按钮。
The intended experience is direct: press the global shortcut, draw a selection box, release the mouse, and watch the translation stream over the original screen area. No confirmation button appears after mouse release.

## 直接使用 / Direct Use

仓库已经包含可直接运行的 Windows 产物：
This repository includes a runnable Windows build:

```text
build/bin/
  snapTrans.exe
  RapidOCR-json_v0.2.0/
    RapidOCR-json.exe
    models/
```

快速启动：
Quick start:

1. 双击或运行 `build/bin/snapTrans.exe`。
   Run `build/bin/snapTrans.exe`.
2. 程序默认进入托盘，右键托盘图标打开 `Settings`。
   The app starts in the system tray. Right-click the tray icon and open `Settings`.
3. 填入 LiteLLM Virtual Key、API Base URL（通常以 `/v1` 结尾）和模型名，保持 `RapidOCR Path` 为默认值 `./RapidOCR-json_v0.2.0`。
   Fill the LiteLLM Virtual Key, API Base URL (usually ending in `/v1`), Model, and keep `RapidOCR Path` as `./RapidOCR-json_v0.2.0`.
4. 保存后按默认快捷键 `Alt+Q` 开始截图翻译。
   Save and press the default shortcut `Alt+Q` to capture and translate.

默认 OCR 路径会按顺序检查当前工作目录、`snapTrans.exe` 所在目录，以及从 `build/bin` 运行时对应的项目根目录。因此只要 `RapidOCR-json_v0.2.0` 放在 `snapTrans.exe` 旁边，默认配置即可工作。
The default OCR path checks the current working directory, the directory containing `snapTrans.exe`, and the project root when running from `build/bin`. Keeping `RapidOCR-json_v0.2.0` next to `snapTrans.exe` works with the default configuration.

不要提交真实 API key。配置会保存到当前用户的系统配置目录中。
Do not commit real API keys. Settings are saved in the current user's OS config directory.

## 从源码运行 / Run From Source

环境要求：
Requirements:

- Windows 10/11
- Go 1.21+
- Wails v2 CLI
- Node.js 22+ and npm
- LiteLLM Virtual Key or another OpenAI-compatible API key

安装前端依赖：
Install frontend dependencies:

```powershell
cd frontend
npm install
```

可选：复制环境变量示例，便于开发时从 `.env` 读取配置。
Optional: copy the environment example for local development.

```powershell
cd E:\workspace\snapTrans
Copy-Item .env.sample .env
```

编辑 `.env` 并填入自己的 key：
Edit `.env` and fill your own key:

```dotenv
LLM_API_KEY=your_litellm_virtual_key
LLM_BASE_URL=https://your-litellm-host/v1
LLM_MODEL=gemini/gemini-3.5-flash-lite
RAPIDOCR_EXE_PATH=./RapidOCR-json_v0.2.0
RAPIDOCR_TIMEOUT_SECONDS=15
SNAPTRANS_SHORTCUT=Alt+Q
SNAPTRANS_AUTO_DIRECTION=true
SNAPTRANS_PERSISTENT_OCR=true
SNAPTRANS_AUTO_COPY=false
```

构建前端并运行测试：
Build and test:

```powershell
cd frontend
npm test
npm run build

cd ..
go test ./...
```

开发模式：
Desktop development:

```powershell
wails dev
```

生产构建：
Production build:

```powershell
wails build
```

## 前端预览 / Frontend-Only Preview

如果还没有安装 Wails，也可以用模拟截图和流式响应预览前端：
If Wails is not installed yet, the frontend can still be previewed with a simulated capture and streamed response:

```powershell
cd frontend
npm run dev
```

打开 Vite URL，点击右上角的 `Capture` 按钮。
Open the Vite URL and use the `Capture` button in the top-right corner.

## 配置 / Configuration

默认配置：
Defaults:

```text
Shortcut: Alt+Q
LLM API Base URL: https://your-litellm-host/v1
LLM Model: gemini/gemini-3.5-flash-lite
RapidOCR Path: ./RapidOCR-json_v0.2.0
RapidOCR Timeout: 15 seconds
```

配置来源：
Configuration sources:

- Settings 窗口保存到用户配置目录。
  Settings window saves to the user's config directory.
- `.env` 仅用于本地开发兜底，并被 git 忽略。
  `.env` is only a local development fallback and is ignored by git.
- `.env.sample` 可以提交，真实 `.env` 不要提交。
  `.env.sample` can be committed; real `.env` files must not be committed.

## OCR Binary

RapidOCR 发布包必须保持 `RapidOCR-json.exe` 和 `models` 文件夹在一起。
The RapidOCR release must keep `RapidOCR-json.exe` and the `models` folder together.

推荐布局：
Recommended layout:

```text
build/bin/
  snapTrans.exe
  RapidOCR-json_v0.2.0/
    RapidOCR-json.exe
    models/
```

也支持将 `RapidOCR Path` 设置为文件夹或可执行文件：
`RapidOCR Path` can point to either the folder or the executable:

```text
./RapidOCR-json_v0.2.0
E:/workspace/snapTrans/build/bin/RapidOCR-json_v0.2.0
E:/workspace/snapTrans/build/bin/RapidOCR-json_v0.2.0/RapidOCR-json.exe
```

OCR 封装会在 RapidOCR 可执行文件目录下运行命令，这样可以正确找到同级 `models` 文件夹。
The OCR wrapper runs the command from the RapidOCR executable directory so the adjacent `models` folder can be found.

## 快捷键 / Shortcut

默认快捷键是 `Alt+Q`。Settings 窗口可以保存快捷键字符串，后端支持常见组合，例如 `Alt+Q`、`Ctrl+Shift+S` 和 `Alt+Space`。
The default shortcut is `Alt+Q`. The Settings window can save a shortcut string, and the backend parses common combinations such as `Alt+Q`, `Ctrl+Shift+S`, and `Alt+Space`.

## 托盘与取消 / Tray And Cancellation

应用启动后默认隐藏，并添加 Windows 托盘菜单，包含 `Capture`、`Settings` 和 `Quit`。截图过程中可以按 `Esc` 或右键取消，并回到隐藏托盘状态。
The app starts hidden and adds a Windows tray menu with `Capture`, `Settings`, and `Quit`. During capture, press `Esc` or right-click to cancel and return to the hidden tray state.

## 更多功能 / Additional Features

- **单实例 / Single instance**: 重复启动会提示已有实例，避免热键与托盘冲突。
  Starting a second instance shows a message box instead of duplicating the tray and hotkey.
- **常驻 OCR / Persistent OCR**: RapidOCR 以 stdin 循环模式常驻运行并随应用预热，显著降低每次截图的冷启动延迟；失败时自动回退到单次调用。常驻期间任务管理器里可见一个 `RapidOCR-json.exe` 进程，退出应用时自动关闭；可在设置中关闭常驻以省内存。
  RapidOCR runs as a persistent stdin-loop worker and warms up at startup; failures fall back to one-shot invocations. While resident, one `RapidOCR-json.exe` process is visible in Task Manager and exits with the app; the setting can disable residency to save memory.
- **自动方向 / Auto direction**: 默认按 OCR 文本自动选择翻译方向（中文为主 → 译英，否则译中），可在设置中关闭；手动反向在本次结果内保持，下次截图恢复自动。
  The translation direction is detected from the OCR text by default; the reverse button overrides it for the current result.
- **翻译历史 / History**: 最近 50 条翻译保存在设置窗口，可复制或清空。
  The latest 50 translations are shown in Settings and can be copied or cleared.
- **测试连接 / Test connection**: 设置页一键校验 API key、Base URL 与模型。
  One-click validation of the LLM endpoint in Settings.
- **快捷键录制 / Shortcut recorder**: 设置页点击 Record 后直接按键生成快捷键字符串。
  Press the Record button, then the desired key combination.
- **启动自检 / Environment status**: 设置页显示 OCR 可执行文件与 API key 状态徽章。
  Settings shows OCR and API key readiness badges.
- **开机自启 / Autostart**: 设置页开关注册到注册表 Run 键。
  Start with Windows is managed through the registry Run key.
- **自定义提示词与术语表 / Custom prompt & glossary**: 可选地为翻译模型追加指令和术语对照表。
  Optional extra instructions and a source->target glossary for the model.
- **自动复制 / Auto-copy**: 设置中开启后，翻译完成自动复制结果到剪贴板。
  When enabled in Settings, the translation result is copied to the clipboard automatically.
- **本地日志 / Logs**: 错误写入用户配置目录 `logs/snaptrans.log`（无遥测），包含各阶段耗时。
  Errors and per-stage timings are logged locally without any telemetry.

## 架构 / Architecture

- `app.go`: Wails 绑定方法和工作流事件。
- `internal/config`: 本地配置和 `.env` fallback。
- `internal/capture`: 屏幕截图、PNG data URL 编码和按显示器 DPI 的坐标元数据。
- `internal/hotkeys`: 全局快捷键注册。
- `internal/ocr`: RapidOCR 常驻 worker（stdin 循环）与单次调用封装、JSON 文本提取。
- `internal/translator`: OpenAI-compatible streaming translation (LiteLLM supported)。
- `internal/history`: 翻译历史持久化。
- `internal/autostart`: Windows 注册表开机自启。
- `internal/logfile`: 本地诊断日志。
- `internal/singleinstance`: 单实例互斥。
- `frontend/src/services/backend.ts`: Wails bridge 和浏览器 fallback。
- `frontend/src/utils/selection.ts`: CSS 到图片坐标映射（含多显示器 DPI 缩放）、裁剪和翻译排版辅助函数。
- `frontend/src/utils/shortcut.ts`: 快捷键录制解析。

## 验证 / Verification

前端测试、类型检查和构建：
Frontend tests, type check, and build:

```powershell
cd frontend
npm test
npm run typecheck
npm run build
```

后端测试：
Backend tests:

```powershell
go test ./...
```

桌面开发运行：
Desktop dev run:

```powershell
wails dev
```

## 开发待办 / Development TODOs

详见 `docs/TODO.md`。
See `docs/TODO.md`.
