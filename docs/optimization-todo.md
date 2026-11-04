# 优化实施清单 / Optimization TODO

按优先级实施，每项完成后运行对应验证并勾选。
Verify each item before checking it off.

## P0 - 正确性 / 可靠性

- [x] P0-1 单实例锁：Windows named mutex，防止多实例导致热键/托盘冲突
- [x] P0-2 翻译 token 竞态防护：工作流 generation id，前端丢弃过期事件
- [x] P0-3 混合 DPI / 多显示器坐标映射修复：按显示器缩放因子精确映射
- [x] P0-4 配置保存与热键注册解耦：先注册后保存，热键冲突在设置页提示

## P1 - 实用模块

- [x] P1-5 OCR 常驻/预热，降低每次截图冷启动延迟
- [x] P1-6 语言自动检测：OCR 后自动选择翻译方向，可记忆手动选择
- [x] P1-7 翻译历史：最近 N 条记录，可回看/复制/清空
- [x] P1-8 设置页「测试连接」按钮
- [x] P1-9 快捷键录制器（按键捕获生成快捷键字符串）
- [x] P1-10 启动自检：OCR 可执行文件与 API key 状态提示
- [x] P1-11 开机自启选项（注册表）

## P2 - 增强 / 打磨

- [x] P2-12 本地快速词典扩展（常见 UI 词汇免 API 调用）
- [x] P2-13 自定义翻译提示词 / 术语表
- [x] P2-14 本地错误日志文件（无遥测）
- [x] P2-15 GitHub Actions CI（前端测试 / Go 测试 / Wails 构建冒烟）
- [x] P2-16 结果框「重试」按钮（失败后无需重新截图）

## 回归修复 / Bugfix Round

- [x] B-1 设置窗口过小导致内容被裁剪：窗口自适应屏幕高度 + 表单内部滚动
- [x] B-2 OCR worker 多行 JSON 输出解析：按括号平衡累积行（忽略字符串内花括号）
- [x] B-3 常驻 OCR 进程被预热 context 杀死（`CommandContext` 默认 Cancel）：改为显式生命周期管理
- [x] B-4 自动方向失效 bug：方向检测移到后端 OCR 之后，新增 `translation-direction` 事件回传
- [x] B-5 手动反向不记忆：反向按钮本次结果内保持手动方向，下次截图恢复自动
- [x] B-6 设置中修改 OCR 路径后 worker 自动重启
- [x] B-7 浏览器预览模式同步 `translation-direction` 事件
- [x] B-8 设置窗口黑色边框：移除透明窗口上的深色 backdrop-blur 遮罩，窗口纯透明只显示表单
- [x] B-9 设置窗口无法拖动：frameless 窗口手动拖拽（GetWindowPosition + 节流 SetWindowPosition）
- [x] B-10 常驻 OCR 进程说明 + 设置开关 `PersistentOCR`（默认开，可关闭为单次模式）

## 收尾优化 / Final Polish

- [x] C-1 Worker.Start 并发等待改为 ctx 感知（等待预热时不阻塞到超时）
- [x] C-2 用户取消（隐藏窗口）时不再重启 OCR 进程，仅真正超时才重启
- [x] C-3 processImage 增加 OCR 耗时 / 总耗时日志（`logs/snaptrans.log`）
- [x] C-4 自动复制翻译结果开关 `AutoCopy`
- [x] C-5 Esc 关闭设置窗口
- [x] C-6 设置页显示版本号（`GetVersion`）
- [x] C-7 markdown-it 显式 `html: false`（翻译输出 HTML 会被转义，防注入）

## 收官打磨 / Final Polish Round 2

- [x] D-1 托盘 Capture 菜单项实时显示当前快捷键（配置修改后同步）
- [x] D-2 设置页「Open Log Folder」按钮（Explorer 打开日志目录）
- [x] D-3 单次 OCR 兼容兜底：`--image=` 快速失败时自动改用文档化的 `--image_path=` 重试
- [x] D-4 原始 `docs/TODO.md` 清理：勾选已完成项，保留真实待验证项
- [x] D-5 `CONTRIBUTING.md` 贡献指南
- [x] D-6 `docs/RELEASE-NOTES.md` 发布说明模板

## 2026 优化轮 / Optimization Round 2

- [x] E-1 翻译整体超时：新增 `translationTimeoutSeconds`（默认 60s），API 挂起时自动取消并在结果框提示，避免无限转圈
- [x] E-2 API Key DPAPI 加密：Windows `CryptProtectData` 用户级加密落盘（`enc:v1:` 前缀），加载时解密，兼容旧版明文配置
- [x] E-3 后端直裁：translate 模式提交选区坐标（`TranslateRegion`），后端从已捕获帧裁剪并复刻前端 96px 短边放大规则，省去前端 toBlob 与 base64 桥传输

## Codex 延迟与可靠性优化交接（2026-10-30）

### 结论与实现基线

Codex 订阅接入仍有优化空间，尚未证明已经达到性能极限，也尚未证明长期稳定。当前使用建议是优先选择实测较稳定的 API 接入，保留 Codex 作为可选通道。后续优化重点是减少客户端重复准备和恢复开销；服务端响应、首次工作区路由发现、代理网络的延迟无法由本地代码保证消除。

当前实现基线为 `7b98ce9`（`fix: stabilize Codex transport and timeout recovery`），位于 `main` 分支。本节记录后续计划，下面的待办尚未实施。

已完成的修复：

- [x] 使用 Codex 原生 `openai` provider，让 Codex 自行选择传输与回退，移除强制 HTTP-only 的自定义 provider。
- [x] 复用本地 App Server 进程，空闲 5 分钟后释放；每次任务仍使用独立临时会话。
- [x] 预热只初始化本地进程，避免在拖选结束时取消正在联网的预热、丢失准备进度。
- [x] 轻量或无思考请求的 20 秒无活动计时，在 `turn/start` 被接受后才开始；思考进度等模型活动会重置计时。已有文字输出后停止此计时，较高思考强度不使用该计时。
- [x] 无文字输出且模型持续无活动时，在原有总超时内最多重连一次；不重发已有部分输出，不重试主动取消的任务。
- [x] `account/read` 的工作区路由发现临时失败最多重试一次；保留流式错误、调用阶段和超时原因，增加回复失败日志。
- [x] 增加慢会话准备、持续思考、无输出恢复、部分输出、取消、路由失败的回归测试，以及可选的真实截图回复测试。

### 已观察到的现象与限制

以下均为 2026-10-30 单机观察，不能当作稳定性保证或同条件性能排名：

| 场景 | 观察 | 对后续工作的意义 |
| --- | --- | --- |
| Codex 工作区路由发现 | 16:59 的一次请求连续两次 `account/read` 各等待约 15 秒，连接准备累计约 30.7 秒，尚未进入模型生成 | 必须拆开账号路由、进程重启和推理耗时，不能只记录总超时 |
| HTTP 与 WebSocket 对照 | 同一张截图，强制 HTTP 有一次等满 60 秒无输出；WebSocket 两次约 12 秒、7 秒完成 | 保留原生传输选择；不要依据少量样本再次全局强制某一种传输 |
| 修复后的真实截图回复（后端调用） | 同一张截图、社交评论、空表达意图，连续 3 次约 14.1、5.5、7.3 秒完成 | 证明该样本当时可用，不能替代冷启动、闲置后和长时间使用测试 |
| API 翻译 | 两次模型首字等待约 2.5、1.3 秒，含 OCR 总耗时约 4.9、2.5 秒 | 可作为当时体验的参照；API 使用 DeepSeek V4 Flash，Codex 使用 GPT-6 Luna，输入也不同，不能把全部差距归因于接入方式 |

复现环境为 Windows x64，应用自动选中的桌面版 CLI 是 `0.166.0-alpha.5.1`。同机 PATH 中的 CLI 为 `0.120.0`，曾因不识别本机配置中的 `service_tier=default` 而无法完成请求；其他版本对照也未证明单纯换版本能解决超时。新电脑应先记录并验证实际运行版本，不照搬某个旧版本作为解决方案。

小色块测试和短时间连续成功只覆盖冒烟场景。后续仍出现路由超时，因此不要把本轮任务标记为“Codex 稳定性优化完成”。

### 代码入口

| 文件 | 重点函数 / 职责 |
| --- | --- |
| [`internal/codex/warm.go`](../internal/codex/warm.go) | `takeWarm`、`returnWarm`、`Warm`、`streamFast`、`streamFastAttempt`：进程复用、账号校验、预热交接、取消与重连 |
| [`internal/codex/client.go`](../internal/codex/client.go) | `start`、`account`、`requireSubscription`、`call`、`read`、`streamWithProgress`：原生 provider、同步 JSONL 读取、会话与流式事件 |
| [`internal/codex/command_windows.go`](../internal/codex/command_windows.go) | CLI 路径解析；留空时优先检测桌面版运行程序，再检测 PATH / npm CLI |
| [`internal/desktop/app_translation_warm.go`](../internal/desktop/app_translation_warm.go) | `prewarmTranslationConnection`：截图前预热入口 |
| [`internal/desktop/app_extensions.go`](../internal/desktop/app_extensions.go) | `StartTextAction`：回复准备、总超时、首字及错误日志 |
| [`internal/desktop/app.go`](../internal/desktop/app.go) | `processText`：文字翻译、总超时与工作流事件 |
| [`internal/translator/reply_test.go`](../internal/translator/reply_test.go) | `TestLocalCodexRepeatedImageReplies`：读取本机 Codex 设置，可选真实截图，连续 3 次请求 |
| [`internal/translator/latency_test.go`](../internal/translator/latency_test.go) | 本机 API 与订阅翻译的可选延迟测试 |

### 后续待办与建议顺序

#### F-1 先补齐分阶段测量与复现基线（P0）

- [ ] 记录并关联进程启动、初始化、账号 / 路由检查、会话创建、模型首字、完成、清理的耗时，以及本次重试次数、进程是否复用。保留原始错误类型和失败阶段。
- [ ] 记录运行程序版本、模型、思考强度、OCR / 图片模式、网络 / 代理是否启用。确认比较的是同一套设置和同一份脱敏输入。
- [ ] 记录每个请求实际发生的 `account/read` 次数与进程启动次数，验证下面两项优化是否真的减少重复工作。
- [ ] 明确区分模型首字等待与鼠标释放到首字显示，不能把 OCR、会话准备或缓存命中混在一个指标里。

日志维持本地、有限大小，不记录密钥、凭据、账号邮箱、截图内容、原文、译文或思考文本。测量模型活动时只记录事件类型和时间。

#### F-2 合并重复账号检查（P1，优先实施）

当前已复用连接的路径是：`takeWarm` 调用 `requireSubscription`，返回后 `streamFastAttempt` 再调用一次。`requireSubscription` 内部又可能重试路由发现。校验失败时，`takeWarm` 还可能关闭旧进程、启动新进程，随后再检查账号。

- [ ] 先消除同一次借用中的重复校验，保留一个明确负责订阅验证的位置；区分进程 / 管道失效与账号 / 路由失败，避免不必要地重新启动进程。
- [ ] 测量是否需要按进程保存短期的认证状态与就绪状态。若需要，定义有效期，以及登录、退出、认证错误、进程退出、设置变更后的失效规则。
- [ ] 新进程首次使用必须完成必要认证；API-key 登录不得被误判为 ChatGPT 订阅登录。
- [ ] 仅保存必要状态，继续由 Codex 管理凭据和续期，不读取、复制 `auth.json`，不另建令牌缓存。

验收：热连接的同一次请求不重复做相同认证；凭据失效、退出登录、切换 CLI 后仍能正确失败或重新认证。

#### F-3 中断当前任务后尽量保留健康进程（P1）

当前请求取消或无活动超时，会经 `context.AfterFunc(ctx, c.close)` 杀掉整个 App Server。重连因而重新承担进程启动、账号路由和会话准备开销。进程复用已有基础，但失败后的恢复仍有优化空间。

- [ ] 评估使用 `turn/interrupt` 中断当前 turn，收到终止事件并完成必要清理后，复用健康进程及连接。先确认当前 CLI 版本的协议与事件行为。
- [ ] 明确 thread / turn / 请求 ID 的归属，隔离取消后的迟到 token、完成事件和旧请求错误；恢复时不得重复显示文字。
- [ ] 清理必须有界；中断未确认、进程已退出、管道损坏或协议失步时仍应关闭进程，不能把未知状态的连接放回池中。
- [ ] 覆盖取消发生在初始化、账号检查、会话创建、模型思考、已有文字、清理阶段的情况；退出应用与变更设置后，旧连接不能重新入池。

实现约束：当前 `call` / `read` 同步读取同一个 Scanner，不能直接让取消处理与正常请求在两个 goroutine 中同时读取。若确需异步中断与响应分发，必须保证只有一个读取者，按 RPC ID 和 thread / turn ID 分发事件，再用测试证明无串流、无死锁。先做最小可验证改动。

#### F-4 把必要准备挪到后台，安全交接给前台（P2）

当前 `Warm` 只启动本地进程。此前联网预热常在前台接管时被取消；仅恢复旧的完整预热代码会重新引入这个问题。

- [ ] 在完成认证去重和生命周期梳理后，评估后台提前完成账号检查、模型 / 会话准备，明确哪些结果能供前台使用。
- [ ] 将进程生命周期与单次请求 context 分开；前台使用已有准备结果，不能因取消预热而丢掉健康进程或反复联网。
- [ ] 后台卡住时，前台不能等待到自身总超时；给交接设置明确边界，不并发读取同一条 JSONL 流。
- [ ] 预热不启动模型生成、不消耗无意义的推理调用，也不把前一次截图或指令带到下一次会话。

#### F-5 梳理总预算与失败提示（P2）

- [ ] 为账号 / 路由、会话准备、模型无活动、流式完成、清理分别记录阶段；所有阶段及重试仍受一次用户请求的总预算约束。
- [ ] 不再次把会话创建时间算入模型无活动时间；模型仍在思考时不能因“尚无文字”被提前重发。
- [ ] 未登录、配置不兼容、权限 / 额度错误直接提示，不盲目重连；部分输出和用户主动取消不重发。
- [ ] UI 明确区分“登录 / 工作区连接失败”和“模型响应超时”。切换 API 由用户决定，不静默回退到可能产生费用的接口，也不擅自改变 OCR / 直接看图选择。

### 验证标准

先用相同设置、相同脱敏输入采集优化前基线，再逐项修改并复测。API 可作为体验参照；不同模型的数据须分开解释。

| 场景 | 最少记录内容 |
| --- | --- |
| 首次启动、第一次截图 | 启动 / 账号 / 会话 / 首字分段耗时，失败阶段 |
| 热连接连续文字请求 | 至少 20 次，成功率、首字与完成耗时的 P50 / P95、认证与进程启动次数 |
| 热连接连续图片回复 | 至少 20 次真实脱敏截图请求；覆盖长文本与复杂截图，不能只测小色块 |
| 空闲超过 5 分钟后请求 | 确认池释放后的冷启动耗时与成功率，至少重复 3 次 |
| 主动取消、无活动超时后再请求 | 中断 / 清理是否有界，进程是否可复用，有无迟到事件与重复文字 |
| CLI / 模型设置变化、应用退出 | 旧进程释放，旧请求不能更新新结果或重新入池 |
| 新电脑与不同网络 / 代理 | 重新记录版本和环境，区分本机改动收益与网络变化 |

20 次样本用于定位问题，仍不是长期服务保证。完成 F-2 / F-3 的判据是重复工作与恢复开销可测地减少、相关回归通过，并报告剩余超时，而非承诺每次 2～3 秒出结果。

### 换电脑继续工作的步骤

1. 拉取 `origin/main`，先阅读本节和 `7b98ce9` 的改动；按 README、`go.mod` 和前端依赖要求安装工具链。
2. 在新电脑上重新完成 Codex 登录，并在应用设置中检测 CLI 路径和可用模型。桌面版和 PATH 中的 CLI 可能不是同一版本；以日志 `Codex runtime started` 的路径为准，仅运行 `codex --version` 可能查到另一份 CLI。不要依赖旧电脑的绝对路径。
3. 如需比较 API，在新电脑重新配置密钥。Windows 配置使用当前用户的 DPAPI 加密，旧电脑的配置密文不能假定可在新电脑解密。密钥、配置、凭据、OCR 二进制及构建产物不提交到仓库。
4. 先跑自动测试，采集当前版本的分阶段基线；建议从 F-1、F-2 开始，再做 F-3 和 F-4。一次只验证一个改动。
5. 实测后将样本数、环境、P50 / P95、失败阶段和结论补充到本节，再勾选相应待办。

自动回归与完整构建，在仓库根目录执行：

```powershell
go test ./... -count=1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 -OutputName snapTrans-check.exe
```

构建脚本可对 Windows 临时 `a.out.exe` / `wailsbindings.exe` 占用做有限重试，并写入构建标识。记录所运行程序的标识，避免测试到旧版本。

可选登录检查（不生成模型回复）：

```powershell
$env:SNAPTRANS_CODEX_STATUS_CHECK = '1'
try {
    go test ./internal/codex -run '^TestLocalCodexStatus$' -v -count=1
} finally {
    Remove-Item Env:SNAPTRANS_CODEX_STATUS_CHECK
}
```

可选真实图片回复回归：会上传指定图片并实际调用订阅模型、消耗额度；只选择经过检查的脱敏截图。下面的路径为示例，先替换为新电脑上的实际路径：

```powershell
$env:SNAPTRANS_CODEX_IMAGE_REPLY_CHECK = '1'
$env:SNAPTRANS_CODEX_REPLY_IMAGE = 'C:\test-data\sanitized-screenshot.png'
try {
    go test ./internal/translator -run '^TestLocalCodexRepeatedImageReplies$' -v -count=1
} finally {
    Remove-Item Env:SNAPTRANS_CODEX_IMAGE_REPLY_CHECK
    Remove-Item Env:SNAPTRANS_CODEX_REPLY_IMAGE
}
```

该图片测试读取应用中保存的 Codex 路径、模型、思考强度和总超时；即使日常选择 API，它仍明确测试 Codex。这里只测后端调用，不含完整快捷键、拖选、OCR 和前端绘制链路。当前一次测试只跑 3 次，未指定图片时使用小色块，只适合冒烟验证；连续 20 次与闲置后统计仍是待补的基准。切换测试 CLI 时先更新已保存设置，不要误以为所有可选测试都支持 `SNAPTRANS_CODEX_PATH`。

本机运行日志位于 `%APPDATA%\snapTrans\logs\snaptrans.log`；在分享日志前检查其中是否包含个人信息。协议参考：[Codex App Server 官方文档](https://learn.chatgpt.com/docs/app-server)。

### 2026-11-01 拉取、打包与性能复核

本轮基于 `a67db64`，保留本机原有 `.gitignore` 改动和未跟踪的 `android/`；未修改运行时代码。Windows x64 工具链为 Go 1.26.5、Node 22.20.0、Wails 2.12.0，实际自动检测的 Codex CLI 为 `0.166.0-alpha.5.1`。

使用 `scripts/package-release.ps1 -Version 0.2.1` 重新生成含 RapidOCR 的便携包，构建标识为 `20261101T122012Z-0c9fc7727d9f62d1`。构建产物和本机设置不提交。

#### 本轮实测（小样本，只作定位）

状态检测单次成功：初始化 268 ms，`account/read` 2415 ms，`model/list` 4538 ms，整体 7.43 s，返回已登录及 8 个模型。此项不生成模型回复。

随后用已有 `TestLocalCodexLatency`，开启 FAST、PREWARM，effort 设为 `none`，连续两次翻译测试内置的固定英文短句。模型为 GPT-6 Luna；不读取或上传用户截图，不含 OCR、快捷键和前端绘制，也不经过翻译结果缓存。

| 请求 | account/read 两次耗时 | config/read | thread/start | turn/start | 首字 | 完成 |
| --- | --- | --- | --- | --- | --- | --- |
| 预热后第一次使用 | 1154 / 4 ms | 7 ms | 338 ms | 24 ms | 7942 ms | 8280 ms |
| 同一进程第二次使用 | 6 / 5 ms | 9 ms | 199 ms | 42 ms | 4433 ms | 4774 ms |

进程预热初始化另用 308 ms，不计入上述请求首字。按日志时间，`turn/start` 返回到首字约为 6.41 s / 4.17 s；这一段仍包含传输、服务端和模型处理，不能进一步归因于纯推理时间。重复账号检查确实存在，但本轮第二次检查只有 4–5 ms；去重不能据此宣称节省数秒。两次样本不足以计算有代表性的 P95，亦不能证明持续稳定。

#### 建议顺序与额外发现

1. **F-1 + F-2 优先：统一阶段指标并消除重复认证。** `warm.go` 的 `takeWarm` 和 `streamFastAttempt` 都调用 `requireSubscription`，假进程复用回归和真实请求日志均复现两次 `account/read`。将同一次借用的校验统一到一个位置，并区分路由错误与进程失效；保留未登录/API-key 登录拒绝、取消和配置变更回归。
2. **补全默认模型路径。** 模型留空时，`client.go:startThread` 每次调用 `model/list`；`fast.go:forTranslation` 又仅在配置模型精确等于两个 Luna ID 时设置 `none`。因而选择“账号默认模型”即使实际解析为 Luna，也不会走同样的快速思考设置。建议按进程缓存已解析的默认模型和模型能力，解析后再选择该模型支持的 effort，设置/登录变化时失效。当前 `Model` 只保留 ID、名称和默认标志，尚未使用协议返回的 `supportedReasoningEfforts` / `inputModalities`。
3. **F-3：取消时优先评估协议中断。** 当前 `context.AfterFunc(ctx, c.close)` 会结束整个 App Server，下一次重走启动、认证和会话创建。官方协议支持 `turn/interrupt`，且必须等待 `turn/completed` 的 `interrupted` 状态；收到确认并清理后才可复用。当前同步 Scanner 必须保持单读取者，不能直接新增一个同时读取的取消 goroutine；清理失败仍关闭进程。
4. **F-4：后台准备先解决交接。** 当前 `Warm` 仅初始化本地进程，前台最多等待预热 150 ms；恢复旧的联网预热会有被前台取消、丢失准备进度的风险。完成阶段测量、去重和生命周期处理后，再将认证及必要模型发现移到后台。每次仍创建独立 ephemeral thread，避免把旧截图上下文带入新请求；不建议直接复用聊天会话。
5. **F-5：为清理单独设置有界预算。** `client.go:streamWithProgress` 收到成功完成后，同步等待 `thread/unsubscribe` 才返回；若清理卡住，文本已显示，外层 done/历史保存等仍可能等待至请求总超时。本轮该调用均为 0 ms，因此这是恢复边界的风险项，尚非本轮已测得的瓶颈。
6. **后续再优化图片和绘制。** `EncodeVisionCrop` 已限制最长边 1600、JPEG 质量 90；翻译及回复面板均已有 requestAnimationFrame token 合并。应先记录图片字节数、文字可读性与首字/绘制时间，再比较 OCR 文本和图片直传；不应盲降分辨率或静默切换用户输入模式。回复的 `MaxTokens: 500` 只传给兼容 API，Codex 分支未传递该字段，不能把它视为订阅请求的硬输出上限。

官方协议核对：[App Server](https://learn.chatgpt.com/docs/app-server) 的模型发现、账号状态和 turn 中断说明。保留原生 provider 与传输回退；本轮数据不支持强制切换 HTTP/WebSocket 或更换 CLI 来保证提速。

验证：前端 18 个文件、190 个测试通过，typecheck 和生产构建通过；`go test -p 2 ./... -count=1` 与 `go vet -p 2 ./...` 通过；build、build-identity、ensure-rapidocr 三组 PowerShell 脚本测试通过。默认并发的首轮 Vitest 出现 worker 启动超时，Go 标准库 unicode 的 vet 进程退出异常；限制并发后复跑通过，未为此修改业务代码。完整桌面交互、混合 DPI、多屏、20 次真实图片及闲置后基准仍待验证，上述待办不勾选。

### 2026-11-04 Codex 请求与绘制优化

本轮保持原生 provider、订阅凭据管理及独立临时会话，未修改本机 Codex 配置。实际 CLI 为 `0.166.0-alpha.5.1`。

- 成功的订阅检查按进程复用 60 秒，失败不缓存；首字前发现空闲子进程退出时最多恢复一次。部分输出、主动取消、总超时不会重发。
- `turn/start` 确认前的文字立即交付；成功后的 `thread/unsubscribe` 等待上限为 250 ms，清理失败丢弃进程，保留完成结果。
- 输入的 `userMessage` 回显不再重置模型空闲计时；真实思考进度仍可重置。前台借用期间不再启动竞争的预热进程。
- 子进程禁用编码 hooks、plugins、预取、skill_search 与 apps。当前用户存在 10 秒同步 SessionStart 钩子，但无推理 A/B 未复现显著差异，不能将全部等待归因于钩子。
- 当前模型列表对两个 Luna ID 返回的 effort 为 `low/medium/high/xhigh/max`。极速翻译改用 `low`，同时用于创建线程与开始回合，避免线程先继承全局 `ultra`。回复仍保留用户设置。
- 用有效的 `features.view_image=false` 替换被 CLI 忽略的旧键；直接图片输入不受影响。
- 前端按冻结的 OCR 区域复用未变化译文的排版。40 区域回归中，只追加末行文字时测量调用减少超过 90%，新截图或尺寸变化会失效缓存。

相同合成短句、GPT-6 Luna、`low`、预热、连续同一进程，未经过译文缓存，不上传用户截图：

| 请求 | 账号检查 | thread/start | 首字 | 完成 |
| --- | --- | --- | --- | --- |
| 1 | 2189 ms | 865 ms | 10276 ms | 12277 ms |
| 2 | 复用 | 2131 ms | 9512 ms | 9838 ms |
| 3 | 复用 | 597 ms | 6030 ms | 6495 ms |

3 次均成功，但仍不足以证明长期稳定或计算有代表性的 P95。此前 `none` 诊断请求出现过 84 秒无输出失败；与本轮 effort 不同，不计算提速比例。网络 Doctor 的 HTTP 与 WebSocket 握手均成功，约 1.18 / 2.10 秒，不能据此排除生成阶段波动。完整拖选、真实图片、20 次采样及长时间闲置回归仍待补充。

验证：`go test -p 1 ./... -count=1 -timeout 120s` 全部通过；最终工具隔离改动的定向回归通过。前端 19 个文件、244 项测试通过（两项 worker 启动超时的文件在单 worker 重跑通过），设置页 8 项测试另行通过，typecheck 与生产构建通过。延迟测试现支持 `SNAPTRANS_CODEX_LATENCY_RUNS=1..20`。

### 2026-11-04 参考公开案例：提前准备未使用会话

公开证据区分如下：

- 已合并的 [OpenAI PR #10698](https://github.com/openai/codex/pull/10698) 在会话启动时预建连接，首次回合采用准备结果；[PR #22198](https://github.com/openai/codex/pull/22198) 明确区分线程建立、上下文 / 提示词准备、WebSocket 预热与首字。因此只启动进程不足以完成传输准备，`thread/start` 返回也不代表连接已完全就绪。
- [Issue #32526](https://github.com/openai/codex/issues/32526) 报告模型目录冷刷新导致首轮 15–33 秒静默；[issue #40137](https://github.com/openai/codex/issues/40137) 报告过期目录与插件元数据阻塞。这些是报告者的环境与测量，不能直接当成本机收益。目录后台刷新已有合并的 [PR #28699](https://github.com/openai/codex/pull/28699)，本机热目录探针为 0 ms，因此没有再添加周期性 `model/list`。

本轮文字翻译改为预先准备一个未使用的 ephemeral thread，精确匹配模型、完整指令与 effort 后只使用一次。请求成功后后台补充下一次会话，最多 15 秒；已有备用不会被猜测的默认方向覆盖。准备不包含截图或用户回合，不共用已使用会话的历史。初次直接图片模式仍只预热进程。前台交接最多等待 150 ms；失败、取消、设置失效后均保留正常冷启动与原有有界恢复。

同时缓存同一进程成功解析的默认模型，并在 `thread/start` 和 `turn/start` 明确传入同一模型。补充任务的 generation 在准入与领取时均验证，防止旧配置任务重新回池或抢走新连接。

同一 CLI、GPT-6 Luna、low、同一合成英文短句，两组每次都等待 3 秒模拟截图 / OCR。前台计时不含单独记录的后台准备，不经过翻译缓存，不包含真实 OCR 或界面绘制：

| 路径 | 首次后台准备 | 3 次首字 | 3 次完成 |
| --- | --- | --- | --- |
| 只预热进程 | 1.034 s | 10.573 / 5.994 / 6.114 s | 10.823 / 6.223 / 6.368 s |
| 准备未使用会话 | 5.316 s | 4.319 / 2.696 / 3.217 s | 4.352 / 3.101 / 3.436 s |

随后单独连续 20 次会话预热请求，全部成功：首字中位数 **2.963 s**，P95（nearest rank）**4.562 s**，范围 1.920–4.583 s；完成中位数 3.388 s，P95 5.029 s。首次后台准备 2.650 s 单独计算。原始样本与限制保存在 [benchmark JSON](benchmarks/codex-prepared-2026-11-04.json)。分组顺序、短句和单机网络限制仍存在，不能据此保证长文本、图片、长期空闲或所有网络的性能。

复测入口为 `TestLocalCodexPreparedSessionLatency`：设置 `SNAPTRANS_CODEX_SESSION_LATENCY_CHECK=1`；可选 `SNAPTRANS_CODEX_SESSION_LATENCY_PROFILE=prepared` 或 `process`，以及 `SNAPTRANS_CODEX_SESSION_LATENCY_RUNS=1..20`。默认先后比较两种路径，各 3 次；会实际使用订阅模型，固定合成输入不读取用户截图。

最终验证：`go test -p 2 ./... -count=1 -timeout 120s` 全部通过；一次性采用、键不匹配清理、默认模型缓存、补充任务失效、旧 generation 领取保护及保留上次备用会话均有回归。Windows 构建 `snapTrans-codex-prewarm.exe` 已生成，标识 `20261104T043234Z-f067ddca60108c67` 与当前源码摘要一致，已核对内嵌前端资源。未改动本机配置、凭据或旧版程序。

### 2026-11-04 OCR 裁剪与 CPU 推理优化

试用日志显示，常驻模式已开启且同一 OCR 进程持续存在，但三个选区识别仍耗时 4088 / 13352 / 8825 ms；另有一次裁剪耗时 2091 ms。没有据此认定模型重复加载。

1. 原尺寸裁剪改为直接复制像素；需要放大时用具体 RGBA 类型避免逐像素装箱。580×388 与 599×500 合成选区裁剪由约 158–234 ms 降至约 1 ms，分配由约 135–180 万次降至约 2 次。2×、4×、5× 插值、透明度、负坐标、子图步幅及 DPI 相关回归保持像素一致。
2. 发布版 RapidOCR-json v0.2.0 仅设置 inter-op，intra-op 设置被注释，因此单改 `numThread` 不足以控制实际运算线程。[发布源码](https://github.com/hiroi-sora/RapidOCR-json/blob/v0.2.0/cpp/src/CrnnNet.cpp) 与 [ORT 配置解析器](https://github.com/microsoft/onnxruntime/blob/v1.12.1/onnxruntime/core/session/inference_session_utils.cc) 支持通过模型 `ort_config` 元数据配置线程及图优化。
3. 仅对二进制、三个默认模型及字典的 SHA-256 全部匹配的自带版本启用配置：intra-op 2、inter-op 1、顺序执行、图优化 99、关闭 profiling。原模型完整字节保留，仅在用户缓存副本尾部追加元数据；不量化、不降低图片尺寸、不关闭方向分类。[CPU 图优化说明](https://onnxruntime.ai/docs/performance/model-optimizations/graph-optimizations.html)。其他安装、显式模型参数或缓存异常保持原命令。
4. 缓存验证完整哈希并原子发布，复制过程再次校验源内容；环境变量仅设置在 OCR 子进程。使用 Unicode 工作目录与 ASCII 相对模型参数，真实中文加空格路径已通过识别验证。日志新增队列、启动、准备、推理、解析及回退阶段，不记录图片或识别原文。

本机 i7-9750H 的合成图片对照，保留同一图像和模型权重：

| 场景 | 原引擎 | 优化后原始进程 | 正式 Worker 两次 |
| --- | --- | --- | --- |
| 浅色 10 行中英文，600×500 | 3502 / 3701 ms | 2562 / 2180 ms | 2416 / 2605 ms |
| 深色 10 行中英文，600×500 | 4696 / 4121 ms | 2482 / 2288 ms | 2197 / 3239 ms |
| 8pt 小字，600×220 | 2468 ms | 1203 ms | 1116 / 1229 ms |
| 长行，1400×180 | 2007 ms | 1439 ms | 1503 / 1438 ms |
| 小选区，505×89 | 1301 ms | 1040 ms | 656 / 617 ms |
| 倒置 180°，600×500 | 3281 ms | 2104 ms | 2156 / 2105 ms |

六类样本的文字、顺序及全部框坐标与原引擎一致，最大置信度差约 `3.61e-7`；正式 Worker 12 次完整结果均与原引擎参考相同。原始测试进程线程数从 19 降为 7。首次模型缓存创建约 1108 ms、包含缓存创建的后台初始化约 3151 ms，单独计时，不计入上表识别耗时。数据见 [OCR benchmark JSON](benchmarks/ocr-2026-11-04.json)。这是合成样本与单机测量，不是对试用日志原截图的直接复测，系统负载与选区内容仍影响耗时。

可选本机回归：`SNAPTRANS_OCR_LATENCY_CHECK=1`、`SNAPTRANS_OCR_EXE` 指向自带引擎、`SNAPTRANS_OCR_FIXTURES` 指向 `build/benchmarks/ocr-20261104`，运行 `go test ./internal/ocr -run '^TestLocalOCR(RuntimeLatency|UnicodeRuntimePath)$' -v -count=1`。只执行本地合成截图识别，不调用翻译服务；生成模型与图片位于已忽略目录。

最终验证：`go test -p 2 ./... -count=1 -timeout 120s` 全部通过；真实 Worker 12 次质量对照及中文空格路径识别通过。构建 `snapTrans-ocr-optimized.exe` 标识 `20261104T053036Z-9466e066c85f98c4`，已核对源码摘要与内嵌前端资源。原始引擎、原模型、本机配置及正在运行的旧版程序均未替换。

### 2026-11-04 局部译文完整性与补译等待

新版真实试用两次 OCR 为 3828 / 3582 ms，队列和初始化均为 0，裁图为 15 / 10 ms。第二次总耗时 18172 ms：首轮结束后还有一次补译，并与后台备用会话准备冲突，额外支付了进程启动、认证和线程建立。日志没有保存译文，不能据此断言该次补译一定属于误判。

- 覆盖检查原先拒绝合法的 `[1]你好`，且 `\s+` 可跨行吞掉下一个编号。改为按物理行识别可选空格，空编号仍需补译；回归直接验证完整输出只调用一次模型。
- 真正补译携带 JSON 引用的全文作为只读上下文，仅缺失编号作为翻译目标，沿用首轮系统指令、术语表和预热键。整次逻辑翻译结束后才补充下一次会话；首轮与补译复用认证进程，每轮仍使用新的独立临时会话。取消、失败、旧配置失效不复活过期预热。
- OCR、选中文字和快速模式均明确按上下文理解视觉换行，保留完整信息及混排姓名，不概括原文。OCR 与快速请求允许在上下文确定时修正明显字符混淆；修正本地短词典“关于”译成英文时仍返回中文的问题。
- 原图问候语和 NEXT 的 OCR 均完整，省略号来自短标签继承正文典型字号、又过早停止缩字。标签改为按实际测量适配，保持原框；原 43×23 / 33×16 框在 Chromium 真实字体下分别以 10px 完整显示问候语和“下一步”。正文继续保持原可读性策略。
- 补译可能乱序到达或替换旧编号；区域结果的全文查看和复制现在按原编号排序并采用最新值，避免重复旧稿。普通无区域文本和直接图片译文保留原段落。

参考 [OpenAI 延迟指南](https://developers.openai.com/api/docs/guides/latency-optimization) 的减少串行请求原则，优先去掉错误补译和进程重建。公开合成正文各一次真实调用：旧提示词首字/完成 4359/6254 ms，新提示词 5855/8057 ms，两次均完整 8 个编号；新提示词纠正 APl→API，保留姓名和模型标识。这组新样本更慢，不能声称生成速度提升。具体记录见 [翻译质量 benchmark](benchmarks/translation-quality-2026-11-04.json)。

本地进一步 profiling 显示识别模型是剩余 OCR 主要成本；分别采样的检测/方向/识别模型运行约 615/347/2884 ms，带 profiling 开销且并非同一请求，不能相加作为端到端指标。继续保留原模型、图像清晰度及方向识别。本轮收益集中于完整显示、上下文质量、避免多余请求及补译稳定性。

最终验证：前端 19 个文件、247 项测试通过，typecheck 与生产构建通过；`go test -p 2 ./... -count=1 -timeout 120s` 全部通过。Chromium 真实字体复核标签、原图正文四行与底部两行分组：正文和底部均以 15px 完整显示，无截断。Windows 构建 `snapTrans-refined.exe` 标识 `20261104T060533Z-53df3ac8f2b57f37`，源码摘要与内嵌最新前端 `index-B5TEGIJy.js` 已验证。未启动新版去抢占正在运行的应用；使用时从托盘退出旧版后启动新版。

### 2026-11-04 全局稳定性复核

继续检查整条截图、识别、翻译、取消与设置链路，新增缺陷以离线回归先复现再修复；本轮没有继续消耗真实模型请求，也没有更换模型、降低图片清晰度或关闭方向识别。

- OCR 不再把正文中的 `init completed` 当作启动日志丢弃，避免合法 JSON 被丢掉后一直等待超时；按官方协议区分成功、无文字和引擎错误。错误不再混入原文，诊断内容不进入状态错误文本。[返回值说明](https://github.com/hiroi-sora/RapidOCR-json#返回值说明)
- 超时保留 `context.DeadlineExceeded`，取消在准备和结果边界优先处理，避免截止后追加昂贵回退。识别块改为先按 Y 定序、固定行锚点后按 X 排序，修复两两容差比较不满足传递性、顺序随检测器遍历改变的问题。
- 真实复现非 ASCII 临时目录中的单次 OCR 返回状态 300。仅对哈希匹配的自带引擎复用单次管道 Worker，直接传 base64，不写临时截图；自定义 CLI 模式保留。中文、空格、emoji 临时路径现在成功且无截图文件残留。这与上游确认的[命令行非 ASCII 路径限制](https://github.com/hiroi-sora/RapidOCR-json/issues/14)一致。
- 应用的后台请求统一归属可取消的生命周期；退出后拒绝新登记，取消尚未调度的预热、识别、翻译和设置检测。选区取消、翻译/OCR配置变更会取消旧任务；同一操作 ID 的旧完成回调不再清掉新任务的取消句柄。历史记录偏好等无关设置不打断正在翻译的内容。
- HTTP 收到正常 stop 后仅为连接复用等待最多 250ms，防止网关不发终止标记而空等整次超时；本地悬空尾部用例由 2s 截止失败变为约 0.27s 完成。保留正常尾部复用、父上下文取消及实际 API 错误。空结果不成功、不缓存，429/503 错误文字中包含 EOF 也不重试，已经输出空白 token 后不重复发送。
- Codex 的重连与上游错误由自身客户端负责，外层 HTTP 的 EOF 重试规则不再重新提交 Codex 终止错误。复现的是 `willRetry=false` 的上游错误文本含 unexpected EOF，不能把普通进程 EOF 一概解释为四次请求。
- 前后端对齐编号、CR/CRLF换行、引号及标记清理规则；合法 Begin/End 标签保留，空编号、半截编号和超范围 ID 不再导致错位或进入复制内容。图片方向回调与缓存命中过程中的取消也能及时停止输出。
- 前端在 done/error/partial 后忽略同代迟到的 token、开始、完成和元数据，避免二次自动复制、译文复活或布局被污染。保留原文中文姓名的英文短句可以正常展示；原生选中的短文本按实际可读字号适配，紧凑框的垂直裁切能正确处理。DPI映射只修复已复现的右下边界越界，没有改变现有单屏捕获和多屏坐标协议。

真实 OCR 重新验证六类样本各两次，全部文字及框坐标对照通过；中文模型缓存路径和单次中文临时路径也通过。采样存在后台测试负载，不用于声称推理速度提升。原始阶段数据与方法见 [全局复核记录](benchmarks/global-review-2026-11-04.json)。

最终验证：19 个前端测试文件、265 项测试全部通过；typecheck、前端生产构建通过。最终 Codex 重试守卫更新后再次运行 `go test -p 2 ./... -count=1 -timeout 120s` 全部通过；OCR 依赖设置、Windows 构建重试与构建标识脚本测试通过。Windows 新版 `snapTrans-stable.exe` 标识 `20261104T064246Z-fab332d9ea861e60`，已核对源码摘要和内嵌前端 `index-BXXUo7Uv.js`。未启动新版占用用户当前热键或退出正在运行的旧程序；从托盘退出旧版后可运行新版。

### 2026-11-04 连续操作与设置一致性复核

本轮继续以可复现问题为准，没有因为旧版日志重新修改 OCR 性能参数、提示词或模型。主要修复如下：

- 局部 OCR 框越过左/上边界时，原归一化只截断起点而保留过大宽高；现在先做完整矩形交集。完全在选区外的有坐标文字不再回退成无坐标原文参与翻译。
- 滚动截图的 Cancel 原来为空操作；现在通过非阻塞取消标记及父上下文停止后续采集、拼接和编码结果发布。正在完成的长图仍保留任务归属，取消或新选区会使其失效；旧预览和旧长图不能进入新截图缓存。
- 前端滚动操作使用本地序号丢弃旧 begin/step/解码/finish 回调，取消后的成功结果不复制旧图，失败结果不重新显示旧窗口。服务边界只将 Begin/Cancel 按发起顺序串行，防止旧取消清理新会话；Step/Finish 继续独立执行。
- 初次配置读取未完成时禁用表单保存，避免默认值覆盖已保存设置；延迟保存、自启读取、复制原文反馈只作用于所属设置或结果。保存返回不再隐藏热键刚打开的新选区。
- 配置发布与旧任务失效合为同一临界区；公开加载、保存、自启更新/回滚、退出及历史写入使用统一事务边界。缺失的快捷键注册句柄可以在保存同一快捷键时恢复，失败正确回滚。配置根对象为 null 时使用有效备份，不默默变成默认配置。
- Codex 仅暂存实际消费的异步通知，限制为 256 条/4 MiB，出队后释放原槽引用；取消先于缓冲内容投递，无关通知的不同 params 形状不再中断翻译，已经拒绝的服务器请求不作为译文处理。模型分页检测完整 cursor 循环，避免 A→B→A 无限循环。
- 按[官方 App Server 协议](https://developers.openai.com/codex/app-server)处理账户更新通知：本地 revision 使订阅检查、默认模型及未用会话失效，RPC 返回不能覆盖更新后的失效状态。完成通知早于 turn/start 确认时，仍即时显示 token，但只有成功确认才能完成并缓存；缺失确认最多等待 250ms，保留部分输出并返回错误，不重放。

验证：19 个前端测试文件、278 项测试全部通过；typecheck、生产构建通过；`go test -p 2 ./... -count=1 -timeout 120s` 全部通过。六类真实 OCR 样本各两次、中文模型缓存路径、中文/空格/emoji 临时路径均通过，文字及坐标保持参考结果；后台测试负载使延迟波动较大，不用于声称模型推理提速。数据与方法见 [本轮复核记录](benchmarks/global-review-v2-2026-11-04.json)。

Windows 构建 `snapTrans-stable-v2.exe` 标识 `20261104T073403Z-9b164867771d6061`，源码摘要及内嵌前端 `index-BsOFXt5-.js` 已核对。未修改用户本机配置、自启或正在运行的程序；从托盘退出旧版后启动此文件。
