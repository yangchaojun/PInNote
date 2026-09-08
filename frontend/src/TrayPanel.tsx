import { useCallback, useEffect, useMemo } from "react";
import { Button } from "antd";
import { PlusOutlined, PushpinOutlined } from "@ant-design/icons";
import { Events } from "@wailsio/runtime";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import * as api from "./lib/api";
import type { Note } from "./lib/api";

/**
 * The tray panel — a small index of live notes opened from the tray menu's
 * 显示笔记列表 item (the icon's left click summons the latest note instead).
 * It is not a main panel (ADR-0001 retired that): rows only
 * focus the note's pin window, the empty state offers a single create button,
 * and the Go side hides the window as soon as a row is chosen or focus is
 * lost. Sorted most-recently-updated first so the note you just touched is on
 * top.
 */

// The backend derives note.title from the first non-empty content line, so
// the excerpt is the body after that line: markdown markers stripped (images
// dropped, links reduced to their text), whitespace collapsed, truncated.
export function noteExcerpt(note: Note, max = 64): string {
  const lines = note.content.split("\n");
  const first = lines.findIndex((l) => l.trim() !== "");
  if (first === -1) return "";
  const plain = lines
    .slice(first + 1)
    .join(" ")
    .replace(/!\[[^\]]*\]\([^)]*\)/g, "")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/[#>*`~_]+/g, "")
    .replace(/\s+/g, " ")
    .trim();
  return plain.length > max ? plain.slice(0, max) + "…" : plain;
}

export function TrayPanel() {
  const queryClient = useQueryClient();
  const { data: notes } = useQuery({ queryKey: ["notes"], queryFn: api.listNotes });

  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["notes"] });
  }, [queryClient]);

  // Notes can be created, edited, or trashed in any pin window while the
  // panel is open.
  useEffect(() => Events.On("notes:changed", refresh), [refresh]);

  const sorted = useMemo(
    () => [...(notes ?? [])].sort((a, b) => b.updatedAt - a.updatedAt),
    [notes],
  );

  const hide = useCallback(
    () => api.hidePanel().catch((err) => console.error("hide panel", err)),
    [],
  );

  const focusNote = useCallback(
    async (id: string) => {
      await api.openPinnedWindow(id).catch((err) => console.error("focus note", err));
      await hide();
    },
    [hide],
  );

  const createNote = useCallback(async () => {
    try {
      const note = await api.createNote("");
      await api.openPinnedWindow(note.id);
    } catch (err) {
      console.error("create note from panel", err);
    }
    await hide();
  }, [hide]);

  if (!notes) {
    return (
      <div className="tray-panel">
        <div className="tray-panel-empty">
          <p className="pin-hint">加载中…</p>
        </div>
      </div>
    );
  }

  if (!sorted.length) {
    return (
      <div className="tray-panel">
        <div className="tray-panel-empty">
          <PushpinOutlined className="tray-panel-empty-icon" />
          <p className="pin-hint">还没有笔记</p>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => void createNote()}>
            新建笔记
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="tray-panel">
      <ul className="tray-panel-list">
        {sorted.map((n) => {
          const title = n.title || "无标题笔记";
          return (
            <li key={n.id}>
              <button
                type="button"
                className="tray-panel-item"
                title={title}
                onClick={() => void focusNote(n.id)}
              >
                <span className="tray-panel-item-title">{title}</span>
                {noteExcerpt(n) && (
                  <span className="tray-panel-item-excerpt">{noteExcerpt(n)}</span>
                )}
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
