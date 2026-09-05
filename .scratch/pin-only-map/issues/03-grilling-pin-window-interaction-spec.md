# 03 — 钉死：pin 窗口交互与窗口规格 spec

Type: grilling
Status: resolved

## Question

纯 pin 形态下，pin 窗口的完整交互规格是什么？逐项定稿：

1. 高度策略落地：默认 300px / 最小 150px（现状 `MIN_HEIGHT=120` 与自适应增高逻辑将移除）、手动 resize 的实现方式（Frameless macOS 窗口如何拖边调整）、超出内部滚动的滚动条样式。
2. 标题语义：移除主面板后 `note.title` 如何产生/展示——自动取首行/首个标题？头部按钮悬停显示？定稿其唯一规则（注意 `note.title` 已参与搜索与 `Note.title` 数据模型）。
3. 头部区域：拖拽区（现为 `pin-header`，双击切编辑的绑定移除）、固定/关闭按钮去留、新增"删除（进回收站）"与"主题切换"按钮的排布。
4. 快捷键最终集合：`⌘⌥N` 新建（改道后）、`⌘B/⌘I/⌘K`、`⌘⇧D` 主题、`⌘⌫` 删除、`⌘W` 关窗？逐键定稿；快捷键帮助模态框内容随之重写。
5. 空笔记编辑态的 placeholder 文案。

产出：pin 窗口交互 spec（作为 Answer 或链接的独立 spec 文件）。

Blocked by: 02

## Answer

**定稿日期：2026-09-05**（grilling，用户逐项确认）。本 Answer 即 pin 窗口交互 spec，并入 06。

> **对既有决策的修订**：header 上**只保留关闭按钮**——删除与主题切换不进 header（修订 map Q5(b) 的"收进 pin 工具条"），入口改为快捷键（`⌘⌫` / `⌘⇧D`）+ 应用菜单栏项。`⌘W` 不保留：关窗的按钮入口唯一（header 关闭按钮）；`⌘⌫` 删除会连带关窗，是唯一的非按钮关窗路径（见 §4）。

### 1. 窗口规格与 resize

- **尺寸**：宽固定 380px（`PIN_WIDTH` 不变）；高默认 300px、最小 150px、无上限（移除 `MAX_HEIGHT_FRACTION` 屏高上限——高度完全由用户控制）。
- **自适应增高逻辑整体移除**：`PinWindow.tsx` 的 `ResizeObserver` + `WailsWindow.SetSize` 回路删除，窗口尺寸只由用户拖拽决定。
- **resize 实现路径（源码核查结论，实现首步须实测）**：
  - Wails runtime 的 JS 边缘热区（`@wailsio/runtime` `drag.ts` 的 5px/角落 10px 热区 → `wails:resize:<edge>`）**只对 Windows / Linux-frameless 生效**——安装版 `drag.js` 有 `if (!resizable || (!IsWindows() && !(IsLinux() && GetFlag("frameless")))) return;` 守卫，macOS 上不可达。不能依赖此路径。
  - macOS 的实际路径：frameless + 默认 `CornerType: Rounded`（radius 0）时，Wails 保留原生 AppKit frame（`NSWindowStyleMaskTitled|Closable|Miniaturizable|Resizable`，按钮隐藏、`titlebarAppearsTransparent` + 标题隐藏）→ **预期原生边缘 resize 直接可用**（主窗口 `InvisibleTitleBarHeight` 同样依赖原生 frame，为旁证）。
  - **实现验收第一步**：实测 frameless pin 窗口的原生边缘拖拽；若原生热区被 webview 事件吞掉（AppKit 边框命中失败），回退方案 = 自绘 CSS resize 手柄（右缘/下缘/右下角 6px 热区）+ `WailsWindow.SetSize`，前端钳制最小 150px。两条路径都不需要自定义 Go 侧手柄协议。
  - 不论哪条路径：`OpenPinnedWindow` 的 `WebviewWindowOptions` 补 `MinHeight: 150`（当前缺失）。
- **拖拽**：保留现状 `--wails-draggable: drag`（header）+ `nodrag`（按钮）；`onDoubleClick` 切编辑绑定随编辑/预览模式一起移除。
- **内部滚动**：`pin-body` 改 `overflow-y: auto`；滚动条为细覆盖式（`::-webkit-scrollbar` 宽 8px、圆角、悬停窗体时显示，静止时透明），不挤压正文宽度。
- **留白边界**：正文内容距窗口边缘有安全边距（现 `padding: 4px 16px 14px` 保留），resize 热区与滚动条不遮正文。

### 2. 标题语义（首行即标题，Apple Notes 式）

- **唯一规则**：`title ≡ deriveTitle(content)`，保存时从 content 首行派生，无独立可编辑标题。
- **派生算法**（Go 侧 `note_service.go` 的 `deriveTitle`，已存在，小幅增强而非重写；前端不传 title）：
  1. 沿用现状：取 content 第一个非空行；`TrimLeft` 剥除行首块语法字符（`#>-*+[0-9] `）；截断至 60 rune（超出加 `…`）；无非空行回退 `"无标题笔记"`；
  2. 新增增强：先剥复选框前缀 `- [ ] `/`- [x] `（现字符集剥除会残留 `[ ] `）；配对剥除行内包裹记号 `` ` ``/`**`/`*`/`_`/`~~`（Apple Notes 式首行摘要不含记号）。
- **数据模型不变**：`Note.title` 字段保留，变为派生缓存；搜索为前端内存过滤（title/content 包含匹配），无需改动。存量数据在下次保存时自然收敛，不做一次性迁移。
- **展示**：header **平时不显示标题文字**（只有图钉图标 + 拖拽区 + 关闭按钮）；悬停 header 拖拽区时以原生 tooltip 显示首行标题（空则"无标题笔记"）。主面板消失后无其他标题展示位。

### 3. 头部区域（最终排布）

```
[图钉图标]  [拖拽区 flex:1，悬停显示首行标题 tooltip]  [× 关闭]
```

- 图钉图标保留（品牌/身份识别）；拖拽区即标题 tooltip 悬停区。
- **移除**：编辑/预览切换按钮（整窗 WYSIWYG）、取消固定按钮（pin-only 下与"关闭"语义重合，语义边界归 04）、删除按钮、主题按钮（入口见快捷键与菜单栏）。
- 关闭按钮：点击 = 关窗（不删除笔记；关窗与固定的语义边界归 04 定稿，03 仅固定"按钮发出 `close` 动作"）。

### 4. 快捷键最终集合

| 快捷键 | 动作 | 备注 |
|---|---|---|
| `⌘⌥N` | 新建笔记 pin 窗口 | 全局；改道为直接开新 pin 窗口（不再聚焦主面板，实现归 05） |
| `⌘B` / `⌘I` / `⌘K` | 粗体 / 斜体 / 链接 | 编辑器内生效（Q9 保留；块格式 `⌘⇧X/7/8/9` **不保留**） |
| `⌘⇧D` | 切换亮/暗主题 | 全局生效（所有 pin 窗口 + 新建窗口） |
| `⌘⌫` | 删除当前笔记（进回收站）并关窗 | pin 窗口内生效；**删除唯一入口**（header 无删除按钮），菜单栏保留同名菜单项 |
| `⌘⇧/` | 快捷键帮助模态框 | 内容按上表重写；窗口内生效 |

- **明确不保留**（随主面板一起出界）：`⌘N`（与全局 `⌘⌥N` 合并）、`⌘F` 搜索、`⌘E` 预览切换、`⌘P` 固定、`⌘⇧T` 回收站视图、`⌥↑/↓` 笔记切换、`⌘W` 关窗。
- Esc：仅用于关闭帮助模态框；不绑定其他行为（编辑态下 Esc 无动作，避免误触失焦）。
- 主题切换在菜单栏保留一项（与 `⌘⇧D` 同义）；删除在菜单栏保留"删除笔记"项（与 `⌘⌫` 同义）——这是无 header 按钮后的可发现性兜底。

### 5. 空笔记 placeholder

- WYSIWYG 编辑器空文档时显示 placeholder：**「记点什么…」**（Apple Notes 式；经 `@tiptap/extension-placeholder` 或等效 ProseMirror 装饰实现）。
- 现文案「用 Markdown 记录…」废弃——pin-only 形态面向普通便签用户，弱化 Markdown 心智。

### 6. 行内图片（ticket 02 移交的产品决策）

- **本期不支持**：`@tiptap/extension-image` 不引入；image token 保持在 02 的审计排除清单——含 `![alt](url)` 的 Markdown 块降级为 rawSource 源码块显示，**零数据丢失**（与注释/链接定义同策略）。
- 理由：引入渲染只解决"已有 URL 的图片"，粘贴本地图片仍无存储路径（SQLite 无 blob 方案、无 assets 目录），半支持反而制造"图挂了"的困惑。将来支持时需先定图片存储方案，另提 ticket；届时 02 的审计语料网需先补 image 用例，才可把 image 移出排除清单。

### 移交给后续 ticket 的输入

- **04（生命周期）**：关窗语义（是否等于取消固定/是否自动恢复）、空笔记丢弃策略——03 已固定"关闭按钮只发 close 动作、删除走 `⌘⌫`/菜单栏"。
- **05（影响清单）**：`⌘⌥N` 改道实现；`MinHeight: 150` 进 `OpenPinnedWindow`；菜单栏增"删除笔记""切换主题"两项；`ShortcutsModal.tsx` 按上表重写；`useMainShortcuts.ts` 大部分逻辑出界，pin 窗口需自己的键盘处理（⌘B/I/K、⌘⌫、⌘⇧D、⌘⇧/）。
- **06（合成）**：本 Answer 全文并入 spec；注意记录 Q5(b) 修订与 `⌘W` 否决。
