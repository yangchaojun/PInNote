import { Modal } from "antd";

// The full shortcut set, kept identical to spec §4.2.
const SHORTCUTS: Array<[string, string]> = [
  ["⌘⌥N", "新建笔记并打开 pin 窗口（全局，任何应用下可用）"],
  ["⌘B / ⌘I / ⌘K", "粗体 / 斜体 / 链接"],
  ["⌘⇧D", "切换亮/暗主题（所有窗口同时生效）"],
  ["⌘⌫", "删除当前笔记（进回收站）并关窗"],
  ["⌘⇧/", "显示本帮助"],
  ["Esc", "关闭本帮助"],
];

export function ShortcutsModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <Modal title="键盘快捷键" open={open} footer={null} onCancel={onClose} width={480}>
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
