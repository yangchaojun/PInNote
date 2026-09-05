# PinNote 纯 pin 形态改造 — 可执行 spec

> 来源：wayfinder map `.scratch/pin-only-map/`（tickets 01–05 已全部 resolved）。
> 本文件是唯一开工依据；各 ticket Answer 为论据与推演过程，冲突时以本文件为准。

## 0. 产品形态（一句话）

PinNote 没有主面板：**每条笔记就是一个常驻桌面的 pin 窗口**——无边框、置顶、打开即写、自动保存；启动恢复全部笔记，删除进 60 天回收站（无 UI）。

## 1. 窗口规格（03 §1）

1. 宽固定 **380px**；高默认 **300px**、最小 **150px**、无上限；移除自适应增高（`ResizeObserver`+`SetSize` 回路删除），窗口尺寸只由用户拖拽决定。
2. resize 走 **macOS 原生 frame**（Wails frameless + 默认 Rounded corner 保留 `Titled|Resizable` mask）——实现**第一步必须实测**原生边缘拖拽；若被 webview 吞事件，回退：自绘 CSS resize 手柄（右缘/下缘/右下角 6px 热区）+ `WailsWindow.SetSize`（前端钳制 ≥150px）。**不得**依赖 Wails runtime 的 JS 边缘热区（`drag.js` 有 `IsWindows()/IsLinux()` 守卫，macOS 不可达）。
3. `OpenPinnedWindow` 补 `MinHeight: 150`。
4. 拖拽：header `--wails-draggable: drag`（现状保留）；`onDoubleClick` 绑定移除（原编辑/预览切换）。
5. 正文超出窗口高度时**内部滚动**：`pin-body` `overflow-y: auto`，细覆盖式滚动条（`::-webkit-scrollbar` 8px、圆角、窗体悬停时显示、静止透明），不挤压正文宽度；正文安全边距沿用 `padding: 4px 16px 14px`。

## 2. 编辑器实现方案（02 Answer + 01 research）

1. 整窗 contenteditable WYSIWYG（TipTap v3 + 官方 `@tiptap/markdown` + StarterKit），打开即编辑态，无编辑/预览切换。
2. **从原型晋升**（`prototype/wysiwyg-editor/src/lib/` → `frontend/src/lib/`）：`audit.ts`、`context.ts`（`createAuditContext`，语料网的依赖，缺它 109 项测试无法运行）、`rawSource.ts`（直通原子块节点）、`SafeParagraph`（editorConfig.ts，段落序列化转义行首块语法）、`roundtrip.test.ts`+`fixtures.ts`（109 项语料网进前端 vitest）。原型的 `App.tsx` 沙盒壳丢弃，但其中定义的 `clipboardTextSerializer`（复制输出 → `getMarkdown`）随 `PinEditor.tsx` 正式化带入。
3. 五条实测铁律随代码晋升：装载按块 trim；守卫比较前归一化顶层空段落；SafeParagraph 转义；rawSource「转为可编辑」必须重过审计；image 在 token 类型门排除清单中（见 §6）。
4. 富文本粘贴 → Markdown；纯 Markdown 文本粘贴走同一审计装载器（`MarkdownPaste` 扩展）；复制输出经 `clipboardTextSerializer → getMarkdown`。
5. 行首块语法 input rules（`# `/`- `/`- [ `）为 StarterKit 自带能力，验收走人工清单（自动化送不进逐键事件）。
6. 空文档 placeholder：**「记点什么…」**（`@tiptap/extension-placeholder` 或等效装饰）。
7. 保存 = 事实源回写：编辑 400ms debounce 后 `getMarkdown()` → `UpdateNote`；`parse(serialize(doc)) ≡ doc`（空段落归一化后）守卫不过则**拒写并保留降级态**，不得静默丢内容。

## 3. 标题语义（03 §2）

1. `title ≡ deriveTitle(content)`，Go 侧保存路径内派生，前端不传 title；无独立可编辑标题。
2. `deriveTitle` 增强（现为 60 rune 截断 + "无标题笔记"回退，沿用）：新增剥复选框前缀 `- [ ] `/`- [x] `；配对剥行内记号 `` ` ``/`**`/`*`/`_`/`~~`。
3. header 平时不显示标题；悬停拖拽区以原生 tooltip 显示首行标题（空则"无标题笔记"）。
4. 搜索为前端内存过滤（title/content 包含匹配），数据模型与 `Note.title` 字段不变（降为派生缓存）；存量数据下次保存自然收敛，无一次性迁移。

## 4. 交互与快捷键（03 §3–§4）

1. Header 最终排布：`[图钉图标] [拖拽区 flex:1，悬停标题 tooltip] [× 关闭]`。编辑/预览、取消固定、删除、主题按钮全部不存在。
2. 快捷键全集：

| 键 | 动作 | 作用域 |
|---|---|---|
| `⌘⌥N` | 新建笔记并打开 pin 窗口 | 全局 |
| `⌘B` / `⌘I` / `⌘K` | 粗体 / 斜体 / 链接 | pin 窗口内 |
| `⌘⇧D` | 切换亮/暗主题 | 全局生效（所有窗口） |
| `⌘⌫` | 删除当前笔记（进回收站）并关窗 | pin 窗口内 |
| `⌘⇧/` | 快捷键帮助模态框（内容=本表） | pin 窗口内 |
| `Esc` | 仅关闭帮助模态框 | pin 窗口内 |

3. 明确不保留：`⌘W`、`⌘N`、`⌘F`、`⌘E`、`⌘P`、`⌘⇧T`、`⌥↑/↓`、块格式键 `⌘⇧X/7/8/9`。
4. 菜单栏兜底项：新建笔记（`⌘⌥N`）、删除笔记（`⌘⌫`，作用于最前 pin 窗口）、切换主题（`⌘⇧D`）、退出。
5. 关窗的按钮入口唯一（header `×`）；`⌘⌫` 删除连带关窗，是唯一非按钮关窗路径。

## 5. 生命周期（04）

1. **关窗 = 收起（Stickies 式）**：笔记保留、`pinned` 不变；**启动恢复全部 live 笔记的窗口**（不再按 `pinned` 过滤）。固定/取消固定概念退役；`Pinned` 字段与 `SetPinned` API 后端冻结。
2. **空笔记硬删**：关窗（及退出收尾）时 `content` trim 空白 → `DiscardIfEmpty` 直接 `DELETE`（不进回收站）。判定基准是当前 content。
3. **自动保存**：400ms debounce + 三层 flush——窗口失焦立即 flush；关窗/删除流程先同步 flush 再动作；`beforeunload` 兜底（覆盖 `⌘Q`）。Go 侧无需退出钩子。
4. **主题**：单一事实源在 Go——`settings` 表持久化（`db.go` 新增 key-value 表），`GetTheme()` 供启动读取，`SetTheme` 写盘 + 背景色同步 + `theme:changed` 广播；前端 `theme.ts` 删 zustand persist 与 storage 监听。
5. **多窗口同步**：`notes:changed` 广播 + react-query invalidation 保留现状（他窗修改/删除感知、trashed 窗口自动关闭都依赖它）。
6. **退出/驻留**：`ApplicationShouldTerminateAfterLastWindowClosed: false` 不变；启动顺序 purge → 恢复全部 live 窗口 → 各窗口从 Go 读主题。

## 6. 行内图片

本期不支持：不引入 `@tiptap/extension-image`；image 保持在审计排除清单，含 `![alt](url)` 的块降级 rawSource 显示，零数据丢失。将来支持需先定图片存储方案（另提 ticket），且 02 语料网先补 image 用例。

## 7. 逐文件改动地图（05 Answer 全文有效）

前端：删 `App.tsx`、`NoteList`、`TrashView`、`Editor`、`MarkdownView`、`useMainShortcuts`（换 `usePinShortcuts`）、`lib/markdown.ts`（全部现存函数随 textarea/MarkdownView 出界；晋升代码的 Markdown 解析由 audit 体系自带）、`store.ts`；重写 `PinWindow.tsx`（核心）、`ShortcutsModal.tsx`、`theme.ts`；调整 `lib/api.ts`、`main.tsx`（只留 `/#/pin/:id`）、`style.css`；新增 `PinEditor.tsx` + 晋升 `lib/{audit,context,rawSource,editorConfig}.ts`。

Go：删主窗口创建、`mainBackground`、`FocusMainWindow`；改道 `⌘⌥N`（CreateNote → OpenPinnedWindow，`app:new-note` 事件消亡）；重写 `restoreAllNoteWindows`（恢复全部 live）；`OpenPinnedWindow` 补 MinHeight；新增 `GetTheme`/`SetTheme` 持久化、`DiscardIfEmpty`、settings 表、菜单栏；冻结 `SetPinned`/`RestoreNote`/`DeleteNoteForever`/`EmptyTrash`/`GetTrashRetentionDays`；调整 `deriveTitle`。

测试：`note_service_test.go` 增 DiscardIfEmpty / deriveTitle 增强 / restoreAll 用例；原型 109 项语料进前端 vitest；`usePinShortcuts` 键位、空笔记丢弃、flush 顺序做组件级测试。

## 8. 验收标准（每条可指认到界面行为）

1. `⌘⌥N` 在任何应用下按下 → 桌面出现一个新 pin 窗口，placeholder「记点什么…」可见，光标就位。
2. 输入 `**粗体** ` → 原地变粗体；`# ` 起行 → 变标题；`- [ ] ` → 变复选框；点击复选框 → 源同步 `- [x]`。
3. 编辑后 ≥400ms 停手 → DB content 为等价 Markdown（沙盒右栏/检视验证）；窗口失焦 → 立即落库。
4. 含 `![img](url)` 的 Markdown 打开 → 该块以源码形态显示、可编辑文本、保存后原文不丢（对照：不得只剩 alt 文本）。
5. 含 HTML 注释的块 → rawSource 降级显示，「转为可编辑」重过审计后仍不丢。
6. 关闭空白窗口 → DB 中无该笔记记录（硬删）；关闭输入过内容的窗口 → 记录保留。
7. 退出并重启 → 全部 live 笔记窗口按上次内容恢复（含上次关闭收起的），主题与退出前一致。
8. `⌘⌫` → 窗口关闭；其他 pin 窗口无感知异常；重启后该笔记不再出现；60 天后启动时 purge。
9. 拖动窗口右缘/下缘 → 窗口可调整；高度低于 150 不可能；正文超长时窗口内滚动，无原生滚动条挤压。
10. `⌘⇧D` → 所有 pin 窗口与原生背景同帧切换，重启后保持。
11. header 悬停拖拽区 → tooltip 显示首行标题（`**加粗**首行` 显示为「加粗首行」样式剥除后文本）。
12. `⌘⇧/` → 帮助模态框内容与本 spec §4.2 一致；Esc 关闭。
13. 应用无任何窗口时 Dock 仍可退出；`⌘Q` 时后台窗口在途输入不丢（beforeunload flush）。

## 9. 文档同步（本 spec 落地时执行）

- `README.md` 重写（快捷键表=§4.2、卖点改"打开即写/自动保存/启动恢复"）；`application.Options.Description` 同步。
- `CONTEXT.md` 已随 06 更新（见同 commit）；ADR-0001/0002 已记录（`docs/adr/`）。
- 研究附录：01 报告在 `research/tiptap-markdown-roundtrip` 分支 `docs/research/tiptap-markdown-roundtrip.md`，实现编辑器时为必读附件。
