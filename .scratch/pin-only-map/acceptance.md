# 验收执行清单：spec §8 十三条

> 依据 `.scratch/pin-only-map/spec.md` §8。2026-09-08 生成。
> 状态含义：**自动** = 已有测试覆盖，跑 `go test ./... && cd frontend && npm test` 即绿；**肉眼** = 只能在真实 GUI 里确认。
> 全量自动化基线（2026-09-08）：Go 9 个测试函数通过；前端 7 文件 132 项通过；`tsc --noEmit` 无错；`wails3 build` 通过（`bin/PinNote`）。

| # | 验收标准 | 状态 | 覆盖点 / 观察点 |
|---|---|---|---|
| 1 | `⌘⌥N` 全局呼新窗、placeholder「记点什么…」、光标就位 | 肉眼（链路自动） | 快捷键注册在 Go 侧无测试；`PinEditor` 的 placeholder 装饰需肉眼。观察：切到别的应用按 `⌘⌥N` |
| 2 | `**粗体**`/`# `/`- [ ] ` 原地渲染，勾选回写 `- [x]` | 自动 + 半肉眼 | input rules 是 StarterKit 自带（spec §2.5 明确走人工）；勾选回写见 `roundtrip.test.ts` 的 checkbox write-back |
| 3 | 400ms debounce 落库、失焦立即落库 | 自动 | `PinEditor.test.tsx` 四条保存管线 |
| 4 | 含 `![img](url)` 的块降级 rawSource、原文不丢 | 自动 | `roundtrip.test.ts` LOSSY 语料（4 条）；肉眼只需确认渲染形态是「源码原样」 |
| 5 | HTML 注释块降级 +「转为可编辑」重过审计不丢 | 自动 + 半肉眼 | 降级链路自动；「转为可编辑」按钮的实际点击需肉眼 |
| 6 | 空笔记关窗硬删、有内容关窗保留 | 自动 | `TestDiscardIfEmpty` + `PinWindow.test.tsx` flush 顺序（含 flush 失败不判空） |
| 7 | 重启恢复全部 live 窗口、主题一致 | 自动（逻辑）+ 肉眼（外观） | `TestRestoreAllNoteWindows`、`TestThemePersistence`；重启后的窗口位置/尺寸是否像上次需肉眼 |
| 8 | `⌘⌫` 删除进回收站、他窗无异常、重启不再出现、60 天 purge | 自动 | `TestTrashRestoreFlow`、`TestPurgeExpiredTrash`、`usePinShortcuts` ⌘⌫、`PinWindow` 删除流程 |
| 9 | 边缘拖拽可 resize、高度不低于 150、正文内部滚动 | **纯肉眼（未验证项）** | spec §1.2 要求实现第一步实测；`MinHeight: 150` 已在 Go 侧设置，但原生边缘热区是否被 webview 吞事件无自动化手段 |
| 10 | `⌘⇧D` 所有窗口与原生背景同帧切换、重启保持 | 自动（状态）+ 肉眼（同帧） | `TestThemePersistence`、`usePinShortcuts` ⌘⇧D；「同帧无闪烁」只能肉眼 |
| 11 | header 悬停拖拽区 → tooltip 显示剥记号后的首行标题 | 自动（派生）+ 肉眼（tooltip） | `TestDeriveTitleEnhanced`；tooltip 出现需肉眼 |
| 12 | `⌘⇧/` 帮助内容与 §4.2 一致、Esc 关闭 | 自动 + 半肉眼 | `usePinShortcuts` 键位测试；模态框正文与 §4.2 表逐条比对需肉眼 |
| 13 | 无窗口时 Dock 可退出；`⌘Q` 在途输入不丢 | 自动（兜底）+ 肉眼（Dock/退出） | `beforeunload` flush 自动；菜单栏/托盘退出路径需肉眼 |

## 一次性肉眼脚本（约 8 分钟）

1. `wails3 dev`（或直接跑 `bin/PinNote`）→ 确认启动后每个 live 笔记各有一个窗口，主题与上次一致（#7）。
2. 拖窗口右缘 → 能否改宽；拖下缘 → 能否改高且停在 150 不继续缩；正文塞长文本 → 窗内滚动、无滚动条挤压正文（**#9，唯一完全没有自动化兜底的项**）。
3. 按 `⌘⌥N` 切到别的应用再按 → 新窗出现、placeholder 可见、光标就位（#1）；输入 `# `、`- [ ] `、`**x**` 看原地渲染，勾一下复选框后关窗重开，源码应是 `- [x]`（#2）。
4. 粘一段含 `![a](b)` 与 `<!-- c -->` 的 Markdown → 两块应为源码形态、字符不差；点「转为可编辑」再关窗重开（#4 #5）。
5. 悬停 header 拖拽区看 tooltip 标题（#11）；按 `⌘⇧/` 逐条对 spec §4.2 表，Esc 关（#12）。
6. 多开两窗，`⌘⇧D` 看是否同帧切换（#10）；`⌘⌫` 删一个，确认他窗正常（#8）。
7. 什么都不输直接关窗（#6），然后 `⌘Q` 退出，重开确认 #7 与「在途输入不丢」（#13）。

## 已知遗留（不在本清单内）

- **行内图片存储方案**：spec §6 要求另提 ticket，`.scratch/` 下尚未创建。
- **托盘增量的规格记录**：`96c80c7`（菜单栏托盘）在 map/spec 里无对应工单，术语已进 `CONTEXT.md`，规格只在提交说明和代码注释里。
- **敏感笔记常驻**：现库中存在含密码的笔记；pin-only 形态下它会以置顶无边框窗口常驻桌面且无锁屏保护，属产品形态的固有暴露面，值得单独决策。
