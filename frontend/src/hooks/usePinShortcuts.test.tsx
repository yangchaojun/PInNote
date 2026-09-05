import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import type { Editor } from "@tiptap/core";
import { usePinShortcuts } from "./usePinShortcuts";

vi.mock("../theme", () => ({ toggleTheme: vi.fn() }));

import { toggleTheme } from "../theme";

function pressKey(init: KeyboardEventInit) {
  window.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init }));
}

function fakeEditor(activeLink = false) {
  const run = vi.fn(() => true);
  const setLink = vi.fn(() => ({ run }));
  const unsetLink = vi.fn(() => ({ run }));
  const extendMarkRange = vi.fn(() => ({ setLink }));
  // The hook's unset path is chain().focus().unsetLink(); its set path is
  // chain().focus().extendMarkRange("link").setLink(...).
  const focus = vi.fn(() => ({ extendMarkRange, unsetLink }));
  const editor = {
    isActive: vi.fn(() => activeLink),
    chain: vi.fn(() => ({ focus })),
    commands: { unsetLink },
  } as unknown as Editor;
  return { editor, run, setLink, unsetLink, focus, extendMarkRange };
}

interface HarnessProps {
  onDelete: () => void;
  onToggleHelp: () => void;
  helpOpen: boolean;
  onCloseHelp: () => void;
  editor: Editor | null;
}

function Harness(props: HarnessProps) {
  usePinShortcuts({
    getEditor: () => props.editor,
    onDelete: props.onDelete,
    onToggleHelp: props.onToggleHelp,
    helpOpen: props.helpOpen,
    onCloseHelp: props.onCloseHelp,
  });
  return null;
}

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
});

describe("usePinShortcuts 键位", () => {
  it("⌘⌫ 触发删除", () => {
    const onDelete = vi.fn();
    render(<Harness onDelete={onDelete} onToggleHelp={vi.fn()} helpOpen={false} onCloseHelp={vi.fn()} editor={null} />);
    pressKey({ key: "Backspace", metaKey: true });
    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  it("⌘⇧/ 与 ⌘⇧? 都切换帮助模态框", () => {
    const onToggleHelp = vi.fn();
    render(<Harness onDelete={vi.fn()} onToggleHelp={onToggleHelp} helpOpen={false} onCloseHelp={vi.fn()} editor={null} />);
    pressKey({ key: "/", metaKey: true, shiftKey: true });
    pressKey({ key: "?", metaKey: true, shiftKey: true });
    expect(onToggleHelp).toHaveBeenCalledTimes(2);
  });

  it("Esc 仅在帮助打开时关闭它", () => {
    const onCloseHelp = vi.fn();
    const { rerender } = render(
      <Harness onDelete={vi.fn()} onToggleHelp={vi.fn()} helpOpen={false} onCloseHelp={onCloseHelp} editor={null} />,
    );
    pressKey({ key: "Escape" });
    expect(onCloseHelp).not.toHaveBeenCalled();

    rerender(
      <Harness onDelete={vi.fn()} onToggleHelp={vi.fn()} helpOpen onCloseHelp={onCloseHelp} editor={null} />,
    );
    pressKey({ key: "Escape" });
    expect(onCloseHelp).toHaveBeenCalledTimes(1);
  });

  it("⌘⇧D 切换主题", () => {
    render(<Harness onDelete={vi.fn()} onToggleHelp={vi.fn()} helpOpen={false} onCloseHelp={vi.fn()} editor={null} />);
    pressKey({ key: "d", metaKey: true, shiftKey: true });
    expect(toggleTheme).toHaveBeenCalledTimes(1);
  });

  it("⌘K 无链接时询问地址并设置链接", () => {
    const { editor, setLink, extendMarkRange, run } = fakeEditor(false);
    const prompt = vi.spyOn(window, "prompt").mockReturnValue("https://example.com");
    render(<Harness onDelete={vi.fn()} onToggleHelp={vi.fn()} helpOpen={false} onCloseHelp={vi.fn()} editor={editor} />);
    pressKey({ key: "k", metaKey: true });
    expect(prompt).toHaveBeenCalled();
    expect(extendMarkRange).toHaveBeenCalledWith("link");
    expect(setLink).toHaveBeenCalledWith({ href: "https://example.com" });
    expect(run).toHaveBeenCalledTimes(1);
  });

  it("⌘K 已有链接时移除链接，不再询问地址", () => {
    const { editor, unsetLink, setLink } = fakeEditor(true);
    const prompt = vi.spyOn(window, "prompt").mockReturnValue("https://example.com");
    render(<Harness onDelete={vi.fn()} onToggleHelp={vi.fn()} helpOpen={false} onCloseHelp={vi.fn()} editor={editor} />);
    pressKey({ key: "k", metaKey: true });
    expect(unsetLink).toHaveBeenCalledTimes(1);
    expect(setLink).not.toHaveBeenCalled();
    expect(prompt).not.toHaveBeenCalled();
  });

  it("⌘K 用户取消时不动文档", () => {
    const { editor, setLink } = fakeEditor(false);
    vi.spyOn(window, "prompt").mockReturnValue(null);
    render(<Harness onDelete={vi.fn()} onToggleHelp={vi.fn()} helpOpen={false} onCloseHelp={vi.fn()} editor={editor} />);
    pressKey({ key: "k", metaKey: true });
    expect(setLink).not.toHaveBeenCalled();
  });
});
