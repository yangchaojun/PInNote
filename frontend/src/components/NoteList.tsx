import { Empty, List, Typography } from "antd";
import { PushpinFilled } from "@ant-design/icons";
import * as api from "../lib/api";

interface NoteListProps {
  notes: api.Note[];
  activeId: string | null;
  onSelect: (id: string) => void;
}

/** Sidebar list of notes, pinned notes first (as returned by the backend). */
export function NoteList({ notes, activeId, onSelect }: NoteListProps) {
  if (notes.length === 0) {
    return (
      <div className="note-list-empty">
        <Empty description="暂无笔记，按 ⌘N 新建" image={Empty.PRESENTED_IMAGE_SIMPLE} />
      </div>
    );
  }

  const groups: Array<[string, api.Note[]]> = [
    ["已固定", notes.filter((n) => n.pinned)],
    ["全部笔记", notes.filter((n) => !n.pinned)],
  ];

  return (
    <div className="note-list">
      {groups.map(([label, items]) =>
        items.length === 0 ? null : (
          <div key={label} className="note-list-group">
            <div className="note-list-group-label">
              {label === "已固定" && <PushpinFilled className="pin-icon" />}
              <Typography.Text type="secondary">{label}</Typography.Text>
            </div>
            <List
              size="small"
              dataSource={items}
              renderItem={(note) => (
                <div
                  className={`note-item${note.id === activeId ? " active" : ""}`}
                  onClick={() => onSelect(note.id)}
                  role="button"
                  tabIndex={-1}
                >
                  <span className="note-item-title">{note.title || "无标题笔记"}</span>
                  <span className="note-item-meta">
                    {new Date(note.updatedAt * 1000).toLocaleDateString("zh-CN", { month: "numeric", day: "numeric" })}
                  </span>
                </div>
              )}
            />
          </div>
        ),
      )}
    </div>
  );
}
