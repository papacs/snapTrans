<div align="center">

# snapTrans

**面向 Windows 的轻量级选词翻译、截图翻译与截图标注工具**

选中文字后按原有快捷键，直接在原位置显示译文；没有选中文字时，框选屏幕，松手即翻译。

[![Windows](https://img.shields.io/badge/platform-Windows_10%2F11-0078D4?logo=windows11&logoColor=white)](https://www.microsoft.com/windows/)
[![Wails](https://img.shields.io/badge/Wails-v2-DF0000?logo=wails&logoColor=white)](https://wails.io/)
[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-42B883?logo=vuedotjs&logoColor=white)](https://vuejs.org/)
[![License](https://img.shields.io/badge/license-MIT-22c55e.svg)](LICENSE)

[快速开始](#-快速开始) · [核心功能](#-核心功能) · [配置说明](#%EF%B8%8F-配置说明) · [本地开发](#-本地开发) · [项目架构](#%EF%B8%8F-项目架构)

</div>

---

## ✨ 为什么选择 snapTrans

snapTrans 将“截图、识别、翻译、阅读”压缩成一次自然的鼠标动作。它不会在框选完成后弹出确认窗口，也不会打断当前工作流。

| 工作模式 | 默认快捷键 | 交互体验 |
| --- | --- | --- |
| 选词 / 截图翻译 | <kbd>Alt</kbd> + <kbd>Q</kbd> | 有可靠文字选区 → 直接原位翻译；否则冻结屏幕 → 框选 → 松手翻译 |
| 截图涂鸦 | <kbd>Alt</kbd> + <kbd>W</kbd> | 冻结屏幕 → 拖出选区 → 标注或向下滚动拼接 → 复制或保存 PNG |

> 两套快捷键互相独立。截图翻译始终保持“松手即翻译”，截图涂鸦则在选区旁显示紧凑工具栏。

## 🚀 核心功能

### 选词与截图翻译

- **同一个快捷键**：选中文字按 Alt+Q 直接翻译，跳过框选和 OCR；不用 Ctrl+C，不读取或改写剪贴板。
- **兼容兜底**：应用不提供可靠选区、跨屏或超出可见范围时回到框选；托盘 Capture 始终手动框选。文字读取限制 350ms，慢应用不会阻塞后续操作。

- **本地 OCR**：通过 RapidOCR 在本机完成文字识别。
- **原位覆盖**：译文直接显示在截图选区上，不需要切换窗口。
- **流式响应**：兼容 DeepSeek、LiteLLM 及其他 OpenAI-compatible API。
- **智能排版**：根据 OCR 文本块位置，在表格、表单和长文本之间自动选择合适布局。
- **自动方向**：自动判断中译英或英译中，也可以针对当前结果手动反向翻译。
- **高 DPI 支持**：处理多显示器、缩放比例和物理像素映射，减少选区偏移。

### 微信式截图涂鸦

- 矩形、椭圆、实心箭头、自由画笔
- 马赛克、文字、表情标注
- 自定义标注颜色与撤销
- 微信式手动滚动长截图：选区实时显示当前滚动画面，旁边同步预览完整长图；每次滚动由用户控制，点击完成后生成长图
- 完成后复制到剪贴板，或通过 Windows 原生对话框保存 PNG
- 工具栏自动贴合选区并避让屏幕边缘

### 桌面体验

- 全局快捷键可视化录入
- Windows 系统托盘：`Capture`、`Screenshot`、`Settings`、`Quit`
- RapidOCR 常驻预热，失败时自动回退到单次调用
- 最近 50 条翻译历史
- 自动复制、开机自启、连接测试和本地诊断日志
- 中英文设置界面与单实例保护

## 📦 快速开始

### 运行已构建版本

Windows 发布目录应保持如下结构：

```text
build/bin/
├── snapTrans.exe
└── RapidOCR-json_v0.2.0/
    ├── RapidOCR-json.exe
    └── models/
```

1. 运行 `build/bin/snapTrans.exe`，应用会进入系统托盘。
2. 右键托盘图标，打开 `Settings`。
3. 配置 API Key、Base URL 和模型名称。
4. 确认 RapidOCR 路径，保存设置。
5. 选中文字后按 <kbd>Alt</kbd> + <kbd>Q</kbd> 原位翻译；不选择文字则进入框选。<kbd>Alt</kbd> + <kbd>W</kbd> 仍用于截图涂鸦。

原生选词翻译不依赖 OCR，也不向模型发送图片；截图翻译仍需保留 RapidOCR。已验证自建 Windows EDIT 控件及 Chrome 测试页面的选区读取，其他应用取决于 UI Automation 支持情况。

RapidOCR 路径既可以填写文件夹，也可以填写完整可执行文件路径：

```text
./RapidOCR-json_v0.2.0
D:/Tools/RapidOCR-json_v0.2.0
D:/Tools/RapidOCR-json_v0.2.0/RapidOCR-json.exe
```

## ⚙️ 配置说明

默认配置与当前代码保持一致：

| 配置项 | 默认值 | 说明 |
| --- | --- | --- |
| 翻译快捷键 | `Alt+Q` | 优先选中文字直译，无可靠选区则框选翻译 |
| 截图涂鸦快捷键 | `Alt+W` | 松开鼠标后进入标注模式 |
| API Base URL | `https://api.deepseek.com` | 支持 OpenAI-compatible 服务 |
| 模型 | `deepseek-v4-flash` | 可替换为服务商暴露的模型 ID |
| RapidOCR 路径 | `./RapidOCR-json_v0.2.0` | 支持文件夹或 EXE 路径 |
| OCR 超时 | `15s` | 单次识别最大等待时间 |
| 翻译超时 | `60s` | 翻译接口无响应时的最大等待时间 |
| 自动识别方向 | 开启 | 根据源文本自动选择翻译方向 |
| OCR 常驻预热 | 开启 | 降低第一次及后续识别延迟 |

设置会保存到当前用户的系统配置目录。开发环境也可以通过根目录 `.env` 提供配置：

```dotenv
LLM_API_KEY=your_api_key
LLM_BASE_URL=https://your-openai-compatible-host/v1
LLM_MODEL=your-model-id

RAPIDOCR_EXE_PATH=./RapidOCR-json_v0.2.0
RAPIDOCR_TIMEOUT_SECONDS=15
SNAPTRANS_TRANSLATION_TIMEOUT_SECONDS=60

SNAPTRANS_SHORTCUT=Alt+Q
SNAPTRANS_SCREENSHOT_SHORTCUT=Alt+W
SNAPTRANS_AUTO_DIRECTION=true
SNAPTRANS_PERSISTENT_OCR=true
SNAPTRANS_AUTO_COPY=false
```

> 请勿提交真实 API Key、`.env`、OCR 二进制文件或本地配置。

## 🛠 本地开发

### 环境要求

- Windows 10/11
- Go 1.22+
- Node.js 22+ 与 npm
- Wails v2 CLI
- RapidOCR-json v0.2.0
- DeepSeek、LiteLLM 或其他 OpenAI-compatible API 凭据

### 安装与启动

```powershell
git clone https://github.com/papacs/snapTrans.git
cd snapTrans

Copy-Item .env.sample .env

cd frontend
npm install
cd ..

wails dev
```

仅预览前端界面：

```powershell
cd frontend
npm run dev
```

浏览器预览模式会生成模拟截图和流式翻译事件，不依赖 Wails 或本地 OCR。

### 构建

```powershell
cd frontend
npm run build
cd ..

wails build
```

## ✅ 质量验证

```powershell
# 前端测试、类型检查与生产构建
cd frontend
npm test
npm run typecheck
npm run build

# Go 后端测试
cd ..
go test ./...
```

重点测试范围包括：

- DPI 与截图选区坐标映射
- 截图/翻译状态切换和过期事件隔离
- OCR 输出解析、常驻 worker 与回退策略
- 配置迁移、双快捷键和历史记录
- 翻译布局、流式输出与截图标注几何

## 🏗️ 项目架构

```text
snapTrans/
├── app.go                         # Wails 绑定、工作流、托盘与快捷键
├── screen_windows.go              # Windows 窗口/显示器能力
├── internal/
│   ├── capture/                   # 屏幕捕获、DPI 与显示器元数据
│   ├── config/                    # 用户配置、默认值与环境变量
│   ├── hotkeys/                   # 全局快捷键注册
│   ├── ocr/                       # RapidOCR worker、解析与回退
│   ├── translator/                # OpenAI-compatible 流式翻译
│   ├── history/                   # 本地翻译历史
│   ├── autostart/                 # Windows 开机自启
│   ├── logfile/                   # 本地诊断日志
│   └── singleinstance/            # 单实例保护
└── frontend/src/
    ├── App.vue                    # 捕获、翻译和设置主状态机
    ├── components/                # 截图标注等独立组件
    ├── services/backend.ts        # Wails 调用边界与浏览器 fallback
    ├── utils/selection.ts         # 坐标映射、裁剪与翻译布局
    └── utils/annotations.ts       # 标注几何与 Canvas 渲染
```

核心数据流：

```text
全局快捷键
   ↓
冻结当前显示器 → 框选区域 → 本地 RapidOCR
   ↓                           ↓
截图涂鸦 / 复制 / 保存        仅发送识别后的文本
                               ↓
                     OpenAI-compatible 流式翻译
                               ↓
                         原屏幕区域覆盖显示
```

## 🔐 隐私与安全

- 截图和 OCR 处理保留在本机；翻译服务仅接收 OCR 提取出的文本。
- API Key 保存在用户本地配置中，不应写入源码或提交到 Git。
- snapTrans 不包含遥测；错误和耗时只写入本地日志。
- 发布包需要由使用者自行准备 RapidOCR 程序与模型文件。

## 🤝 参与贡献

欢迎提交 Issue 和 Pull Request。修改代码时请优先保证：

1. 松开鼠标后立即翻译，不增加确认步骤。
2. DPI、多显示器和坐标映射保持正确。
3. 前后端调用统一通过明确边界，并为状态转换和解析行为补充测试。
4. 不提交 API Key、OCR 二进制、生成构建产物或本地配置。

更多开发约定请查看 [AGENTS.md](AGENTS.md)，待办事项请查看 [docs/TODO.md](docs/TODO.md)。

## 📄 License

本项目基于 [MIT License](LICENSE) 开源。

---

<div align="center">

**snapTrans — capture less, understand faster.**

</div>
