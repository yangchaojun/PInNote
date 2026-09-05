# ADR-0002: Markdown 为唯一事实源，Pin 窗口整窗 WYSIWYG 编辑

日期：2026-09-05 · 状态：已接受 · 依据：wayfinder tickets 01/02（research + 可运行原型，均 GO）

## 背景

Pin 窗口要求"打开即写、语法原地即时渲染"（Apple Notes 式），同时数据库存储与既有数据都是 Markdown。二者天然冲突：WYSIWYG 编辑器的内部文档模型（富文本树）与 Markdown 文本互不为对方的自然表示，往返不当会静默丢内容。

## 决策

1. `note.content` 的 Markdown 文本是唯一事实源；WYSIWYG 视图只是它的一种可编辑呈现。
2. 编辑器选型 TipTap v3 + 官方 `@tiptap/markdown`；往返范围 = 现有 GFM 子集。
3. 数据安全机制（原型已验证，随实现晋升）：
   - **审计双保险**：静态 token 类型门 + 行为不动点（`parse(serialize(doc)) ≡ doc`），两道全过才可安全装载/编辑；
   - **降级保数据**：审计未通过的块以 rawSource 原子块（源码形态）显示与写回；
   - **保存守卫**：守卫比较前归一化顶层空段落；不过则拒写、保留降级态，绝不静默丢内容；
   - `SafeParagraph`：段落序列化转义行首块语法，防重开后段落变标题。
4. 行内图片本期不支持（image 在类型门排除清单，含图块整体降级）——图片存储方案未定前不引入 `@tiptap/extension-image`。

## 理由

- 放弃 Markdown 事实源（纯富文本存 HTML/JSON）会破坏既有数据与"纯文本可携带"的产品性质；放弃 WYSIWYG 则整窗编辑的产品目标不成立。
- 原型实测证明纯行为审计会漏 `<details>` 类解析丢标签、纯静态白名单会漏 codespan 相邻 backtick 等边界——双保险缺一不可。

## 影响

- `MarkdownView`（只读渲染）与 textarea `Editor` 退役；`lib/markdown.ts` 的 textarea 格式化函数出界。
- 原型 `audit.ts`/`rawSource.ts`/`SafeParagraph`/语料网（109 项回归）直接晋升为正式实现。
- 序列化是规范化重写：保存可能规整用户手写的 Markdown（如 HTML 注释类不可映射内容走降级块），属预期行为。
