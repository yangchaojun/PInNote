# PinNote

打开即写、自动保存、启动恢复的桌面便签。**每条笔记就是一个常驻桌面的 pin 窗口**——无边框、置顶、跨 Space，没有主面板，基于 **mygo native UI**（纯 Go 绘制，无 webview、无前端构建）+ **Go + SQLite** 构建。

## 产品形态

- **打开即写**：`⌘⌥N` 在任何应用下呼出一个新 pin 窗口，光标就位；整窗是富文本编辑器（`ui/richtext`），`# `、`- `、`- [ ] `、`**粗体**` 等语法原地即时渲染，无编辑/预览切换。
- **自动保存**：编辑 400ms 后自动落库；窗口失焦立即保存；关窗/删除/更新前强制 flush。`note.content` 是 Markdown 事实源，每次保存经过 round-trip 守卫（serialize→重开→文档相等），不能无损往返的内容（HTML 块、表格、链接定义等）以源码形式的 raw 块原样保留，零静默丢失。
- **启动恢复**：关窗只是收起（Stickies 式）；重启后只恢复**最新一条**笔记的窗口，其余 live 笔记经托盘「显示笔记列表」按需打开，主题保持一致。
- **空笔记硬删**：从未输入过内容的窗口关闭即删除记录，不进回收站。
- **回收站**：`⌘⌫` 或删除按钮（pin 窗口右上角 / 笔记列表 item，均二次确认）删除的笔记保留 **60 天**，可从托盘「回收站」恢复或清空，到期自动清除。
- **驻留与托盘**：所有窗口关闭后应用仍在后台运行；菜单栏图钉图标是驻留态的可见锚点——左键直接唤起**最新笔记**（一条笔记都没有时就地新建一条），右键开托盘菜单（新建笔记 / 显示笔记列表 / 回收站 / 检查更新 / 退出）；「显示笔记列表」打开轻量索引面板，列出全部 live 笔记，点一条即聚焦其窗口并收起，失焦自动收起；「回收站」打开回收站面板，逐条恢复（连 pin 窗口一起回来）或二次确认后清空。

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

应用菜单栏（PinNote）提供 新建笔记 / 删除当前笔记 / 切换主题 兜底项；托盘右键菜单提供 新建笔记 / 显示笔记列表 / 回收站 / 检查更新 / 退出。

## 开发

```bash
# 开发模式（热重载）
go run github.com/egoist/mygo/cmd/mygo@v0.6.3 dev

# 直接构建（裸二进制，可用于手动验证；自更新要求 .app，见下）
go build .

# 生产构建：三平台 .app/.dmg/.deb + 更新清单（需要 mygo.json 配置，见「发版」）
go run github.com/egoist/mygo/cmd/mygo@v0.6.3 build

# 运行测试（Go 单测 + Markdown 往返语料网）
go vet . && go test .
```

## 发版

打 tag 即发版：`.github/workflows/release.yml` 在 `macos-latest` 上串起完整流水线——`go vet` + `go test` →
改写 `mygo.json` 版本号 → 转换 `PINNOTE_UPDATE_KEY` Secret（hex → base64）→ `mygo build -platform darwin/universal` →
上传 `.app` / `.dmg` / `update-darwin-*.json` 到 GitHub Release。

```bash
git tag v0.2.0
git push origin v0.2.0
```

产物（`dist/darwin-universal/`）：`PinNote <版本>.dmg`、`pinnote-<版本>-darwin-universal.tar.gz`
（自更新载荷）、`update-darwin-universal.json`（自更新清单，含 ed25519 签名；feed 在构建期
烧进二进制，与运行架构无关）。

- 手动 `workflow_dispatch` 只构建并上传 workflow 产物，**不发 release**，用来验证流水线。
- tag 与 `mygo.json` 版本不一致时以 tag 为准，CI 在打包前改写 `mygo.json`（仓库里那份不动）。

### 更新签名密钥

自更新只信编译期钉死的 ed25519 公钥（`mygo.json` 的 `updates.publicKey`），签名由 `mygo build` 用
`MYGO_UPDATER_PRIVATE_KEY` 环境变量里的私钥完成。密钥对与 Wails 时代是**同一对**（格式从 hex 换成了 base64）：

```bash
# 查看公钥（应与 mygo.json 一致）
printf %s "<PINNOTE_UPDATE_KEY 的 hex 私钥>" | xxd -r -p | base64 | head -c 0
# 私钥在 GitHub Secret PINNOTE_UPDATE_KEY（hex），CI 负责 hex→base64 转换
```

Secret 缺失时发版流水线**直接失败**：production 构建拒绝任何验不了的产物（ADR-0003 D6 延续）。

- 只 macOS；无 Developer ID 签名与 notarize：从浏览器首次下载 `.dmg` 安装要 **右键 → 打开**。
  之后的升级走应用内自更新，不经过 Gatekeeper。
- 开发构建（版本为 dev）、以及从 `.dmg` 挂载盘直接运行的副本都不允许换自己的 bundle，
  托盘菜单在这些情况下退化为「打开下载页」（ADR-0003 D8）。

## 架构

- `main.go` — 应用入口：应用菜单（含 `RoleEditMenu`，编辑器剪贴板快捷键走这里）、全局快捷键（⌘⌥N → newNote）、启动 purge 过期回收站、恢复最新一条笔记的窗口、mygo 更新卡片插件装载。
- `appui.go` — `pinApp` 共享状态：notes/trash 缓存、quitting 标志、`refreshNotes`（任何笔记变更后主线程刷新所有窗口与面板）。
- `pin_window.go` — pin 窗口（`PinWindow.tsx` + `PinEditor.tsx` 的移植）：无边框置顶跨 Space、头部拖拽区（标题 tooltip）+ 删除（二次确认）/ × 收起、整窗 richtext 编辑器、⌘⌫ 删除、⌘⇧/ 帮助模态、⌘K 链接模态、400ms 防抖自动保存、失焦 flush、空笔记硬删、被删占位态（「该笔记已被删除。」）。
- `markdown_audit.go` — 无损装载 + 保存守卫（`lib/audit.ts` 的 Go 移植）：块级审计（解析器 kind 白名单 → 非空 → 序列化不动点 + 文档相等），不安全的块降级为逐行 raw 段落原样写回；代码块语言经 Tag 保留；`GuardSaveRoundTrip` 拒写任何无法重开的序列化。
- `panel.go` — 托盘面板（`TrayPanel.tsx` 移植）：live 笔记列表（首行标题 + 摘要）、空态新建、点击聚焦收起。
- `trash_panel.go` — 回收站面板：60 天保留区索引（删除时间 + 剩余天数）、逐条恢复（恢复后自动开 pin 窗口）、二次确认后清空回收站。
- `tray.go` — 菜单栏状态项：模板图标/彩色角标、左键唤起最新、右键菜单、更新入口两个菜单项、两个索引面板定位到图标下方。
- `window_service.go` — pin 窗口注册表与主题单一事实源（settings 表 + `mygo.Theme.SetSource` 全窗同帧切换）。
- `db.go` / `note_service.go` — SQLite（`modernc.org/sqlite`，纯 Go 无 CGO）：`notes`/`settings` 两表，数据存于 `~/Library/Application Support/PinNote/pinnote.db`；笔记 CRUD、`deriveTitle`、`DiscardIfEmpty`、回收站恢复/清空/purge；`SetPinned`/`DeleteNoteForever` 为冻结 API（数据兼容保留）。
- `update_service.go` — 自动更新（ADR-0003 的 mygo 移植）：后台只 `mygo.Updater.Check`（45s 首查 + 12h±15min 节奏，命中只亮托盘角标）、手动检查先 `FlushAll` 再开框架卡片、退出时同步 flush + 空笔记清理（编辑器同进程，屏障即一次同步调用）。
- `markdown_audit_test.go` — Markdown 往返语料网（TipTap 时代 51 条语料的移植，按 richtext 契约校准）：安全子集不动点、危险语法逐字节保留、规范化明确断言、空段/尾随段不卡保存、checkbox 写回、raw 块转换。

## 决策记录

- [ADR-0001 移除主面板](docs/adr/0001-remove-main-panel.md)
- [ADR-0002 Markdown 事实源 + WYSIWYG](docs/adr/0002-markdown-source-of-truth-wysiwyg.md)
- [ADR-0003 自动更新与检查更新](docs/adr/0003-auto-update.md)
- [ADR-0004 迁移到 mygo native UI](docs/adr/0004-mygo-native-rewrite.md)
