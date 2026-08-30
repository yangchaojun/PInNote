import { Button, Empty, List, Popconfirm, Typography } from "antd";
import { DeleteOutlined, RedoOutlined } from "@ant-design/icons";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import * as api from "../lib/api";
import { daysUntilPurge } from "../lib/api";

/** Trash list: restore items or delete them forever; items purge after 60 days. */
export function TrashView() {
  const queryClient = useQueryClient();
  const { data: notes = [], isLoading } = useQuery({ queryKey: ["trash"], queryFn: api.listTrash });

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["trash"] });
    queryClient.invalidateQueries({ queryKey: ["notes"] });
  };

  const restore = async (id: string) => {
    await api.restoreNote(id);
    invalidate();
  };

  const removeForever = async (id: string) => {
    await api.deleteNoteForever(id);
    invalidate();
  };

  if (!isLoading && notes.length === 0) {
    return (
      <div className="trash-view">
        <Empty description="回收站是空的" image={Empty.PRESENTED_IMAGE_SIMPLE} />
      </div>
    );
  }

  return (
    <div className="trash-view">
      <div className="trash-header">
        <Typography.Text type="secondary">回收站中的笔记保留 {api.TRASH_RETENTION_DAYS} 天，之后自动清除</Typography.Text>
        <Popconfirm
          title="清空回收站？"
          description="所有回收站内的笔记将被永久删除。"
          okText="清空"
          cancelText="取消"
          okButtonProps={{ danger: true }}
          onConfirm={async () => {
            await api.emptyTrash();
            invalidate();
          }}
        >
          <Button size="small" danger icon={<DeleteOutlined />} tabIndex={-1}>
            清空回收站
          </Button>
        </Popconfirm>
      </div>
      <List
        size="small"
        loading={isLoading}
        dataSource={notes}
        renderItem={(note) => {
          const days = daysUntilPurge(note.deletedAt ?? 0);
          return (
            <List.Item
              className="trash-item"
              actions={[
                <Button key="restore" size="small" icon={<RedoOutlined />} onClick={() => restore(note.id)} tabIndex={-1}>
                  恢复
                </Button>,
                <Popconfirm
                  key="delete"
                  title="永久删除？"
                  description="该笔记将被永久删除，无法恢复。"
                  okText="删除"
                  cancelText="取消"
                  okButtonProps={{ danger: true }}
                  onConfirm={() => removeForever(note.id)}
                >
                  <Button size="small" danger icon={<DeleteOutlined />} tabIndex={-1} />
                </Popconfirm>,
              ]}
            >
              <div className="trash-item-main">
                <span className="trash-item-title">{note.title || "无标题笔记"}</span>
                <Typography.Text type="secondary" className="trash-item-meta">
                  {days > 0 ? `${days} 天后自动清除` : "即将自动清除"}
                </Typography.Text>
              </div>
            </List.Item>
          );
        }}
      />
    </div>
  );
}
