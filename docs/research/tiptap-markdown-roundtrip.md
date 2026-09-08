# TipTap × Markdown 往返能力调研（wayfinder ticket 01）

Date: 2026-08-31 · Method: primary sources only — TipTap repo source (read at `d02646bdaa6a1cdf88a450a67fb2596abf5305d9`, 2026-08-28), npm registry metadata (2026-08-31), official docs (tiptap.dev, prosemirror.net), CommonMark/GFM specs.

Scope reminder: pin 窗口整窗为 TipTap（ProseMirror）WYSIWYG 编辑器，`note.content`（SQLite）仍是 Markdown 事实源。需支撑的 GFM 子集由现有渲染器定义：`frontend/src/components/MarkdownView.tsx`（react-markdown + remark-gfm）与 `frontend/src/lib/markdown.ts`（工具栏产生的语法：`#` 标题、`-`/`n.` 列表、`- [ ]` 任务项、`**`/`*`/`~~`、行内代码、围栏代码块、链接、`>` 引用、GFM 表格）。

## Verdict: **GO**（附条件）

TipTap v3 + 官方 `@tiptap/markdown` 可以覆盖全部所需子集，往返为 **AST 级无损、字节级规范化重写**。三个附条件：

1. 必须实现一个“原始内容直通节点”（见 Q2），否则注释、链接定义等无法映射的块会被**静默丢弃**；
2. 必须接受（或关闭）每次保存对 Markdown 源的规范化重写（转义、定界符统一），否则需按块 diff；
3. 官方 markdown 包自述 **early release**，需以回归测试钉死子集往返（见 Q1 测试建议）。

---

## 0. 生态版本现状（截至 2026-08，npm registry）

| 事实 | 值 | 来源 |
|---|---|---|
| TipTap 当前大版本 | **v3 是 latest**：`@tiptap/core@3.30.5`（2026-08-26 发布）；v2 仍被维护但已非主线：dist-tag `v2-latest=2.27.2` | npm registry `dist-tags` |
| 官方 Markdown 包 | **`@tiptap/markdown@3.30.5`**，MIT，首个版本 3.7.0，发布于 2025-10-14，随 monorepo 持续发版 | npm registry；repo `packages/markdown` |
| 社区 `tiptap-markdown` | 0.9.0（2025-09-08），MIT，peer `@tiptap/core ^3.0.1`；**作者已宣布弃用**：“Tiptap released a markdown extension in 3.7.0, please prefer using the official extension… I don't plan to address current issues / PR” | npm registry + [aguingand/tiptap-markdown README](https://github.com/aguingand/tiptap-markdown) |

结论：序列化方案直接选**官方 `@tiptap/markdown`**，不引入社区包。若未来因 early-release 缺陷需要退路，v2/v3 均可换回 `tiptap-markdown`（API 面相似：`Markdown` 扩展 + `getMarkdown()`），但其维护性更差（单人、已弃维护）。

## 1. 往返覆盖：子集逐项核验

全部语法在官方包 + 官方扩展中**成对实现了 `parseMarkdown` / `renderMarkdown`**（源码逐一核对；文件路径相对 `ueberdosis/tiptap@3.30.5`）：

| 语法 | 解析 | 序列化 | 源码位置 |
|---|---|---|---|
| 标题 `#…######` | marked `heading` token | `'#'.repeat(level)`（ATX） | `packages/extension-heading/src/heading.ts` |
| 无序列表 | marked `list`（+ 官方自写 ordered-list 块 tokenizer） | 固定 `- ` 前缀 | `packages/extension-list/src/item/list-item.ts` |
| 有序列表（含 `start` 值、`type` 标记） | 自定义 `markdownTokenizer`（块级 marked 扩展，保留 token.raw） | 编号由 `start + index` 生成 | `packages/extension-list/src/ordered-list/ordered-list.ts` |
| 任务项 `- [ ] / - [x]` | **自定义块 tokenizer**（marked 核心不支持 GFM task list，官方为此手写，支持嵌套/混合内容回退） | `` `- [${'x'|' '}] ` `` | `packages/extension-list/src/task-list/task-list.ts`、`task-item/task-item.ts` |
| 粗体 / 斜体 / 删除线 | marked `strong`/`em`/`del` token → mark | `**` / `*` / `~~` | `packages/extension-bold/src/bold.tsx`、`italic.ts`、`strike.ts` |
| 行内代码 | marked `codespan` | `` ` `` 包裹；code 上下文内跳过转义/实体编码 | `packages/extension-code/src/code.ts`、`MarkdownManager.encodeTextForMarkdown` |
| 围栏代码块 | marked `code`（含 lang） | ```` ```lang ```` 围栏 | `packages/extension-code-block/src/code-block.ts` |
| 链接 | marked `link` | `[text](href "title")` | `packages/extension-link/src/link.ts` |
| 引用 | marked `blockquote` | 单层 `>` 前缀递归 | `packages/extension-blockquote/src/blockquote.tsx` |
| GFM 表格（含列对齐） | marked `table` + 官方 tokenizer 预处理“单元格内反引号中的竖线” | 管道表，`\|` 转义 | `packages/extension-table/src/table/table.ts`、`utilities/markdown.ts` |

### 已核实的坑点（按影响排序）

1. **序列化是“规范化重写”，不是逐字回写。** `MarkdownManager.escapeMarkdownSyntax` 对普通文本转义 `` \ ` * _ [ ] ~ ``，`encodeHtmlEntities` 把 `& < >` 写成实体（`packages/core/src/utilities/htmlEntities.ts`）；`**bold**` 变 `**bold**` 不变，但 `a_b_c`（非强调下划线）会变 `a\_b\_c`，`<` 会变 `&lt;`。语义不变（再次 parse 可还原），但**每次保存都可能产生大 diff**，也让 git 式历史/外部编辑器协作变吵。
2. **未识别内容会被静默丢弃。** 无 handler 的块 token 走 `parseFallbackToken` 的 `default` 分支，若无子 token 直接返回 `null`（`MarkdownManager.ts`）。官方文档明确列出：**Markdown 注释“unsupported and may be lost”**。典型受害者：`<!-- -->`、链接定义 `[id]: url`、脚注（核心 marked 不解析脚注，`[^1]` 会退化成普通文本段落并被转义，语义保留但形态改变）。→ 必须用 Q2 的直通节点兜底。
3. **混合任务/普通列表被拆分。** `parseListToken`：同一 `ul` 里既有 `- [ ]` 又有 `- ` 项时，会被切成相邻的独立 `taskList` 与 `bulletList` 两个节点。AST 结构发生变化（再序列化后 `-` 项与 task 项分家）。
4. **连续空段落引入 `&nbsp;` 噪声。** 空段落序列化为首个空行、连续第 2+ 个输出 `&nbsp;` 段落（`packages/extension-paragraph/src/paragraph.ts` 的 `EMPTY_PARAGRAPH_MARKDOWN`），这是为保往返而故意写进源的污染文本；remark 渲染时 `&nbsp;` 显示为空白，无视觉问题但源变脏。
5. **跨边界的 mark 可能以 HTML 形式重新打开。** 官方 mark 可声明 `markdownOptions.htmlReopen`（bold = `<strong></strong>`，`bold.tsx`），当强调跨过一个内联节点边界时，序列化输出会在中间插入 `<strong>` 原始 HTML —— 合法 Markdown（CommonMark 允许内联 HTML），但 react-markdown 默认不渲染裸 HTML，会以文本露出。低频，预览侧可用 `skipHtml` 一致化处理。
6. **StarterKit 自带的 `Underline` 序列化为 `++text++`**（非 GFM；`packages/extension-underline/src/underline.ts`）。若不禁用，Ctrl/Cmd+U 会把扩展语法写进 `note.content`。→ 从扩展列表剔除 Underline，或覆盖其 `renderMarkdown`。`Highlight`（`==`）同理勿加入。
7. **表格单元格只允许一个子节点**（官方 docs 明示：MD 表格语法表达不了多子节点），多段落单元格会被压扁——在 PinNote 的表格里限制 Enter 换行为硬换行即可。
8. **官方自述 early release**：tiptap.dev markdown 文档横幅注明“may carry unsupported edge cases”。→ 用 fixture 往返测试钉死（拿现有 `frontend/src/lib/markdown.ts` 各工具函数能产出的语法 + 典型手写 Markdown 做 corpus，断言 `parse→serialize→parse` 双解析 AST 全等）。
9. 与 PinNote 预览的一致性：现有 `MarkdownView` 用 react-markdown+remark-gfm（CommonMark 语义 + GFM 表格/删除线/任务项）。`@tiptap/markdown` 底层是 marked（默认 `gfm:true, breaks:false`，`MarkdownManager` options），两侧语法语义基本同构；主要差异即上面 2/5/7（裸 HTML、注释）。`markedOptions` 可注入以对齐行为（`MarkdownExtensionOptions.markedOptions`，`packages/markdown/src/Extension.ts`）。

## 2. 不丢数据策略（解析失败的原始行必须逐字保留并写回）

先厘清“解析失败”：CommonMark 对任意输入都能 lex（marked 同样近乎 total），所以真正的风险不是“报错”，而是**被丢弃或被改写语义**。ProseMirror 不保留源位置（无 source map，doc 模型见 [ProseMirror Guide §1](https://prosemirror.net/docs/guide/)），因此“按原始行 diff 合并”不成立：每个字段每次保存都可能因规范化而“看起来被改动”。

### 推荐方案：按块审计 + `rawSource` 直通原子节点（机制均为官方扩展点）

1. **新增自定义块节点 `rawSource`**：`group:'block'`、`atom:true`、属性 `source:string`；`renderMarkdown: node => node.attrs.source`（逐字回写）。官方为此提供了扩展点：节点可携带 `markdownTokenizer`（marked 块级扩展，官方 `MarkdownManager.registerTokenizer` 注册，taskList/orderedList/table 即先例）与 `parseMarkdown/renderMarkdown`（`packages/core/src/types.ts` 的 `MarkdownTokenizer`/`MarkdownExtensionSpec`；公共助手 `createBlockMarkdownSpec / createInlineMarkdownSpec / createAtomBlockMarkdownSpec` 由 `@tiptap/core` 导出，`packages/core/src/utilities/markdown/`，mention 等在用）。
2. **装载时逐块审计**（用与 manager 相同的 marked 词法器，`editor.markdown.instance` 暴露了 marked 实例，`MarkdownManager` 的 `get instance`）：对每个顶层 token——
   - 若其 `type` 在“有 handler 且往返稳定”的白名单内 → 交 `editor.markdown.parse` 正常解析；
   - 否则（`html` 注释、`def`、以及任何“解析后序列化不再能还原语义”的块——可用 `serialize(parse(raw))` 与本块 raw 做结构比对）→ 直接生成 `rawSource` 节点，`source = token.raw`（marked token 天然携带 verbatim `raw` 文本）。
3. **编辑期保证**：`rawSource` 为 atom，正文编辑碰不到它；node view 渲染为等宽只读块 + “转为可编辑”按钮（点击时把 source 以 `insertContent(source, {contentType:'markdown'})` 重新注入解析——该命令原生支持 markdown 文本入参，`packages/markdown/src/Extension.ts`）。
4. **保存**：`getMarkdown()` 天然把 `rawSource` 逐字写回。零 diff 合并、零特判。

补充一条“防丢保险”（低成本、强烈建议）：每次保存前做 `parse(serialize(doc))` 深度等于 `doc` 的断言（JSON 比较），不一致则**拒写并回退为整篇原文入 `rawSource`**、提示切换到源码模式。这把“官方包 early-release 边角 case”变成可检测事件而非数据丢失。

若产品后续要求“逐字保真优先于规范化”（例如笔记要与外部编辑器共享），升级为**按块脏标记**：装载时给每个顶层块存 `attrs.srcRaw` + `dirty:false`，用 `prosemirror Changes` 插件把 `tr.mapping` 反查到的块置脏；序列化时对干净块用 `srcRaw`、脏块走 serializer。成本明显更高，PinNote 单编辑面场景下第一版不必做。

## 3. 富文本粘贴 → Markdown

成立，且正是 ProseMirror/TipTap 的默认管线：**粘贴 HTML → schema 的 `parseHTML` 规则 → doc → `getMarkdown()` 序列化为 MD**。

- 富文本（带 `text/html`）：剪贴板管线由 ProseMirror 完成（[Guide §3.2 Pasting](https://prosemirror.net/docs/guide/)），TipTap 在 `Editor` 组装期把各扩展的 `transformPastedHTML` 钩子链式合成后交给 ProseMirror（`packages/core/src/Editor.ts` L592-604），可在入 doc 前清洗（如从 Notion/网页粘贴时剥掉 `<span style>`）。表格/列表/粗斜体链接都在 parseDOM 覆盖内，故能落到对应节点再序列化。
- 纯 Markdown 文本粘贴：官方 markdown 包**没有**内置自动转换（`packages/markdown/src` 无 paste 代码）。需要自建：`handlePaste`（或扩展 `transformPastedText` 钩子）中当 clipboard 只有 `text/plain` 且疑似 markdown 时，拦截并 `insertContent(text, { contentType:'markdown' })`。官方 `OrderedList` 扩展自己就用这个 handlePaste 模式处理纯文本列表粘贴（`ordered-list.ts` `addProseMirrorPlugins`），是站内先例。
- 反向（复制/导出）：默认复制文本是纯文本拼接；要 `note.content` 语义的复制，用官方 Clipboard 工具/覆写 `clipboardTextSerializer: () => editor.getMarkdown()`（docs: tiptap.dev → Editor → API → Utilities → Clipboard）。

## 4. 任务复选框点击回写

路径完全成立，且是官方内置行为，比 PinNote 现状（预览层 `toggleCheckboxAtLine` 按行号改源，见 `frontend/src/lib/markdown.ts` L107-115、`MarkdownView.tsx`）干净得多：

- `TaskItem` 自带 NodeView：渲染 `<input type=checkbox>`（`contentEditable=false` 的 label 包裹），`change` 事件里 `tr.setNodeMarkup(pos, undefined, {...attrs, checked})` 直接改 doc 节点属性（`packages/extension-list/src/task-item/task-item.ts` `addNodeView`）。
- 编辑器 `onUpdate` → `editor.getMarkdown()` → 写回 `note.content`。taskItem 的 `renderMarkdown` 输出 `- [x] `/`- [ ] `（同文件 L182-187），`parseMarkdown` 读回 `checked` 属性；readonly 态另有 `onReadOnlyChecked` 回调选项。
- 注意点：`- [X]`（大写 X）解析接受（tokenizer 字符类 `[ xX]`）但回写恒为小写 `x`——与 PinNote 现有 `toggleCheckboxAtLine` 行为一致（也写小写 x）。混合列表拆分坑见 Q1 第 3 条。

## 5. 推荐扩展清单 / 序列化方案 / 体积许可

**编辑器**：`@tiptap/react`（v3，配 React 19）+ `@tiptap/pm`。

**扩展清单**（均 3.30.5、MIT）：

- `StarterKit`（Document/Paragraph/Text/Heading/Blockquote/Bold/Italic/Strike/Code/CodeBlock/HardBreak/HorizontalRule/Link/Underline + BulletList/ListItem/OrderedList/ListKeymap + UndoRedo/Dropcursor/Gapcursor/TrailingNode，逐一核对 `packages/starter-kit/src/starter-kit.ts`；**不含 TaskList/Table/Markdown**）。需**减去 Underline**：`StarterKit.configure({ underline: false })`。
- `@tiptap/extension-list` 的 `TaskList` + `TaskItem`（StarterKit 不含）。
- `@tiptap/extension-table` 的 `Table/TableRow/TableHeader/TableCell`。
- `@tiptap/markdown`（`Markdown` 扩展）。
- 自写 `rawSource` 节点（Q2）。
- 不需要：Underline、Highlight（`==` 非 GFM）、TextStyle/Color（MD 无对应）、Placeholder（整窗即编辑区，用 CSS 伪元素占位即可）。

**序列化**：`editor.getMarkdown()` / `setContent(md,{contentType:'markdown'})`，底层 marked（MIT）。

**维护状态与许可**：官方 monorepo 活跃发布（core 最新发版 2026-08-26），全链路 MIT，无商用风险；社区 `tiptap-markdown` 已弃维护（作者原话，见 §0），**不用**。

**体积（Wails 桌面 WebView 场景，量级参考即可）**：Bundlephobia 实测 `@tiptap/starter-kit@3.30.5` minified ≈ **337 KB**（gzip ≈ **105 KB**，24 deps，含 @tiptap/core）；`@tiptap/markdown` npm 解包 ≈ 433 KB、`marked` ≈ 480 KB 解包（marked 的 gzip 典型在 10–16 KB 量级）。TaskList/Table/React 胶水另计约几十 KB。对本地桌面应用完全可接受；无法用 code-splitting 也无所谓——首屏只有一个编辑器。

**v2 vs v3 关键告诫**：本项目应直接上 **v3**（v2 的 `tiptap-markdown@0.8` 生态文档不再适用；v3 官方包 API 与 v2 社区包不同：`editor.getMarkdown()` 而非 `editor.storage.markdown.getMarkdown()`；TaskList/TaskItem 在 v3 并入 `@tiptap/extension-list`；`setContent` 的 markdown 入口是 `{contentType:'markdown'}` 选项而非重载参数）。注意 v3 下 `note.content` 加载/保存必须始终带 `contentType:'markdown'`，否则会被当 HTML/JSON 解析。

**NO-GO 触发线**：若 early-release 边角 case 在实际语料上频繁触发（防丢保险断言频繁失败），fallback 顺序：① 继续 TipTap，把无法映射块全部降为 `rawSource` 只读块（体验降级但数据零丢失）；② 回退到“渲染视图 + 源码 textarea”双模式（即现状增强版，放弃整窗 WYSIWYG）；③ 换社区 `tiptap-markdown@0.9`（仅当缺陷确为官方包独有且其可绕开——预期不会走到）。

## 落地下一步（供后续 ticket 参考，不在本 ticket 内执行）

1. 用 `frontend/src/lib/markdown.ts` 各 FormatFn 的输出 + 手写语料建往返 fixture，断言 `parse∘serialize∘parse` 幂等；
2. `MarkdownView.tsx` 保留给非 pin 的只读场景；pin 窗口挂 TipTap 编辑器组件，`onUpdate → PUT /notes/:id`；
3. 删除 `toggleCheckboxAtLine` 在 pin 模式的使用（被 TaskItem NodeView 取代）。
