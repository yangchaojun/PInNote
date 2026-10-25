# ADR-0004: 迁移到 mygo native UI

日期：2026-10-25
状态：已接受

## 背景

PinNote 原先基于 Wails v3 + React 19 + TipTap v3：每条笔记一个无边框 webview 窗口，
编辑器在渲染进程里，Go 侧只提供服务。这带来三块持续成本：

1. **两条技术栈**：前端构建链（Vite/TS/vitest）与 Go 后端并行维护，绑定生成、
   事件桥（`notes:changed`、`pin:flush-requested` 等）都是胶水。
2. **跨进程信任协议**：更新屏障要靠广播 + 回执（ADR-0003 D7）——每个 webview
   必须自己证明「缓冲已落库」，因为后端看不到渲染进程的编辑器状态。
3. **无损往返约束被锁死在 TipTap**：`lib/audit.ts` 的守卫语义与 TipTap 的
   schema/serializer 深度耦合。

[mygo](https://mygo.egoist.dev/) 的 native UI 模式（纯 Go 声明式 UI，GPU 绘制，
无 webview）在 2026-10 已具备本项目所需的全部原语：`ui/richtext` 富文本编辑器
（Markdown 导入导出、粗斜体/链接/任务列表、图片附件）、无边框/置顶/跨 Space 窗口、
模板托盘图标、全局快捷键、以及带 ed25519 验签的 `mygo.Updater` 与现成更新卡片。

## 决策

用 mygo v0.6.3 的 native UI 模式完全重写界面层，功能保持不变：

- **后端不动**：`db.go`、`note_service.go` 的 CRUD/标题派生/空笔记硬删/回收站
  purge 语义原样保留（同一数据库、同一 schema）。
- **编辑器**：TipTap → `ui/richtext`。整窗编辑、`# `/`- `/`- [ ] `/`**粗体**`
  原地即时渲染、"记点什么…" 占位、⌘K 链接弹窗全部对应保留。
- **无损往返**：`lib/audit.ts` 移植为 `markdown_audit.go`，守卫语义不变
  （serialize→重开→文档相等，否则拒写），语料网随迁并按新引擎契约校准（见下）。
- **窗口/托盘/面板/菜单/快捷键**：一一对应移植；托盘右键「临时弹出菜单」沿用
  mygo 官方 menubar 模式（macOS 上常驻 Menu 会吞掉左键）。
- **更新**：GitHub Releases + 编译期钉死公钥 + 框架卡片的原则不变（ADR-0003），
  换成 mygo 的实现：`mygo build` 签名、`update-darwin-*.json` 清单、native 更新
  卡片插件；密钥对沿用 Wails 时代那对（hex→base64 转换，GitHub Secret 不换）。

### 实测行为差异（有意为之，均为数据安全方向的收紧或持平）

| 构造 | TipTap 时代 | mygo richtext 时代 |
|---|---|---|
| 内联图片 `![alt](url)` | 降级 rawSource（schema 无图片） | **原生附件，可编辑**（URL 往返） |
| 表格 | 安全（可编辑） | 降级 raw 块（引擎把表格压平为制表符行，编辑必失真） |
| 主题分割线 `---` | 安全 | 降级 raw 块（解析为空段，序列化会静默丢） |
| 代码块语言 ` ```js ` | 安全 | 仍可编辑；语言经 run Tag 保留并在写回时还原围栏 |
| 链接定义 `[r]: url` | 降级 | 降级（解析器吞块，靠源码缺口检测找回） |
| footnote 引用 `[^1]` | 规范化（转义） | 规范化（转义，跨渲染器语义漂移不变） |

全部由 `markdown_audit_test.go` 语料网锁定；「零静默丢失」的合同未变——凡引擎
不能无损表达的构造，一律以源码原样保留。

### 简化：更新屏障坍缩为一次同步调用

编辑器与守卫都在本进程内，ADR-0003 D7 的广播/回执协议（token、逐窗 ack、
dirty 否决）不再必要：`WindowService.FlushAll()` 在主线程同步序列化并写库，
`OnWillQuit` 与手动检查更新各调用一次。任何重启路径（含更新卡片的
Restart & Apply）都无法越过它——比原协议更强。

### 弃用

- `update.channel`（beta 通道）：mygo 的 GitHub feed 只看 latest release，
  无 prerelease 通道。该设置从无 UI，仅文档化弃用。
- `tools/sign-release`：keygen/sign/verify 由 `mygo keygen` / `mygo build`
  承担；仓库只保留公钥（`mygo.json`）。
- Wails 的 `build/` 模板、`frontend/`、`Taskfile.yml`：整体删除。

## 后果

- 单二进制、单语言；无前端构建，无绑定生成，无 IPC 胶水。
- 「降级块」的呈现从 TipTap 节点视图换成「光标落入 raw 块时底部出现
  『转为可编辑』操作行」——仍是逐块审计、拒转即保持源码。
- 托盘「检查更新」不再有 ToolTip（mygo MenuItem 无该 setter）；上次检查
  时间仍持久化在 settings 表，标签与角标行为不变。
- 发版产物从 `.zip + .sig + SHA256SUMS` 换为 mygo 的清单模型
  （`update-darwin-*.json` + 差量包）；已安装的 Wails 版无法就地迁移到
  mygo 更新通道——最后一个 Wails 版之后需手动装一次 `.dmg`。
