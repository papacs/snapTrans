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
