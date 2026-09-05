# 05 — 钉死：主面板移除的影响清单

Type: grilling
Status: resolved

## Question

移除主面板会波及哪些既有模块，最终形态各是什么？产出一份逐文件的影响/删除/保留清单，作为 spec 的"改动地图"：

- `frontend/src/App.tsx`、`components/{NoteList,TrashView,Editor,ShortcutsModal}.tsx`、`hooks/useMainShortcuts.ts`——删除还是收缩为 pin 专用？
- `window_service.go` 的 `main` 窗口、`mainBackground()`、`FocusMainWindow()`、`restorePinnedWindows` 调整；`main.go` 启动流程与 `ApplicationShouldTerminateAfterLastWindowClosed`。
- 全局快捷键 `⌘⌥N` 改道（04/03 定稿语义）；`app:new-note` 事件的去向。
- `note_service.go` API 表面：`SetPinned`/`TrashNote`/列表接口哪些保留、哪些前端不再调用（后端保留与否）。
- 测试影响：`note_service_test.go` 及新增交互 spec 的可测缝。
- README、应用描述、菜单栏结构。

产出：影响清单 + 每项的去向决策，并入 06 的 spec。

Blocked by: 02, 03, 04

## Answer

**定稿日期：2026-09-05**（基于 03/04 已定稿的交互与生命周期决策，逐文件核对现有源码）。图例：`删除` = 文件/代码移除；`重写` = 原地改造为 pin 形态；`冻结` = 保留但不新增调用方；`新增` = pin 形态需要的新代码。

### 前端（`frontend/src/`）

| 文件 | 去向 | 说明 |
|---|---|---|
| `App.tsx` | **删除** | 主面板唯一装配点。被吸收的逻辑：`createNote` 走 05 改道后的 Go 直开路径（见下）；`notes:changed` → invalidate 移入 `PinWindow`（已有）；定时器清理随编辑器重做。 |
| `components/NoteList.tsx` | **删除** | 列表 UI 出界。 |
| `components/TrashView.tsx` | **删除** | 回收站 UI 出界（60 天 purge 保留）。 |
| `components/Editor.tsx` | **删除** | textarea 编辑器，被 WYSIWYG 编辑器（02 原型晋升）替代。 |
| `components/MarkdownView.tsx` | **删除** | 只读渲染与复选框点击由 WYSIWYG 编辑器接管。 |
| `components/ShortcutsModal.tsx` | **重写** | 内容按 03 §4 快捷键表重写；挂载点移入 `PinWindow`（每个 pin 窗口独立打开）。 |
| `components/PinWindow.tsx` | **重写**（核心） | ① textarea/MarkdownView 双态 → WYSIWYG 编辑器单态（02 晋升的 audit/rawSource/SafeParagraph + `@tiptap/starter-kit` + `@tiptap/markdown`）；② 删自适应 resize 回路（`ResizeObserver`+`SetSize`），`pin-body` 改内部滚动；③ header 收缩为 [图钉图标 | 拖拽区 | 关闭]，标题 tooltip；④ 编辑器接入 placeholder「记点什么…」；⑤ 保存改 400ms debounce + 失焦/动作/beforeunload 三层 flush；⑥ 关窗前空笔记 `discardIfEmpty`；⑦ trashed 占位分支保留（其他窗口删除时本窗口需感知），文案简化。 |
| `hooks/useMainShortcuts.ts` | **删除**，`新增 usePinShortcuts` | pin 窗口键盘处理：`⌘B/I/K`（编辑器 format）、`⌘⌫` 删除、`⌘⇧D` 主题、`⌘⇧/` 帮助、Esc 关帮助。主面板遗留键不迁移。 |
| `store.ts` | **收缩** | `view`/`searchQuery`/`activeNoteId`/`setView` 等出界；仅 `shortcutsOpen` 保留 → 降为 `PinWindow` 本地 state，store 文件可删。 |
| `theme.ts` | **重写** | 删 zustand `persist` + `storage` 监听；启动从 `api.getTheme()` 读初始值 → `applyTheme`（DOM + `SetTheme` 原生背景）；切换 = 调 Go `SetTheme` → Go 广播 → 各窗口应用（04 §4 单一事实源）。 |
| `lib/api.ts` | **调整** | 删 `setPinned`/`focusMainWindow`/`daysUntilPurge`（随 UI 出界）；增 `discardIfEmpty(id)`、`getTheme()`；`setTheme` 若 wails3 CLI 可用改为正式 binding，否则维持 `Call.ByName`。 |
| `lib/markdown.ts` | **删除** | 全部现存函数（`toggleCheckboxAtLine`/`formatBold` 等与 marked 只读渲染）随 Editor/MarkdownView/useMainShortcuts 出界；晋升代码的 Markdown 解析由 `audit.ts` 体系自带，不保留该文件。 |
| `main.tsx` | **简化** | 路由只留 `/#/pin/:id`；`/` 主面板路由删除（不再有任何窗口加载它；恢复的窗口全部带 pin id）。 |
| `style.css` | **收缩** | 删主面板/列表/回收站段落；pin 段落按 03 更新（header、滚动条、WYSIWYG 编辑器样式）。 |

新增：`components/PinEditor.tsx`（02 原型 `App.tsx` 沙盒壳的正式化：TipTap 实例 + placeholder + paste 扩展）与 `lib/{audit,rawSource,editorConfig}.ts`（从 `prototype/wysiwyg-editor/src/lib/` 晋升，含 `roundtrip.test.ts`+`fixtures.ts` 语料网进 `frontend/` 测试体系）。

### Go 后端

| 位置 | 去向 | 说明 |
|---|---|---|
| `main.go` 主窗口创建 | **删除** | `main` 窗口及其 `Mac` 选项整块移除。 |
| `main.go` `ApplicationShouldTerminateAfterLastWindowClosed` | **保留** | `false` 不变（04 §6 驻留行为不变）。 |
| `main.go` `⌘⌥N` 全局快捷键 | **改道** | 由 `FocusMainWindow + emit("app:new-note")` 改为：`CreateNote("")` → `OpenPinnedWindow(newID)`，**不再发 `app:new-note`**（事件随之消亡，App.tsx 的监听已删）。 |
| `main.go` 菜单栏 | **新增** | 按 03：应用菜单含"新建笔记 `⌘⌥N`"、"删除笔记 `⌘⌫`"（作用于最前 pin 窗口，Go 侧记录 focused window 对应 note id）、"切换主题 `⌘⇧D`"、"退出"。无 UI 时的可发现性兜底。 |
| `window_service.go` `mainBackground()` | **删除** | 主窗口没了。 |
| `window_service.go` `FocusMainWindow()` | **删除** | 无主窗口可聚焦。 |
| `window_service.go` `OpenPinnedWindow` | **调整** | `WebviewWindowOptions` 补 `MinHeight: 150`（03 §1）；其余（Frameless/AlwaysOnTop/Mac 选项）不变。 |
| `window_service.go` `restorePinnedWindows` | **重写** | 按 04 §1：恢复**全部 live 笔记**（去掉 `Pinned` 过滤），改名 `restoreAllNoteWindows`；启动顺序 purge → 恢复 → 前端各自读主题。 |
| `window_service.go` 主题持久化 | **新增** | `theme` 内存值落盘（`GetTheme()` 给前端启动读取；`SetTheme` 写盘 + 广播 `theme:changed` 事件）。存储位置：`db.go` 新增 `settings` 表（key-value），随 SQLite 单文件部署，不引入配置文件。 |
| `note_service.go` `SetPinned` | **冻结** | 后端保留（数据兼容 + 未来复用），前端不再调用（04 §1 固定/取消固定退役）。 |
| `note_service.go` `RestoreNote`/`DeleteNoteForever`/`EmptyTrash`/`ListTrash`/`GetTrashRetentionDays` | **冻结** | 回收站 UI 出界后无前端调用方（`ListTrash` 例外：PinWindow 的 trashed 感知分支仍用）；后端保留——purge 与未来找回入口都建立在它们之上，删除属过度清理。 |
| `note_service.go` `DiscardIfEmpty(id)` | **新增** | 04 §2：`content` trim 空白 → `DELETE`（硬删，绕过回收站）；返回是否丢弃，前端关窗前调用。 |
| `note_service.go` `deriveTitle` | **调整** | 03 §2 增强：剥复选框前缀 `- [ ] `/`- [x] `、配对剥行内记号；60 rune 截断与"无标题笔记"回退沿用。 |
| `note_service.go` `notes:changed` 广播 | **保留** | 多窗口同步语义不变（map 遗留问题的决策：pin-only 下每个窗口仍需感知他窗修改/删除，PinWindow 的关窗感知依赖它）；`theme:changed` 为新增的第二广播。 |
| `db.go` | **新增** settings 表 | `CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`，仅存 `theme`。 |

### 测试

- `note_service_test.go`：增 `DiscardIfEmpty`（空白/非空白/回收站态）、`deriveTitle` 增强用例（复选框、`**粗体**` 首行）；现有用例不受影响。
- 晋升 `prototype/wysiwyg-editor` 的 `roundtrip.test.ts`+`fixtures.ts`（109 项）进前端 vitest。
- 交互可测缝：`usePinShortcuts` 键位表、空笔记丢弃时机、flush 顺序——以组件/hook 级测试覆盖（vitest + testing-library），不追求 e2e。
- Go 侧启动流程（恢复全部 live）加 `restoreAllNoteWindows` 单测（内存 DB + 假 window 服务接口）。

### 文档

- `README.md`：功能描述与快捷键表按 03 §4 重写；删除"回收站恢复/搜索"卖点，新增"打开即编辑、自动保存、启动恢复"。
- `CONTEXT.md`：由 06 同步更新（术语退役/新增）。
- `application.Options.Description`："键盘优先的桌面便签笔记" → "桌面便签：打开即写的 pin 窗口笔记"（或近似，实现时定稿）。
