# 03 — 钉死：pin 窗口交互与窗口规格 spec

Type: grilling
Status: open

## Question

纯 pin 形态下，pin 窗口的完整交互规格是什么？逐项定稿：

1. 高度策略落地：默认 300px / 最小 150px（现状 `MIN_HEIGHT=120` 与自适应增高逻辑将移除）、手动 resize 的实现方式（Frameless macOS 窗口如何拖边调整）、超出内部滚动的滚动条样式。
2. 标题语义：移除主面板后 `note.title` 如何产生/展示——自动取首行/首个标题？头部按钮悬停显示？定稿其唯一规则（注意 `note.title` 已参与搜索与 `Note.title` 数据模型）。
3. 头部区域：拖拽区（现为 `pin-header`，双击切编辑的绑定移除）、固定/关闭按钮去留、新增"删除（进回收站）"与"主题切换"按钮的排布。
4. 快捷键最终集合：`⌘⌥N` 新建（改道后）、`⌘B/⌘I/⌘K`、`⌘⇧D` 主题、`⌘⌫` 删除、`⌘W` 关窗？逐键定稿；快捷键帮助模态框内容随之重写。
5. 空笔记编辑态的 placeholder 文案。

产出：pin 窗口交互 spec（作为 Answer 或链接的独立 spec 文件）。

Blocked by: 02
