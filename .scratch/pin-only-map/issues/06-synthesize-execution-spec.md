# 06 — 汇总：纯 pin 形态可执行 spec

Type: task
Status: resolved

## Question

把 02–05 的全部决策合成为一份**可直接开工的 spec**（wayfinder 到此为止，执行交回）：

- 汇总为 `.scratch/pin-only-map/spec.md`（或等价文档）：产品形态、交互规格、编辑器实现方案、窗口规格、生命周期规则、逐文件影响清单、验收标准（每条 spec 论断可被指认到界面行为）。
- 同步更新 `CONTEXT.md`（术语：pin 窗口、WYSIWYG、事实源 Markdown 等）与必要的 ADR（如"移除主面板""Markdown 为唯一事实源 + WYSIWYG 编辑"若判定满足 ADR 三条件）。
- 检查 01 的 research 结论是否需要并入 spec 附录。

产出后，本 map 应无剩余决策，到达 Destination。

Blocked by: 05

## Answer

**完成（2026-09-05）。全部产出：**

1. **可执行 spec**：`.scratch/pin-only-map/spec.md`——产品形态、窗口规格、编辑器实现方案、标题语义、交互与快捷键、生命周期、图片决策、逐文件改动地图、13 条可指认的验收标准、文档同步清单。各 ticket Answer 与 spec 冲突时以 spec 为准。
2. **CONTEXT.md**：新增「收起」「首行标题」「降级块（rawSource）」术语；「固定/取消固定」「未固定笔记」标注退役（ADR-0001）。
3. **ADR 两篇**（均满足难逆转 + 多合理选项 + 影响后续决策三条件）：`docs/adr/0001-remove-main-panel.md`、`docs/adr/0002-markdown-source-of-truth-wysiwyg.md`。
4. **01 research 附录检查**：不必并入——spec §2 已把编辑器实现所依赖的结论（TipTap v3 + `@tiptap/markdown`、审计双保险、降级策略）内联；完整报告保持在 `research/tiptap-markdown-roundtrip` 分支，spec §9 标注为编辑器实现的必读附件。

**遗留给执行阶段的两个非决策事项**（spec 已覆盖，非 map 级开放点）：resize 原生路径实现首步实测（§1.2）；wails3 CLI 可用时 `setTheme` 从 `Call.ByName` 换正式 binding（05）。

Map 无剩余决策，Destination 达成。
