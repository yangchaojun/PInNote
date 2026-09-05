import { act, cleanup, render, waitFor } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AUTOSAVE_DELAY_MS, PinEditor, type PinEditorHandle } from "./PinEditor";
import * as api from "./lib/api";
import { events } from "./testkit";

vi.mock("./lib/api", () => ({
  updateNote: vi.fn(async () => {
    events.push("updateNote");
    return {
      id: "n1",
      title: "",
      content: "",
      pinned: false,
      deletedAt: null,
      createdAt: 0,
      updatedAt: 0,
    };
  }),
}));

/** Insert text through a real ProseMirror transaction (drives onUpdate). */
function typeText(handle: PinEditorHandle, text: string) {
  const ed = handle.getEditor();
  if (!ed) throw new Error("editor not mounted");
  ed.commands.command(({ tr }) => {
    tr.insertText(text);
    return true;
  });
}

afterEach(cleanup);

beforeEach(() => {
  events.length = 0;
  vi.clearAllMocks();
});

describe("PinEditor 保存管线", () => {
  it("装载走审计加载器（标题渲染为 heading），400ms debounce 后回写事实源", async () => {
    const handle = createRef<PinEditorHandle>();
    const onSaved = vi.fn();
    render(
      <PinEditor ref={handle} noteId="n1" initialContent={"# 标题\n\n正文"} onSaved={onSaved} />,
    );

    await waitFor(() => expect(handle.current?.getEditor()).toBeTruthy());
    // Audited load: the heading became a real heading node.
    expect(handle.current!.getEditor()!.getHTML()).toContain("<h1>");
    expect(events).toEqual([]); // nothing saved yet

    act(() => {
      typeText(handle.current!, "!");
    });
    expect(events).toEqual([]); // still inside the debounce window

    await new Promise((r) => setTimeout(r, AUTOSAVE_DELAY_MS + 80));
    expect(events).toEqual(["updateNote"]);
    expect(api.updateNote).toHaveBeenCalledWith("n1", "# 标题\n\n正文!");
    expect(onSaved).toHaveBeenCalledTimes(1);
    // Dirty flag cleared after a successful save.
    expect(handle.current!.isDirty()).toBe(false);
  });

  it("窗口失焦立即 flush，不等 debounce", async () => {
    const handle = createRef<PinEditorHandle>();
    render(<PinEditor ref={handle} noteId="n1" initialContent="草稿" onSaved={vi.fn()} />);
    await waitFor(() => expect(handle.current?.getEditor()).toBeTruthy());

    act(() => {
      typeText(handle.current!, " 追加");
    });
    act(() => {
      window.dispatchEvent(new Event("blur"));
    });
    await waitFor(() => expect(events).toEqual(["updateNote"]));
    expect(api.updateNote).toHaveBeenCalledWith("n1", "草稿 追加");
  }, 10_000);

  it("无编辑时失焦不产生写库", async () => {
    const handle = createRef<PinEditorHandle>();
    render(<PinEditor ref={handle} noteId="n1" initialContent="原文" onSaved={vi.fn()} />);
    await waitFor(() => expect(handle.current?.getEditor()).toBeTruthy());

    act(() => {
      window.dispatchEvent(new Event("blur"));
    });
    await new Promise((r) => setTimeout(r, 60));
    expect(events).toEqual([]);
    expect(api.updateNote).not.toHaveBeenCalled();
  });

  it("beforeunload 兜底 flush", async () => {
    const handle = createRef<PinEditorHandle>();
    render(<PinEditor ref={handle} noteId="n1" initialContent="" onSaved={vi.fn()} />);
    await waitFor(() => expect(handle.current?.getEditor()).toBeTruthy());

    act(() => {
      typeText(handle.current!, "⌘Q 前的输入");
    });
    act(() => {
      window.dispatchEvent(new Event("beforeunload"));
    });
    await waitFor(() => expect(events).toEqual(["updateNote"]));
  }, 10_000);
});
