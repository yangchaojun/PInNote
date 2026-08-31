# Prototype: pin 窗口整窗 WYSIWYG 编辑器（wayfinder ticket 02）

一次性样机：验证「整窗 contenteditable WYSIWYG + Markdown 事实源」模型是否成立。
不接 DB / Wails；核心序列化逻辑与线上同构，可晋升为正式实现。

## 运行

```sh
cd prototype/wysiwyg-editor
pnpm install
pnpm test        # 109 个往返/审计/守卫测试（vitest + jsdom）
pnpm dev         # http://localhost:5199 沙盒：左=编辑器，右=事实源 markdown
```

沙盒里可人工过一遍 ticket 02 的验收清单：

- [ ] 键入 `# `、`- `、`- [ `、`**粗体**` —— 原地即时渲染（TipTap input rules，StarterKit 自带）
- [ ] 编辑渲染后的内容，400ms 后右栏事实源同步更新且不变式通过
- [ ] 真 Ctrl+V 粘贴富文本（或点「模拟富文本粘贴」）→ 右栏出现等价 Markdown
- [ ] 真 Ctrl+V 粘贴纯 Markdown 文本 → 原地转成渲染内容（handlePaste 启发式，只处理无 HTML 的 text/plain）
- [ ] 点击任务复选框 → 右栏 `- [ ]` ↔ `- [x]` 回写
- [ ] 注释 / 链接定义等不可映射块 → 黄色虚线 rawSource 块原样保留；「转为可编辑」重新过审计，数据不丢

## 结论（详见 ticket 02 Answer）

模型成立。对 01 推荐实现的实测修正：

1. **审计从「静态白名单」升级为「静态类型门 + 行为不动点」双保险**——纯行为审计会漏
   `<details>`（marked 解析时把标签整个丢掉，不动点照样通过）；纯静态白名单会漏
   backtick-相邻 codespan（序列化不是不动点）。两道门都过才算 safe。
2. **装载按块 trim**——marked 的 block token `raw` 含尾部空行，逐块 parse 会注入幽灵空段落。
3. **守卫的比较要归一化空段落**——TrailingNode 尾段、`&nbsp;` 噪声段、段间单个空段都不携带
   内容，逐结构 eq 会让常见编辑（连按两次回车）永久拒写。不变式保的是数据，不是空行。
4. **段落序列化必须转义行首块语法**（`## x` 文本 → `\## x`）——官方 renderer 不转义，实测
   重开后段落变标题。`SafeParagraph`（editorConfig.ts）补上。
5. **rawSource「转为可编辑」必须重过审计**——直接 `insertContent(markdown)` 会让降级块
   （注释）被解析吞掉，实测丢数据。

## 文件地图

- `src/lib/audit.ts` — 三道门块审计、无损装载器、保存守卫（纯函数，可整体晋升）
- `src/lib/rawSource.ts` — 直通原子块节点
- `src/lib/editorConfig.ts` — 扩展清单 + SafeParagraph 转义（晋升时并入正式编辑器配置）
- `src/lib/roundtrip.test.ts` + `fixtures.ts` — 语料与不变式回归网
- `src/App.tsx` — 沙盒壳（丢弃）
