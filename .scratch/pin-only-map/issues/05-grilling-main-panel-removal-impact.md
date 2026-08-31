# 05 — 钉死：主面板移除的影响清单

Type: grilling
Status: open

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
