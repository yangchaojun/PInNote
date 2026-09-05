import { cleanup, render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TrayPanel, noteExcerpt } from "./TrayPanel";
import * as api from "./lib/api";
import { Events } from "@wailsio/runtime";
import { events as callOrder } from "./testkit";

vi.mock("./lib/api", async () => {
  const note = (id: string, content: string, updatedAt = 1, title = "") => ({
    id,
    title,
    content,
    pinned: false,
    deletedAt: null,
    createdAt: 1,
    updatedAt,
  });
  return {
    listNotes: vi.fn(async () => [
      note("n1", "# 最早的\n正文一", 1, "最早的"),
      note("n2", "# 最新的\n正文二", 2, "最新的"),
    ]),
    openPinnedWindow: vi.fn(async () => {
      callOrder.push("openPinnedWindow");
      return true;
    }),
    createNote: vi.fn(async () => {
      callOrder.push("createNote");
      return note("n3", "");
    }),
    hidePanel: vi.fn(async () => {
      callOrder.push("hidePanel");
    }),
  };
});

vi.mock("@wailsio/runtime", () => {
  const handlers: Record<string, Array<(ev: unknown) => void>> = {};
  return {
    Events: {
      On: vi.fn((name: string, cb: (ev: unknown) => void) => {
        (handlers[name] ??= []).push(cb);
        return () => {
          handlers[name] = (handlers[name] ?? []).filter((h) => h !== cb);
        };
      }),
      __emit: (name: string, ev?: unknown) => (handlers[name] ?? []).forEach((cb) => cb(ev)),
    },
  };
});

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <TrayPanel />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  callOrder.length = 0;
  vi.clearAllMocks();
});

describe("noteExcerpt", () => {
  it("strips markdown and drops the title line and images", () => {
    expect(
      noteExcerpt({
        id: "n",
        title: "标题",
        content: "# 标题\n看 **这里** [链接](https://x.y) ![图](a.png)\n第二段",
        pinned: false,
        deletedAt: null,
        createdAt: 1,
        updatedAt: 1,
      }),
    ).toBe("看 这里 链接 第二段");
  });

  it("returns empty for a title-only note", () => {
    expect(
      noteExcerpt({
        id: "n",
        title: "标题",
        content: "标题",
        pinned: false,
        deletedAt: null,
        createdAt: 1,
        updatedAt: 1,
      }),
    ).toBe("");
  });
});

describe("TrayPanel", () => {
  it("lists notes most-recently-updated first with title and excerpt", async () => {
    renderPanel();
    const items = await screen.findAllByRole("button");
    expect(items).toHaveLength(2);
    expect(items[0].textContent).toContain("最新的");
    expect(items[0].textContent).toContain("正文二");
    expect(items[1].textContent).toContain("最早的");
  });

  it("focuses the note's pin window then hides the panel, in that order", async () => {
    renderPanel();
    fireEvent.click(await screen.findByTitle("最早的"));
    await waitFor(() => expect(api.hidePanel).toHaveBeenCalled());
    expect(api.openPinnedWindow).toHaveBeenCalledWith("n1");
    expect(callOrder).toEqual(["openPinnedWindow", "hidePanel"]);
  });

  it("refreshes the list when notes:changed fires", async () => {
    renderPanel();
    await screen.findAllByRole("button");
    expect(vi.mocked(api.listNotes)).toHaveBeenCalledTimes(1);
    (Events as unknown as { __emit: (name: string, ev?: unknown) => void }).__emit(
      "notes:changed",
      { data: null },
    );
    await waitFor(() => expect(vi.mocked(api.listNotes)).toHaveBeenCalledTimes(2));
  });

  it("offers a create button in the empty state that creates and focuses a pin window", async () => {
    vi.mocked(api.listNotes).mockResolvedValueOnce([]);
    renderPanel();
    const create = await screen.findByRole("button", { name: /新建笔记/ });
    fireEvent.click(create);
    await waitFor(() => expect(api.hidePanel).toHaveBeenCalled());
    expect(api.createNote).toHaveBeenCalledWith("");
    expect(callOrder).toEqual(["createNote", "openPinnedWindow", "hidePanel"]);
  });
});
