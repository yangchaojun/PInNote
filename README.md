# PinNote

键盘优先的桌面便签笔记应用。基于 **Wails v3 + React 19 + TypeScript + Vite + Ant Design + TanStack Query + Zustand + Go + SQLite** 构建。

## 功能

- **Markdown 支持**：标题、列表、有序/无序列表、待办复选框（`- [ ]`）、粗体/斜体/删除线、行内代码、链接、引用、表格等 GFM 语法；主窗口支持"编辑 + 实时预览"分屏，预览中的复选框可以直接勾选并回写源文本。
- **键盘优先**：常用操作全部有快捷键，无需鼠标：

  | 快捷键 | 功能 |
  | --- | --- |
  | `⌘⌥N`（全局） | 在任何应用中呼出 PinNote 并新建笔记 |
  | `⌘N` / `⌘F` | 新建笔记 / 搜索笔记 |
  | `⌥↓` / `⌥↑` | 切换下一条 / 上一条笔记 |
  | `⌘E` | 切换 Markdown 预览 |
  | `⌘P` | 固定 / 取消固定当前笔记到桌面 |
  | `⌘⌫` | 移入回收站 |
  | `⌘⇧T` | 切换笔记 / 回收站视图 |
  | `⌘⇧D` | 切换暗色 / 亮色主题 |
  | `⌘B` / `⌘I` / `⌘K` | 粗体 / 斜体 / 链接 |
  | `⌘⇧7` / `⌘⇧8` / `⌘⇧X` | 有序列表 / 无序列表 / 待办事项 |
  | `⌘⇧9` / `⌘⇧/` | 引用块 / 快捷键帮助 |

- **自动调整窗口**：笔记固定到桌面后成为独立的无边框置顶窗口，窗口高度随内容自动调整（`ResizeObserver` + `Window.SetSize`），也可在固定窗口中直接编辑。
- **回收站恢复**：删除的笔记保留 **60 天**，可随时恢复或彻底删除；启动时自动清除超过 60 天的笔记（`PurgeExpiredTrash`，有单元测试覆盖）。
- **Pin 到桌面**：固定的笔记以置顶、无边框、跨 Space 的桌面窗口显示，重启应用后自动恢复所有固定窗口。
- **暗 / 亮主题**：工具栏按钮或 `⌘⇧D` 一键切换；主题持久化到 `localStorage`（重启保留，固定窗口通过 `storage` 事件实时跟随），并经 `WindowService.SetTheme` 同步原生窗口背景色。

## 开发

```bash
# 安装 Wails v3 CLI（若未安装）
go install github.com/wailsapp/wails/v3/cmd/wails3@latest

# 安装前端依赖
cd frontend && npm install && cd ..

# 开发模式（热重载）
wails3 dev

# 生产构建（产物在 bin/）
wails3 build

# 重新生成前端绑定（修改 Go service 后）
wails3 generate bindings

# 运行后端单元测试
go test .
```

## 架构

- `main.go` — 应用入口：主窗口、全局快捷键（⌘⌥N）、启动时清理过期回收站、恢复固定窗口。
- `db.go` — SQLite（`modernc.org/sqlite`，纯 Go 无 CGO），数据存于 `~/Library/Application Support/PinNote/pinnote.db`。
- `note_service.go` — 笔记 CRUD、回收站（60 天保留）、pin 状态；变更后广播 `notes:changed` 事件同步所有窗口。
- `window_service.go` — 固定窗口管理：无边框、置顶（`MacWindowLevelFloating`）、隐藏任务栏。
- `frontend/src` — React 前端：`App.tsx`（主窗口）、`PinWindow.tsx`（固定窗口，路由 `/#/pin/<id>`）、`theme.ts`（主题 store：持久化、`data-theme` CSS 变量切换、跨窗口同步）、Zustand（UI 状态）、TanStack Query（数据缓存与失效）。
