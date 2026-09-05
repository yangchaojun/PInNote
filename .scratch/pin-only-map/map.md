# Map: Pin 窗口即全部界面

Label: wayfinder:map

## Destination

将 PinNote 改造为"pin 窗口即全部界面"的产品形态，并交付可直接开工的 spec：

1. 移除主面板（主列表窗口及其列表/编辑/回收站/搜索 UI）；应用启动即恢复所有 pinned 笔记的 pin 窗口，无 pinned 笔记时静默驻留（Dock/菜单栏保留）。
2. pin 窗口：默认高度 300px、最小 150px、可手动调整、超出内部滚动；整窗可编辑（WYSIWYG，contenteditable），移除编辑/预览切换模式。
3. 新建入口改为：全局 `⌘⌥N` 直接创建并打开新笔记 pin 窗口 + 菜单栏"新建笔记"项。
4. 删除（进回收站）与主题切换收进 pin 窗口工具条；搜索与回收站 UI 整体出界（60 天 purge 后台逻辑保留）。

## Notes

- 技术栈：Wails v3 (Go) + React 19 + TS + Vite + Ant Design + TanStack Query + Zustand + SQLite。
- 每个会话应参考 `/grilling`、`/domain-modeling`；编辑器验证用 `/prototype`。
- 计划为主（plan, don't do）：tickets 产出决策与可执行 spec，执行交回给后续会话。
- 已确认的关键决策（两轮 grilling，2026-08-31）：
  - Q1 新建 = `⌘⌥N` 直接开 pin + 菜单栏项；Q2 启动恢复 pins，无 pin 静默驻留；
  - Q3(c) 整窗 contenteditable WYSIWYG（Apple Notes 式）；
  - Q4(a) 固定默认 300 / 最小 150 / 可手动 resize / 内部滚动，取消自适应增高；
  - Q5(b)（修订 by 03）删除+主题**不进** header（原"收进 pin 工具条"改为仅快捷键+菜单栏入口）；搜索/回收站 UI 出界不变；
  - Q6 驻留行为不变，Dock 图标保留；
  - Q7 往返子集 = MarkdownView 现有 GFM 子集，解析失败原样保留；
  - Q8 TipTap + 自定义 Markdown 序列化（选型已定，实现细节见 ticket 02）；
  - Q9 保留 `⌘B/⌘I/⌘K` 快捷键，不加悬浮格式条；
  - Q10 放弃"恢复/彻底删除"UI；Q11 空笔记即正常编辑态，删除回收站占位视图。

## Decisions so far


- [TipTap × Markdown 往返能力调研](issues/01-research-tiptap-markdown-roundtrip.md) — **GO**：TipTap v3 + 官方 `@tiptap/markdown`（社区包已弃用）；往返覆盖全部所需 GFM 子集；不丢数据用 `rawSource` atom 块节点 + 保存时 `parse(serialize(doc)) ≡ doc` 守卫；序列化是规范化重写（Underline 需关闭）；复选框/粘贴路径均有官方扩展点。完整报告在 `research/tiptap-markdown-roundtrip` 分支 `docs/research/tiptap-markdown-roundtrip.md`。

- [原型：pin 窗口整窗 WYSIWYG 编辑器](issues/02-prototype-pin-wysiwyg-editor.md) — **GO**：模型成立。原型在 `prototype/tiptap-wysiwyg-editor` 分支 `prototype/wysiwyg-editor/`（109 项回归全绿 + 浏览器实测）。对 01 的 6 条实测修正：审计需静态类型门+行为不动点双保险；装载按块 trim；守卫比较归一化顶层空段落；段落序列化转义行首块语法（SafeParagraph）；rawSource 转换必须重过审计；行内图片静默丢失→类型门排除（产品决策归 03/06）。audit/rawSource/SafeParagraph/语料网可直接晋升。

- [Pin 窗口交互与窗口规格](issues/03-grilling-pin-window-interaction-spec.md) — **定稿**（2026-09-05）。窗口：380 宽 / 高默认 300 最小 150 无上限，自适应增高移除；frameless resize 走 macOS 原生 frame（`Titled|Resizable` 保留，Wails JS 热区仅 Windows/Linux 不可用），实现首步实测、备 CSS 手柄 + `SetSize` 回退，补 `MinHeight: 150`；正文内部滚动 + 细覆盖滚动条。标题：首行即标题（增强现有 `deriveTitle`：剥复选框前缀与行内记号，其余沿用），header 平时不显示、悬停 tooltip。Header 只留 [图钉图标 | 拖拽区 | 关闭]。快捷键：`⌘⌥N`、`⌘B/I/K`、`⌘⇧D`、`⌘⌫`（删除唯一入口，连带关窗）、`⌘⇧/` 帮助；`⌘W`/块格式键/主面板遗留键全部不保留。placeholder「记点什么…」。行内图片本期不支持（rawSource 降级保数据，存储方案另提 ticket）。**修订 Q5(b)**：删除/主题不进 header，仅快捷键 + 菜单栏。

- [笔记生命周期](issues/04-grilling-note-lifecycle-spec.md) — **定稿**（2026-09-05，grilling 未应答、按推荐项落定，可推翻）。关窗 = Stickies 式收起：**启动恢复所有 live 笔记窗口**（不再按 pinned 过滤），固定/取消固定概念退役（`Pinned` 字段与 `SetPinned` 后端冻结）；空笔记（content 空白）关窗即硬删；自动保存 400ms debounce + 失焦/动作/beforeunload 三层 flush；主题迁移 Go 侧持久化（`GetTheme`/`SetTheme` 单一事实源，localStorage 方案废弃）；存量未固定笔记被"恢复全部"消解，无需迁移；退出驻留行为不变。

- [主面板移除影响清单](issues/05-grilling-main-panel-removal-impact.md) — **定稿**（2026-09-05）。逐文件改动地图：前端删 App/NoteList/TrashView/Editor/MarkdownView/useMainShortcuts（换 usePinShortcuts），重写 PinWindow（WYSIWYG + 新 header + flush）与 theme.ts（Go 单一事实源）；Go 删主窗口/mainBackground/FocusMainWindow，⌘⌥N 改道 CreateNote→OpenPinnedWindow（`app:new-note` 消亡），restoreAllNoteWindows 恢复全部 live，新增 DiscardIfEmpty/settings 表/菜单栏，冻结 SetPinned 等回收站 API；`notes:changed` 同步语义保留；原型 109 项语料进前端 vitest。

- [可执行 spec 合成](issues/06-synthesize-execution-spec.md) — **完成**（2026-09-05）。产出 [spec.md](spec.md)（含 13 条验收标准）+ CONTEXT.md 术语更新 + ADR-0001/0002；01 research 不并入（关键结论已内联 spec §2，报告标注为必读附件）。**Map Destination 达成。**

## Not yet specified

- 无。执行期的两个实现级事项已写入 spec（resize 原生路径实测 §1.2；`setTheme` binding 形式随 wails3 CLI 可用性定）。

## Out of scope

- 搜索 UI（随主面板移除）。
- 回收站 UI（恢复 / 彻底删除入口）；仅保留 60 天自动 purge 后台逻辑。
- 悬浮格式工具条（SelectionToolbar）——先看用户反馈。
- Windows/Linux 平台行为差异（当前仅 macOS 交付形态）。
- 行内图片支持（03 定稿：存储方案未定前不引入，另提 ticket）。

## Destination 达成

**2026-09-05：全部 6 张工单 resolved，可执行 spec 在 [spec.md](spec.md)，无剩余决策。** 后续工作为按 spec 开工实现（4 个提交建议：编辑器晋升 → pin 窗口重写 → Go 侧改造 → 文档与测试），不再经过本 map。
