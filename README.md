# PinNote

打开即写、自动保存、启动恢复的桌面便签。**每条笔记就是一个常驻桌面的 pin 窗口**——无边框、置顶、跨 Space，没有主面板，基于 **Wails v3 + React 19 + TypeScript + TipTap v3 + Go + SQLite** 构建。

## 产品形态

- **打开即写**：`⌘⌥N` 在任何应用下呼出一个新 pin 窗口，光标就位；整窗是 TipTap WYSIWYG 编辑器，`# `、`- `、`- [ ] `、`**粗体**` 等语法原地即时渲染，无编辑/预览切换。
- **自动保存**：编辑 400ms 后自动落库；窗口失焦立即保存；关窗/删除前强制 flush。`note.content` 是 Markdown 事实源，每次保存经过 `parse(serialize(doc)) ≡ doc` 守卫，不能无损往返的内容（HTML 注释、行内图片等）以 rawSource 块原样保留，零静默丢失。
- **启动恢复**：关窗只是收起（Stickies 式）；退出重启后全部 live 笔记的窗口按原样恢复，主题保持一致。
- **空笔记硬删**：从未输入过内容的窗口关闭即删除记录，不进回收站。
- **回收站**：`⌘⌫` 删除的笔记保留 **60 天**（无 UI），启动时自动清除过期笔记。
- **驻留与托盘**：所有窗口关闭后应用仍在后台运行；菜单栏图钉图标是驻留态的可见锚点——左键直接唤起**最新笔记**（一条笔记都没有时就地新建一条），右键开托盘菜单（新建笔记 / 显示笔记列表 / 退出）；「显示笔记列表」打开轻量索引面板，列出全部 live 笔记，点一条即聚焦其窗口并收起，失焦自动收起。

## 快捷键

| 键 | 动作 | 作用域 |
|---|---|---|
| `⌘⌥N` | 新建笔记并打开 pin 窗口 | 全局 |
| `⌘B` / `⌘I` / `⌘K` | 粗体 / 斜体 / 链接 | pin 窗口内 |
| `⌘⇧D` | 切换亮/暗主题（所有窗口同帧生效） | 全局生效 |
| `⌘⌫` | 删除当前笔记（进回收站）并关窗 | pin 窗口内 |
| `⌘⇧/` | 快捷键帮助模态框 | pin 窗口内 |
| `Esc` | 关闭帮助模态框 | pin 窗口内 |
| 单击图钉图标 | 唤起最新笔记（无笔记则新建） | 菜单栏 |

应用菜单栏（PinNote）提供 新建笔记 / 删除当前笔记 / 切换主题 兜底项；托盘右键菜单提供 新建笔记 / 显示笔记列表 / 退出。

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

# 打包 .app（bin/PinNote.app）——注意 `wails3 build` 只更新裸二进制 bin/PinNote，
# 不会刷新 .app；从 .app 启动前必须先 package，否则跑的是旧版本
wails3 package

# 重新生成前端绑定（修改 Go service 后）
wails3 generate bindings

# 运行测试（Go 单元测试 + 前端 vitest 132 项，其中 Markdown 往返语料网 51 条语料）
# 注意：先跑过 `npm run build` 产出 frontend/dist，否则 `go test`/`go vet` 会因为
# main.go 的 //go:embed all:frontend/dist 找不到文件而编译失败
cd frontend && npm test && cd ..
go test .
```

## 发版

打 tag 即发版：`.github/workflows/release.yml` 在 `macos-latest` 上串起完整流水线——前端
`vitest` + `tsc` + `vite build` → `go vet` + `go test` → 写入版本号 → `wails3 package` →
样式化 `.dmg`（无 GUI 会话时回退 `hdiutil`）→ ad-hoc 签名校验 → GitHub Release。

```bash
git tag v0.2.0
git push origin v0.2.0
```

产物：`PinNote-<版本>-macOS-<arch>.zip`（`ditto --keepParent`，保留 bundle 结构与签名）、
同名 `.dmg`、`SHA256SUMS.txt`。

- 手动 `workflow_dispatch` 只构建并上传 workflow 产物，**不发 release**，用来验证流水线。
- tag 与 `build/darwin/Info.plist` 版本不一致时以 tag 为准，CI 在打包前改写 plist（仓库里那份不动）。
- 只出 arm64，且没有 Developer ID 签名与 notarize：用户首次打开要 **右键 → 打开**。要正式签名需配证书与
  notary 凭据，见 `wails3 signing` 与 `build/darwin/Taskfile.yml` 的 `sign` 任务。

## 架构

- `main.go` — 应用入口：应用菜单（含 `EditMenu` role，否则 webview 里的 ⌘V/⌘C/⌘X 收不到 AppKit 分发）、全局快捷键（⌘⌥N → CreateNote + OpenPinnedWindow）、启动 purge 过期回收站、恢复全部 live 窗口。
- `tray.go` — 菜单栏状态项：macOS 用单色 template 图标、其他平台用彩色图标；左键走 `summonLatest`（唤起最新笔记），托盘菜单的「显示笔记列表」开/定位面板窗口（路由 `/#/panel`）并在 `WindowLostFocus` 时收起。
- `db.go` — SQLite（`modernc.org/sqlite`，纯 Go 无 CGO），`notes` 与 `settings`（key-value）两张表，数据存于 `~/Library/Application Support/PinNote/pinnote.db`。
- `note_service.go` — 笔记 CRUD、`deriveTitle`（保存路径内从首行派生标题）、`DiscardIfEmpty`（空笔记硬删）、`latestLiveNote`（菜单栏左键的唤起目标，未导出以免扩大绑定 API）、回收站 purge；变更后广播 `notes:changed`。`SetPinned`/`RestoreNote`/`DeleteNoteForever`/`EmptyTrash` 为冻结 API（pin-only 形态退役，仅为数据兼容保留）。
- `window_service.go` — pin 窗口管理：无边框（保留 `Titled|Resizable` mask，原生边缘可拖拽调尺寸，最小高 150）、置顶、跨 Space；`HidePanel`（托盘面板收起）、`RequestFrontmostDelete`（菜单兜底 ⌘⌫）；主题单一事实源（`GetTheme`/`SetTheme` 读写 settings 表并广播 `theme:changed`）。
- `frontend/src` — `PinWindow.tsx`（主界面，路由 `/#/pin/<id>`）、`TrayPanel.tsx`（托盘面板，路由 `/#/panel`）、`PinEditor.tsx`（TipTap 整窗编辑器：审计装载、保存守卫、三层 flush）、`lib/audit.ts`（逐块审计 + rawSource 降级 + 保存守卫）、`hooks/usePinShortcuts.ts`、`theme.ts`（镜像 Go 主题）、TanStack Query（数据缓存与 `notes:changed` 失效）。

## 决策记录

- [ADR-0001 移除主面板](docs/adr/0001-remove-main-panel.md)
- [ADR-0002 Markdown 事实源 + WYSIWYG](docs/adr/0002-markdown-source-of-truth-wysiwyg.md)
- [TipTap × Markdown 往返能力调研](docs/research/tiptap-markdown-roundtrip.md)（spec §2 的完整论据；文中引用的 `MarkdownView.tsx`/`lib/markdown.ts` 已随主面板移除，属历史记录）。
