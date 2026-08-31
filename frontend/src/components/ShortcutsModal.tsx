import { Modal } from "antd";
import { useUIStore } from "../store";

const SHORTCUTS: Array<[string, string]> = [
  ["⌘ Option N", "全局呼出 PinNote 并新建笔记（任何应用下可用）"],
  ["⌘ N", "新建笔记"],
  ["⌘ F", "搜索笔记"],
  ["⌥ ↓ / ⌥ ↑", "切换到下一条 / 上一条笔记"],
  ["⌘ E", "切换 Markdown 预览"],
  ["⌘ P", "固定 / 取消固定当前笔记到桌面"],
  ["⌘ ⌫", "将当前笔记移入回收站"],
  ["⌘ ⇧ T", "切换 笔记 / 回收站 视图"],
  ["⌘ ⇧ D", "切换 暗色 / 亮色 主题"],
  ["⌘ B / ⌘ I / ⌘ K", "粗体 / 斜体 / 插入链接"],
  ["⌘ ⇧ 7 / ⌘ ⇧ 8", "有序列表 / 无序列表"],
  ["⌘ ⇧ X", "待办事项（复选框）"],
  ["⌘ ⇧ 9", "引用块"],
  ["⌘ ⇧ /", "显示本帮助"],
  ["Esc", "清除搜索 / 关闭弹窗"],
];

export function ShortcutsModal() {
  const open = useUIStore((s) => s.shortcutsOpen);
  return (
    <Modal
      title="键盘快捷键"
      open={open}
      footer={null}
      onCancel={() => useUIStore.getState().setShortcutsOpen(false)}
      width={480}
    >
      <div className="shortcuts-list">
        {SHORTCUTS.map(([keys, desc]) => (
          <div className="shortcut-row" key={keys}>
            <kbd className="shortcut-keys">{keys}</kbd>
            <span className="shortcut-desc">{desc}</span>
          </div>
        ))}
      </div>
    </Modal>
  );
}
