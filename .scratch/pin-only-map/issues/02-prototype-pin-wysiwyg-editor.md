# 02 — 原型：pin 窗口整窗 WYSIWYG 编辑器

Type: prototype
Status: resolved

## Question

基于 01 的结论，用 `/prototype` 做一个**可运行**的最小编辑器样机（不需要接真实 DB/Wails），验证整窗可编辑渲染形态是否成立：

- 打开即编辑态，输入 `**bold**`、`# `、`- [ ]` 等 Markdown 语法原地即时渲染；
- 渲染内容可继续编辑，序列化回 Markdown 源文本；
- 解析失败的行原样显示、原样写回（不丢数据）；
- 富文本粘贴自动转 Markdown；
- 任务复选框点击回写。

产出：可 react 的原型 + 对 01 推荐的修订（如有）。原型作为资产链接回本 ticket，验证过的序列化逻辑指明是否可直接晋升为正式实现。

Blocked by: 01

## Answer

**结论：模型成立**（GO）。原型在 `prototype/wysiwyg-editor/`（分支 `prototype/tiptap-wysiwyg-editor`，pnpm 独立 workspace，不接 Wails/DB）：`pnpm test` 109 项往返/审计/守卫回归全绿；`pnpm dev` 沙盒在浏览器实测通过「打开即编辑 → 编辑 → 400ms 自动保存 → 事实源回写 → 重开一致」全链路。

对 01 推荐的实测修正（5 条，全部有回归测试钉死）：

1. **审计 = 静态 token 类型门 + 行为不动点双保险**。纯行为审计会漏 `<details>`（标签在 marked 解析时被整个丢掉、不动点照样通过、序列化后内容已变）；纯静态白名单会漏 backtick-相邻 codespan（`` `` x `y` `` `` 序列化不是不动点）。
2. **装载按块 trim**：marked block token 的 `raw` 含尾部空行，逐块 parse 注入幽灵空段落，多块文档结构腐化。
3. **守卫比较前归一化顶层空段落**（TrailingNode 尾段 / `&nbsp;` 噪声段 / 段间孤立空段都不携带内容）：逐结构 eq 会让「连按两次回车」这种最常见编辑永久拒写。不变式保数据，不保空行。
4. **段落序列化转义行首块语法**（`SafeParagraph`）：官方 renderer 不转义 `## ` / `- ` / `1. ` 开头的段落文本，实测重开后段落变标题。
5. **rawSource「转为可编辑」必须重过审计后再插回**：直接 `insertContent(markdown)` 会吞掉注释类内容（实测丢数据）；不安全的源就地保持降级态。

6. **行内图片会被静默丢弃**：schema 无 Image 节点，`![alt](url)` 进 doc 时只剩 alt 文本——
   doc 级相等看不见"进门前就丢"的内容。已从 token 类型门排除 image → 整块降级保数据；
   是否引入 `@tiptap/extension-image` 归 ticket 03/06 产品决策。另验证：选中 rawSource 块后
   粘贴 = 替换该块（正常 ProseMirror 选择语义，非 bug）。

纯 Markdown 文本粘贴（01 §3 的 handlePaste 缺口）已补：`MarkdownPaste` 扩展，启发式识别
且走同一审计装载器插入；复制输出以 `clipboardTextSerializer → getMarkdown` 覆盖。

晋升判定：`src/lib/audit.ts`、`src/lib/rawSource.ts`、`SafeParagraph`（editorConfig.ts）与 `roundtrip.test.ts`+`fixtures.ts` 语料网可直接晋升为正式实现（纯函数 + 扩展配置，无沙盒依赖）；`App.tsx` 沙盒壳丢弃。input rules 原地转换（`# `/`- `/`- [ `）为 StarterKit 自带能力，自动化通道送不进逐键事件，留给 README 人工清单确认。
