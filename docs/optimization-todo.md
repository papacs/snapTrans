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
