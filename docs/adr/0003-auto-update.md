# ADR-0003: 自动更新与检查更新（GitHub Releases + Wails updater）

日期：2026-09-08 · 状态：**已接受** · 决策者：用户

三个开放问题的结论：**Q1 不上 Developer ID**（继续 ad-hoc 签名，自动更新定位为自用/内测通道，但 ed25519 更新签名升为强制项）· **Q2 采纳 universal 单产物** · **Q3 复用框架内置更新卡片**（不自建 `/#/update` 窗口）。

## 背景

### 已有能力（不必重造）

依赖 `github.com/wailsapp/wails/v3 v3.0.0-beta.16`，其 `pkg/updater` 是一套完整的自更新框架：

| 能力 | 位置 | 说明 |
|---|---|---|
| Provider 抽象 | `pkg/updater/types.go:107` | 内置 `github` / `appcast` / `keygen` / `endpoint` 四种源 |
| 检查 + 版本比较 | `pkg/updater/updater.go:201` | `Check(ctx)`，semver 比较容忍 `v` 前缀 |
| 下载 + 流式摘要 | `pkg/updater/download.go` | 边下边算 sha256/sha512，进度事件 ~10/s |
| 校验 | `pkg/updater/verify.go:69` | `Digest` 与 `Signature` 双轨；`ed25519` / `ed25519ph` / `ecdsa-p256`；无公钥时带签名 fail-closed |
| 解包 | `pkg/updater/extract.go:66` | zip/tar.gz → **要求恰好一个顶层条目**、防 zip-slip |
| 原子换包 + 重启 | `pkg/updater/helper.go:88` | fork 自身为 helper（sentinel env），等父进程退出 → `.bak` 备份 → rename 换包 → `open -n <bundle>` |
| 事件总线 | `pkg/updater/events.go` | `wails:updater:*`，Go 与 JS 同源订阅 |
| 内置更新窗口 | `pkg/updater/assets/window.html` | `CheckAndInstall` 会自动开；可用 `WindowNone` / `BuiltinWindow` / `BYOWindow` 三选一 |

`app.Updater` 已在应用层暴露（`pkg/application/application.go:433`、`application.go:597`）。

### 现状事实

- 发布链路已存在：`.github/workflows/release.yml` 打 tag `v*` → `wails3 package` → zip + dmg + `SHA256SUMS.txt` → GitHub Release（`yangchaojun/PInNote`）。
- 签名 = **ad-hoc**（`build/darwin/Taskfile.yml` 的 `codesign:adhoc`），无 Developer ID、无 notarize。
- 产物 = **单架构 arm64**（`macos-latest` runner 上 `uname -m`）。
- 版本号只存在于 `build/darwin/Info.plist`（`0.1.0`），**Go 侧没有任何版本常量**；CI 用 PlistBuddy 按 tag 覆写 plist，二进制本身不知道自己是什么版本。
- 从未打过 tag（`git tag` 为空）→ 线上一条 release 都没有，检查逻辑当前无从命中。
- 产品形态 pin-only：无常驻主界面，托盘是唯一鼠标侧锚点（`tray.go`）。数据在 `~/Library/Application Support/PinNote/pinnote.db`，与 bundle 同 lifecycling 无关，更新不丢数据。
- 落库靠前端异步 `flush()` + `beforeunload` 兜底（`frontend/src/PinEditor.tsx:133`）。

## 四个阻塞项（不修就不能开自更新）

### B1 · 现有 zip 结构更新器解不开（实测）

`release.yml` 用 `ditto -c -k --sequesterRsrc "bin/${APP_NAME}.app" "dist/${base}.zip"`。实测其顶层条目是：

```
Contents
__MACOSX
```

而 `maybeExtractInto`（`pkg/updater/extract.go:56`）硬要求**恰好一个顶层条目**，且 helper 需要一个 `.app` 目录才能替换 `/Applications/PinNote.app`。当前 zip 两条都不满足 → `DownloadAndInstall` 在 install 阶段直接失败。

修法：改成 `ditto -c -k --keepParent`（顶层唯一 `PinNote.app`，实测通过），并在 CI 加断言：

```bash
test "$(unzip -Z1 "dist/${base}.zip" | cut -d/ -f1 | sort -u | wc -l | tr -d ' ')" = 1
```

### B2 · `CFBundleExecutable` 与实际二进制不一致 → 更新后可能起不来

`build/darwin/Info.plist` 写的是 `pin-note`，而 `create:app:bundle` 拷入的是 `bin/PinNote`；`bin/PinNote.app/Contents/MacOS/` 下现在**同时躺着 `pin-note` 和 `PinNote` 两个文件**。换包后 LaunchServices 按 `CFBundleExecutable` 找入口——新 bundle 里若只有其中一个，就是「更新完成，双击打不开」。

修法：确定唯一可执行名（建议 `PinNote`，与 `APP_NAME` 一致），改 plist，删掉 stale 副本，CI 增加 `test -x bin/PinNote.app/Contents/MacOS/$(PlistBuddy -c 'Print :CFBundleExecutable' ...)`。

### B3 · Go 侧没有版本号，`Config.CurrentVersion` 无处可取

单一事实源改为**构建期注入**：新增 `version.go` 的 `var Version = "dev"`，`-ldflags "-X main.Version=${number}"`；CI 里同一个 `${number}` 既喂 ldflags 又喂 PlistBuddy，二者不可能漂移。前端通过 binding 读 `Version`（关于面板、更新窗口都要用）。

### B4 · 只有 arm64 产物，amd64 用户检查必报错

`DefaultAssetMatcher`（`providers/github/github.go:395`）按 `platform` + `arch` 子串匹配 asset 名，找不到就返回 `-1` → `Check` 抛 `release <tag> has no asset for darwin/amd64`。

修法（推荐）：CI 改用已存在的 `wails3 task darwin:package:universal`，asset 名 `PinNote-<ver>-macOS-universal.zip`，并配一个自定义 `AssetMatcher`（显式认 `universal`，其次认本机 arch，绝不回落 `.dmg`）。这样一条 release 一个主 asset，匹配逻辑最短。

## 决策

1. **更新源 = GitHub Releases**，复用 `providers/github`。不引入自建服务器、不做 Sparkle/appcast。（Repository 串保持远端原样大小写 `yangchaojun/PInNote`。）
2. **不使用 `Config.CheckInterval`**（内置周期循环走 `CheckAndInstall`：`updater.go:140` → 自动开卡片 → 下载完自动 `Restart` 掉用户正在写的笔记，见 D7）。改为**自有调度**：启动后 45s 首检，之后每 12h ± 0–15min jitter，加一条手动「检查更新」。后台那条路径只调 `Check()`——它不开窗口（窗口仅在 `CheckAndInstall` 的 `openSession` 里创建），只有用户主动点菜单项才走 `CheckAndInstall()` 弹卡片。
3. **命中新版本只提示，不自动下载**。发现 → 托盘角标 + 菜单项变化；下载由用户在卡片里按「Install」开始（`wails:updater:user:install` → `DownloadAndInstall`）。
4. **UI = 复用框架内置更新卡片**：`Config.Window = &updater.BuiltinWindow{CSS: pinnoteCardCSS, Options: updater.WindowOptions{AlwaysOnTop: true}}`。pin-only 形态没有可挂横幅的主界面，托盘仍是发现入口（角标 + tooltip + 菜单项「检查更新…」/「更新到 vX.Y.Z…」）；卡片自带 检查中/已最新/有新版(release notes)/进度/待重启/出错 六个态与 Skip、Remind later、Cancel、Install、Restart & Apply 全套按钮，省掉一整个前端窗口 + 路由 + 状态机。
   - CSS 覆写对齐 PinNote 视觉（`#6f8ffa` 强调色、8px 圆角、13px 字号、跟随 `prefers-color-scheme`）。中文文案若模板里是硬编码字符串，走 `BuiltinWindow.HTML` 整页替换（模板在 `pkg/updater/assets/window.html`，可复制后改文案）；此项列为 P2 可选打磨，不阻塞功能。
   - **关键约束**：卡片的「Restart & Apply」发出 `wails:updater:user:restart`，updater 自己 `go u.Restart(ctx)` → `host.Quit()`（`window_lifecycle.go:85-87`），**Go 侧无法否决这次 Quit** → 屏障必须提前完成（D7）。反向通道可用：我们从 Go `app.Event.Emit(updater.EventUserInstall/EventUserSkip/EventUserCancel)` 驱动卡片，守卫不过时直接把卡片关掉。
5. **信任链 = sha256 摘要 + 编译期钉死的 ed25519 公钥**。
   - 摘要：`github.Config.ChecksumAsset = "SHA256SUMS.txt"`（`parseChecksumLine` 兼容 `sha256sum` 行格式）。
   - 签名：额外发布 `<zip>.sig`，**签的是 sha256 摘要字节而非文件**（`verify.go:112-120` 是 `ed25519.Verify(pub, digest, sig)`）。CI 用一个 ~50 行的 `tools/sign-release` 生成，私钥进 GitHub Secrets，公钥以常量编进 `Config.PublicKey`。
   - `providers/github` 不填 `Signature`，因此需要一层薄 Provider 包装：委托 `github.Provider.Check`，再按 `Release.Artifact.Filename + ".sig"` 取回签名塞进 `Release.Verification`。命名带 `.sig` 天然被 `DefaultAssetMatcher` 的 sidecar 规则跳过，不会被当成主产物。
   - 只有摘要不足以自辩：`SHA256SUMS.txt` 与 zip 同在一个 release，能改写 release 就能改写校验文件。摘要防传输错误，签名防源被投毒。
6. **生产构建要求签名存在**：`-tags production` 下 `requireSignature = true`，缺 `.sig` 的 release 一律拒绝 `DownloadAndInstall`，退化为「打开下载页」。宁可没有自动更新，也不要一条看着安全、实际同源可伪造的通道。
   - Q1 定了不做 Developer ID 之后，这条从「加分项」变成**唯一信任根**：更新包既不经 notarize、也不被 Gatekeeper 判定，全链路只有这把编译期公钥。因此 ed25519 签名与 P2 同期落地，不允许先上「仅摘要」的中间态。
7. **主动式 flush 屏障（PinNote 特有的最高风险项）**：`Restart()` 派生 helper 后立刻 `host.Quit()`，helper 只等父进程 30s（`helper.go:105-115`）。而落库是前端 `beforeunload` 里的**异步** `flush()`，进程被终止时不保证写完 → 自更新重启可能吃掉未落笔的编辑。因为卡片的重启按钮不可拦截（D4），屏障不能挂在「用户点重启」之后，必须抢跑：
   - `WindowService.RequestFlushAll()`：向所有 `pin-*` 窗口 `EmitEvent("pin:flush-requested")`；
   - 每个窗口 `await editor.flush()` → 空笔记 `DiscardIfEmpty` → 回 `pin:flushed`（payload 带 noteID + dirty 标志，沿用 `pin:delete-requested` 的「广播 + 自过滤」模式）；
   - 触发点：`updater.EventInstalling` 抢跑一次，`updater.EventUpdateReady` 再确认一次；Go 侧记录 `clean` / `dirty`；
   - `dirty`（任一口报脏，或 3s 内未收齐全部 ack）→ 从 Go 发 `updater.EventUserCancel` 关掉卡片 + 托盘 tooltip 提示「有笔记未能保存，请稍后再试」，**本轮更新作废**，用户需重新点「检查更新」；
   - `clean` → 什么都不做，等用户按 Restart & Apply；此时 Quit 是安全的，因为编辑器缓冲区里已经没有东西可丢。
   - 兜底不变：`beforeunload` 的 flush 仍保留，它覆盖 ⌘Q 与崩溃路径。
8. **自更新前置守卫**（在打开卡片之前判定），任一不满足则菜单项退化为「打开下载页（.dmg）」并说明原因：
   - `os.Executable()` 必须落在某个 `.app` 内（否则 `bundleTarget()` 返回裸二进制，`wails3 build` 的开发二进制会覆盖仓库产物）；
   - 版本为 `dev`、或 bundle 位于仓库工作区（含 `PinNote.dev.app`）→ 禁自更新；
   - bundle 父目录必须可写（在挂载的 DMG `/Volumes/…` 里跑 = 不可自更新）；
   - `/Applications` 之外的路径允许（helper 用 `rename`，不需要管理员），但首次成功后提示「建议移到 /Applications」。
9. **跳过/稍后提醒自己持久化**：`Updater.SkipVersion`（`window_lifecycle.go:165`）只在内存，重启即失。写进 `settings` 的 `update.skippedVersion`，`Init` 之后回填调用；`update.lastCheckedAt`、`update.remindedAt` 同表存放，同版本 24h 内不重复弹提示。
10. **频道**：stable 走 `releases/latest`（天然排除 prerelease/draft）；内测走 `Prerelease: true` + `v0.x.0-beta.n` tag，由 `settings.update.channel` 决定，**只在 `Init` 时读取一次**（运行时热切频道会造成半路换轨的状态错乱）。
11. **失败与网络语义**：后台检查失败一律静默（只 `log.Printf` + 保留上次状态），不打扰；手动检查才把错误分四类呈现——网络不可达 / 无本平台 asset / 校验失败 / 目标不可写。分类靠 `ErrorInfo.Stage` + 包装错误前缀。

## 状态机与事件映射

Go 侧订阅 `wails:updater:*`，维护一个内部状态机（`UpdateService.Status()` 暴露），**只服务托盘角标与菜单文案**；渲染交给框架卡片，不再有前端 reducer：

| `wails:updater:*` | 自有状态 | 附带 |
|---|---|---|
| `check-started` | `checking` | |
| `no-update` | `up-to-date` | `currentVersion` |
| `update-available` | `available` | `Release{version,notes,publishedAt,size}` |
| `download-started` / `download-progress` | `downloading` | `Progress{written,total,rate}` |
| `verifying` | `verifying` | |
| `installing` | `staging` | |
| `update-ready` | `ready`（等用户点重启） | |
| `error` | `error` | `ErrorInfo{stage,message}` |

托盘侧的映射最短：`available` → 亮角标 + 菜单文案换成「更新到 vX.Y.Z…」；`up-to-date` / `error` / 任一中间态 → 收角标（手动检查时 error 才提示）。

`ready` 之后**不自动重启**：Restart & Apply 由用户在卡片上按；屏障是抢跑式的（D7），按下那一刻缓冲区已经干净。若抢跑判定为 `dirty`，我们从 Go 发 `updater.EventUserCancel` 关掉卡片，本轮更新作废。

## 代码落点

| 文件 | 动作 | 内容 |
|---|---|---|
| `version.go` | 新增 | `var Version = "dev"`，ldflags 注入 |
| `update_service.go` | 新增 | Wails service：`Status` / `CheckNow` / `StartDownload` / `ApplyAndRestart` / `SkipVersion` / `RemindMeLater` / `OpenDownloadPage`；自有定时器与守卫；把 updater 事件翻译成 `update:state` |
| `update_provider.go` | 新增 | 包装 `github.Provider`，补 `Verification.Signature` |
| `update_verify_prod.go` / `update_verify_dev.go` | 新增 | build-tag 控制 `requireSignature` |
| `main.go` | 修改 | `app.Updater.Init`（CurrentVersion/Provider/PublicKey/BuiltinWindow+CSS）+ 回填 `skippedVersion` + 注册 service |
| `build/darwin/Info.plist` | 修改 | `CFBundleExecutable` → `PinNote`（B2），版本仍由 CI 注入 |
| `build/darwin/Taskfile.yml` | 修改 | 打包时清除 bundle 里 stale 的 `MacOS/pin-note`（B2） |
| `tray.go` | 修改 | 新版本角标/tooltip，菜单加「检查更新…」「更新到 vX.Y.Z…」 |
| `window_service.go` | 修改 | `RequestFlushAll()` + ack 收集与 `clean`/`dirty` 判定（不再新增窗口） |
| `frontend/src/PinWindow.tsx` | 修改 | 订阅 `pin:flush-requested`，串起 flush → DiscardIfEmpty → ack |
| `tools/sign-release/` | 新增 | CI 用的签名小程序 + 一次性 `keygen` 子命令 |
| `.github/workflows/release.yml` | 修改 | B1–B4 全部修法 + `.sig` 生成 + release body 改写 |
| `docs/adr/0003-auto-update.md` | 新增 | 本文 |

改完 Go service 后需 `wails3 generate bindings`（产物 `frontend/bindings/pinnote/updateservice.ts`）。

## 分期

- **P0 前置修复**：B1–B4 + 首次打 tag 发版。没有它，后面全是空中楼阁。
- **P1 只读**：检查更新 + 托盘提示 + 「打开下载页」。零副作用，先解决「不知道有新版」，可在 P2 之前独立上线。
- **P2 写入**：下载 / 校验 / 换包 / 重启 + flush 屏障 + 跳过与提醒持久化 + 守卫与错误分类。
- **P3 打磨**：beta 频道、进度速率显示、更新完成后首启的 changelog 高亮、asset 命中率观测。

## 测试

- Go 单测：包装 Provider 是否正确填 `Verification`；守卫各分支（路径/可写性/`dev` 版本）；flush 屏障的收齐、超时、dirty 三分支（用假 `appHandle` + 计数 ack）；`skippedVersion` 回填后 `Check` 返回 up-to-date。
- 前端 vitest：`PinWindow.tsx` 收到 `pin:flush-requested` 后的 flush→ack 顺序与 dirty 上报（沿用 `PinWindow.test.tsx` 的事件顺序断言写法）。框架卡片不测——它不是我们的代码。
- 端到端手测（唯一能证明「不会变砖」的一步）：私有测试 repo 发相邻两个 tag → 装到 `~/Applications` 旧版 → 走一次真实升级 → 校验新 bundle 的 `codesign --verify`、`open -n` 起得来、笔记库与 pin 窗口几何/主题原样、`⌘⌥N` 仍在。
- CI 追加断言：zip 顶层唯一条目；`CFBundleExecutable` 可执行文件存在；本地用公钥验一遍刚生成的 `.sig`。

## 影响与未覆盖

- 引入 `tools/sign-release`（Go，标准库 `crypto/ed25519`，**不新增第三方依赖**）。
- universal 产物使包体与 CI 时间约翻倍。
- ad-hoc 签名下每次构建的 code signing identity 都不同：跨更新的 TCC / 钥匙串授权可能被判定为「不同应用」而重问。当前全局快捷键走 Carbon `RegisterEventHotKey`（`pkg/application/global_shortcut_darwin.go:46`，源码注释明确它不需要辅助功能授权），影响面小；但一旦引入任何需要 TCC 授权的能力，就必须先有 Developer ID 稳定身份 —— 见 Q1。
- 自更新走 Go HTTP 下载，产物**不带 quarantine xattr**，因此换包后首次启动不会触发 Gatekeeper。这等于「更新路径绕过了 notarize」：正因如此，D5/D6 的签名与 fail-closed 门禁不是可选项。
- 明确不做：Windows/Linux（`release.yml` 已声明出界）；崩溃后自动回滚（helper 有 `.bak` 但无首启看门狗）；差分更新（全量 zip ~13MB，够了）；灰度/统计（无服务端）。

## 已定

- **Q1 不投入 Developer ID**（$99/年）。后果：① 首次从浏览器下载 `.dmg` 仍需右键 → 打开绕过 Gatekeeper，release 说明保留这段指引；② 自更新路径因下载不产生 quarantine xattr 而不受影响，但等于绕开 notarize → ed25519 签名从可选变强制；③ 每次构建 ad-hoc 身份都在变，将来若引入任何需要 TCC 授权的能力，必须先补 Developer ID。
- **Q2 采纳 universal 单产物**：CI 用 `wails3 task darwin:package:universal`，asset 名 `PinNote-<ver>-macOS-universal.zip`，配自定义 `AssetMatcher`（只认 `universal`，永不回落 `.dmg`）。代价是包体与 CI 时间约翻倍。
- **Q3 复用内置卡片**：删掉自建 `/#/update` 窗口方案，改为 `BuiltinWindow{CSS}`。省下约 1–2 天前端工作量，代价是重启按钮不可拦截（屏障改主动式，见 D7）、文案与视觉只能靠 CSS/HTML 覆写贴合。

## 实施记录（2026-09-08 · `codex/auto-update`）

P0–P2 已落地。以下是实现过程中被证伪或需要修正的地方，以代码为准。

- **B1 的真凶不是 `--keepParent`，而是 `--sequesterRsrc`。** 仓库里的 `release.yml` 当时已经带了
  `--keepParent`；实测 `ditto -c -k --keepParent --sequesterRsrc Foo.app` 的顶层是
  `Foo.app` + `__MACOSX` **两条**，仍然被 `maybeExtractInto` 的「恰好一个顶层条目」拒掉。
  去掉 `--sequesterRsrc` 后顶层唯一。CI 断言同时检查「顶层唯一」与「等于 `PinNote.app`」。
- **D3 需要修正：手动检查会立即下载。** `CheckAndInstall` 是 `openSession → Check → DownloadAndInstall`，
  中间**不等**用户按 Install，所以「命中新版本只提示、下载由用户开始」只在后台路径成立。
  两个后果：① 强制验签的门禁只能放进包装 Provider 的 `Check`（否则 `requireSignature` 无从生效）；
  ② `DownloadAndInstall` 只往临时目录落地并解包，破坏性动作只有 `Restart`，因此提前下载是无害的，
  真正的同意点就是卡片上的 Restart and Apply。
- **D7 屏障按「抢跑」实现**，触发点 `EventInstalling` + `EventUpdateReady`，不干净就 `Event.Emit(EventUserCancel)`
  关卡片并作废本轮。**残留风险**：`ready` 之后用户继续敲字再按 Restart —— 此时屏障早已通过。
  兜底是既有的 blur-flush（卡片抢焦点即触发）+ 500ms 防抖 + `beforeunload`；未做二次屏障，
  因为卡片的 Restart 无法从 Go 侧否决。
- **`update.remindedAt` 取消。** 提示只有托盘角标与菜单文案，二者常驻、不重复打扰，
  没有需要节流的东西。`update.skippedVersion`、`update.lastCheckedAt` 保留。
- **`UpdateService` 不是 Wails service**（`app.Updater` 的 UI 由框架卡片承担，渲染层无人调用它），
  因此本特性**不需要** `wails3 generate bindings`，`frontend/bindings` 不变。
- `signedReleases` 携带 `requireSignature` 字段而非直接读常量，为的是让「生产拒绝无签名 release」
  这一分支能在非 production 测试二进制里被跑到。
- `updatePublicKeyHex` 默认为空，因此**在跑过一次 keygen 并提交公钥之前，发版流水线会在
  Sign release artifacts 步骤失败**。这是刻意的失败：没有信任根就没有自动更新。
- 版本注入走 `build/darwin/Taskfile.yml` 的全局 `PINNOTE_VERSION`（env 或 CLI 皆可），
  实测 `-ldflags="-w -s -X main.Version=0.2.0"`；未设置时为 `dev`，守卫即禁自更新。

### 已验证 / 未验证

| 项 | 状态 |
|---|---|
| zip 顶层唯一（`ditto` 实测） | ✅ 本地实测 |
| universal 打包 + `Contents/MacOS` 唯一入口 | ✅ 本地 `wails3 task darwin:package:universal` |
| 守卫各分支、matcher、`.sig` 拉取与生产拒绝、flush 屏障三分支 | ✅ Go 单测 |
| 卡片 → `pin:flush-requested` → ack 顺序 | ✅ vitest（137 项全绿） |
| keygen → sign → verify（错钥必须失败） | ✅ 本地跑通 |
| 真实升级（旧版 → 新版换包后仍能启动、笔记与窗口几何不变） | ❌ 需要先发第一个 tag |
