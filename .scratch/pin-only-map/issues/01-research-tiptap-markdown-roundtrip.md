# 01 — TipTap × Markdown 往返能力调研

Type: research
Status: resolved

## Question

TipTap（ProseMirror）能否支撑 pin 窗口的"整窗可编辑渲染 + Markdown 事实源"模型？具体要回答：

1. 往返覆盖：PinNote 现有 GFM 子集（`frontend/src/lib/markdown.ts` / `MarkdownView.tsx` 所渲染的：标题、有序/无序列表、任务复选框 `- [ ]`、粗体/斜体/删除线、行内代码、代码块、链接、引用、表格）用 TipTap 扩展 + Markdown 序列化（如 `@tiptap/markdown` 或社区方案）是否可无损往返？各语法的坑点？
2. 不丢数据策略：**解析失败的原始 Markdown 行必须原样保留并写回**。用什么机制（placeholder node / 保留原 source 段 / diff 合并）最可靠？
3. 富文本粘贴 → Markdown：TipTap 的 paste 处理链能否做到（clipboard HTML → ProseMirror doc → markdown 序列化）？
4. 复选框点击回写：任务列表切换直接改 doc 再序列化回 `note.content`，路径是否成立？
5. 包体/许可/维护状态结论，以及推荐的具体扩展清单与序列化方案。

结论落盘为 `docs/research/tiptap-markdown-roundtrip.md`（research 分支），并回填本 ticket 的 Answer。

Blocked by: (无)

## Answer

**结论：GO**（TipTap v3 + 官方 `@tiptap/markdown`）。完整报告在 `research/tiptap-markdown-roundtrip` 分支（commit `35e2d4d`）的 `docs/research/tiptap-markdown-roundtrip.md`；TipTap 仓库读取于 v3.30.5（2026-08-28）。

要点：

1. **往返覆盖：完整**。全部所需语法（标题、有序/无序列表、任务复选框、粗/斜/删、行内码、围栏代码块、链接、引用、GFM 表格）在官方 v3 扩展中都成对提供 `parseMarkdown`/`renderMarkdown`，含任务列表/表格的自定义 marked tokenizer。坑点：序列化是**规范化重写**而非逐字保留（反斜杠转义、`&`/`<`/`>` 实体化、分隔符归一）；混合任务+普通列表会被拆成两个列表；连续空段落产生 `&nbsp;` 行；StarterKit 的 Underline 序列化成非 GFM 的 `++text++`（必须关闭）；官方承认 Markdown 支持是 early release，注释可能被丢弃。
2. **不丢数据策略**：逐行 diff/merge 不可行（ProseMirror 不保留源位置 + 规范化重写）。推荐 **`rawSource` atom 块节点**：`renderMarkdown` 原样返回 `source` 属性，由块级 parse 审计填充（marked token 自带逐字 `raw`；自定义 tokenizer 是官方扩展点，有 taskList/table 先例与 `createBlockMarkdownSpec`/`createAtomBlockMarkdownSpec` 公开辅助）。再加保存时不变式守卫：`parse(serialize(doc)) ≡ doc`，不匹配拒绝写回。
3. **富文本粘贴 → Markdown**：走原生管线（HTML → `parseDOM` → doc → `getMarkdown()`）即可；纯 Markdown 文本粘贴**不自动**转换，需加 `handlePaste` 调 `insertContent(text, {contentType:'markdown'})`；复制输出用 `clipboardTextSerializer` 覆盖为 `getMarkdown()`。
4. **复选框回写**：完全内置——TaskItem NodeView 勾选触发 `setNodeMarkup`，`onUpdate → getMarkdown()` 写回 `- [x]`；可替换现有 `toggleCheckboxAtLine` 行号 hack。
5. **选型栈**：TipTap v3（core 3.30.5，v2 已停更于 2.27.2）+ 官方 `@tiptap/markdown@3.30.5`（MIT，v3.7.0 起）；社区 `tiptap-markdown@0.9` 已被作者官方弃用，勿用。扩展清单：StarterKit（关 Underline）+ TaskList/TaskItem + Table + Markdown + 自定义 rawSource。体积 ≈ 105KB gz，本地应用无碍。

GO 的三条件：① rawSource 直通节点落地；② 接受规范化重写（配 fixture 往返测试）；③ 保存时不变式守卫。后备阶梯（若 early-release 边缘问题爆发）：降级为 raw 块 → 源码/预览双态 → 社区包。

对 ticket 02 的输入：原型必须覆盖 rawSource 节点 + 规范化 fixture 测试 + `⌘B/⌘I/⌘K` 与 Markdown 文本粘贴的 `handlePaste`。
