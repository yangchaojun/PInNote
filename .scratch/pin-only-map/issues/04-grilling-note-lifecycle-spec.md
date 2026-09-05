# 04 — 钉死：笔记生命周期与数据形态 spec

Type: grilling
Status: resolved

## Question

没有主面板后，笔记的完整生命周期规则是什么？逐项定稿：

1. 空笔记策略：`⌘⌥N` 可能反复产生从未编辑过的笔记——关闭未编辑过的空 pin 窗口时是否自动丢弃（软删进回收站 vs 硬删 vs 保留）？防止数据垃圾。
2. 取消固定 / 关闭 / 删除三个动作的语义边界：关窗不取消固定（重启恢复）、取消固定关窗、删除进回收站关窗——确认无遗漏组合。
3. 自动保存：现 400ms debounce 在纯 WYSIWYG 下沿用？关窗/退出时 flush？
4. 主题持久化：现依赖 `localStorage` + storage 事件跨窗口同步（`theme.ts`）。pin-only 形态下主题归谁持久化（localStorage 每 WebView 独立吗？迁移到 Go 侧或 DB？）。
5. 存量数据迁移：升级后 DB 中存在未 pin 的笔记——用户入口消失，处理策略（首次启动提示？保留后台不显示即视为可接受？）。
6. 应用退出路径：所有 pin 关闭后驻留，从 Dock/菜单栏退出——确认退出时保存状态。

产出：笔记生命周期 spec。

Blocked by: (无)

## Answer

**定稿日期：2026-09-05**。grilling 提问未获应答，按提问时给出的推荐项落定；四项均为可推翻的默认决策，推翻时只需改本 Answer 与 map 对应行，不影响 03 已定稿内容。

### 1. 关窗语义 = Stickies 式收起（核心决策）

- **关窗 = 收起**：笔记保留、`pinned` 不变，本次会话不再显示；**启动时恢复所有 live 笔记的窗口**（不再按 `pinned` 过滤）。
- 主面板移除后没有任何"再打开"入口，只有"每个 live 笔记始终对应一个窗口"才能保证笔记永远可达——这是本决策的根据。Apple Stickies 同构。
- **"固定 / 取消固定"概念整体退役**：`Note.Pinned` 字段与 `SetPinned` API 后端保留（兼容 + 未来可能复用），前端不再调用；`restorePinnedWindows` 改为恢复全部 live 笔记。
- **动作边界（无遗漏枚举）**：

| 动作 | 入口 | 笔记状态 | 窗口 |
|---|---|---|---|
| 关闭（收起） | header `×` | 保留，live | 关；下次启动恢复 |
| 删除 | `⌘⌫` / 菜单栏 | 进回收站（60 天 purge） | 关，且其他窗口同步感知关闭 |
| 退出应用 | Dock / 菜单栏 / `⌘Q` | 全部保留 | 全关；下次启动全恢复 |

- 不存在"取消固定关窗但保留窗口"之类的混合动作；主面板时代的 `⌘P` 已随 03 出界。

### 2. 空笔记策略 = 空白即硬删

- 关窗（含 `⌘Q` 退出前的收尾，见 §3 flush 顺序）时若 `content` trim 后为空 → **硬删**（`DELETE`，不进回收站——空白内容无数据价值，也不占 60 天名额）。
- 判定基准是**当前 content**，不是"是否编辑过"：输入后全部删空的窗口同样丢弃；输入过内容的笔记不受影响。
- `⌘⌥N` 连按产生零垃圾。
- 实现落点：前端关窗前调用新增的 `discardIfEmpty(noteId)`（05 定接口位置），Go 侧单条 SQL。

### 3. 自动保存 = 400ms debounce + 三层 flush

- 400ms debounce 沿用（与 ticket 02 原型实测的节奏一致）。
- **三层 flush 覆盖所有退出路径**：
  1. **失焦 flush**：窗口失焦（blur）立即保存——最常用的兜底，用户点开别的窗口即落库；
  2. **动作 flush**：关闭窗口 / `⌘⌫` 删除的流程内**先同步 flush 再执行关窗/删除**（保证 §2 的空白判定与真实内容一致）；
  3. **`beforeunload` 兜底**：覆盖 `⌘Q`/Dock 退出时后台窗口在途的 debounce。
- Go 侧无需退出钩子（保存请求全部来自前端，`app.Run()` 返回即结束）。

### 4. 主题持久化 = 迁移到 Go 侧

- `localStorage` + storage 事件方案废弃：它依赖"所有窗口同源共享同一 localStorage"这一未验证假设，且新窗口打开时有主题闪烁风险；pin-only 下每个窗口都是独立 WebView，多窗口主题不一致的暴露面变大。
- 新方案：`WindowService` 已持有 `theme` 内存值（`SetTheme` 已做全窗口背景色同步）——扩展为**持久化**（settings 存储位置归 05/06 定，倾向 DB 内一行或 app 配置文件）+ 新增 `GetTheme()` 供前端启动时读取初始值；切换主题 = 前端调 `SetTheme` → Go 广播事件 → 所有窗口统一应用。**单一事实源在 Go，消灭跨窗口同步问题。**
- 前端 `theme.ts` 收缩为"从 Go 读初始值 + 应用到 DOM"，zustand persist 与 storage 监听删除。

### 5. 存量数据迁移 = 无需迁移（被 §1 消解）

- 启动恢复所有 live 笔记 → 未 pin 的存量笔记升级后自动获得窗口，无滞留数据、无首次启动提示。
- 唯一残留：存量笔记的 `pinned` 字段值不再有语义（冻结，不清洗）。

### 6. 应用退出路径 = 驻留行为不变

- `ApplicationShouldTerminateAfterLastWindowClosed: false` 保留：所有窗口关闭后应用继续驻留（Dock + 菜单栏），`⌘Q`/Dock 右键退出。
- 退出时数据安全性由 §3 三层 flush 保证；启动顺序为 purge → 恢复全部 live 笔记窗口 → 主题从 Go 读入。

### 移交给后续 ticket 的输入

- **05（影响清单）**：`SetPinned`/固定按钮出界；`restorePinnedWindows` 改"恢复全部 live"；新增 `discardIfEmpty`；`GetTheme`/`SetTheme` 持久化改造；`theme.ts` 收缩；失焦/beforeunload flush 的挂载点。
- **06（合成）**：本 Answer 并入 spec 的"生命周期"章；`pinned` 退役需在 CONTEXT.md 术语中标注。
