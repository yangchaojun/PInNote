import { cleanup, render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PinWindow } from "./PinWindow";
import { events } from "./testkit";
import * as api from "./lib/api";
import { emitted } from "./testkit";
import { Events, Window } from "@wailsio/runtime";

const mockFlush = vi.fn(async () => {
  events.push("flush");
});
const mockDirty = vi.fn(() => false);

// The real PinEditor's save pipeline is covered in PinEditor.test.tsx; here it
// is a handle stub so the window-level close/delete ordering is observable.
vi.mock("./PinEditor", async () => {
  const { forwardRef, useImperativeHandle } = await import("react");
  return {
    PinEditor: forwardRef(function MockPinEditor(_props, ref) {
      useImperativeHandle(ref, () => ({
        flush: mockFlush,
        isDirty: () => mockDirty(),
        getEditor: () => null,
      }));
      return null;
    }),
  };
});

vi.mock("./lib/api", async () => {
  const { events } = await import("./testkit");
  const note = (id: string, content: string, deletedAt: number | null = null) => ({
    id,
    title: "标题",
    content,
    pinned: false,
    deletedAt,
    createdAt: 1,
    updatedAt: 1,
  });
  return {
    listNotes: vi.fn(async () => [note("n1", "有内容")]),
    listTrash: vi.fn(async () => []),
    updateNote: vi.fn(async () => {
      events.push("updateNote");
      return note("n1", "");
    }),
    discardIfEmpty: vi.fn(async (id: string) => {
      void id;
      events.push("discardIfEmpty");
      return false;
    }),
    trashNote: vi.fn(async () => {
      events.push("trashNote");
      return note("n1", "");
    }),
    closePinnedWindow: vi.fn(async () => {
      events.push("closePinnedWindow");
    }),
    getTheme: vi.fn(async () => "dark"),
    setTheme: vi.fn(async () => undefined),
  };
});

vi.mock("@wailsio/runtime", async () => {
  // Hoisted factory: the recorder has to come from a module, not this file.
  const { emitted } = await import("./testkit");
  const handlers: Record<string, Array<(ev: unknown) => void>> = {};
  return {
    Events: {
      Emit: vi.fn((name: string, data: unknown) => {
        emitted.push({ name, data });
        return Promise.resolve(false);
      }),
      __emitted: emitted,
      On: vi.fn((name: string, cb: (ev: unknown) => void) => {
        (handlers[name] ??= []).push(cb);
        // Real unsubscribe: React effect cleanups (component unmounts) must
        // remove dead handlers or emits leak across tests.
        return () => {
          handlers[name] = (handlers[name] ?? []).filter((h) => h !== cb);
        };
      }),
      __emit: (name: string, ev?: unknown) => (handlers[name] ?? []).forEach((cb) => cb(ev)),
    },
    Window: { Close: vi.fn(() => Promise.resolve()) },
    Call: { ByName: vi.fn(), ByID: vi.fn() },
  };
});

const LIVE_NOTE = {
  id: "n1",
  title: "标题",
  content: "有内容",
  pinned: false,
  deletedAt: null,
  createdAt: 1,
  updatedAt: 1,
};

function renderPinWindow() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <PinWindow noteId="n1" />
    </QueryClientProvider>,
  );
}

afterEach(cleanup);

beforeEach(() => {
  events.length = 0;
  emitted.length = 0;
  vi.clearAllMocks();
  mockDirty.mockReturnValue(false);
  // Re-register default mock implementations cleared by clearAllMocks.
  mockFlush.mockImplementation(async () => {
    events.push("flush");
  });
});

describe("PinWindow 关窗 / 删除流程", () => {
  it("× 关窗：先 flush，再空笔记判定，最后关窗（flush 顺序）", async () => {
    renderPinWindow();
    await waitFor(() => expect(screen.getByRole("button")).toBeTruthy());

    fireEvent.click(screen.getByRole("button"));

    await waitFor(() => expect(events).toEqual(["flush", "discardIfEmpty", "closePinnedWindow"]));
    expect(events).not.toContain("updateNote"); // debounced path never fired
  });

  it("flush 失败（仍 dirty）时不判空硬删，直接收起", async () => {
    mockDirty.mockReturnValue(true); // flush 后编辑器仍持有未落库内容
    renderPinWindow();
    await waitFor(() => expect(screen.getByRole("button")).toBeTruthy());

    fireEvent.click(screen.getByRole("button"));

    await waitFor(() => expect(events).toEqual(["flush", "closePinnedWindow"]));
    expect(events).not.toContain("discardIfEmpty");
    mockDirty.mockReturnValue(false);
  });

  it("⌘⌫：先 flush，再移入回收站，最后关窗", async () => {
    renderPinWindow();
    await waitFor(() => expect(screen.getByRole("button")).toBeTruthy());

    fireEvent.keyDown(window, { key: "Backspace", metaKey: true });

    await waitFor(() => expect(events).toEqual(["flush", "trashNote", "closePinnedWindow"]));
  });

  it("菜单栏兜底删除（pin:delete-requested）仅作用于指定笔记", async () => {
    renderPinWindow();
    await waitFor(() => expect(screen.getByRole("button")).toBeTruthy());

    const emit = (Events as unknown as { __emit: (name: string, ev?: unknown) => void }).__emit;
    // Addressed to a different window: ignored.
    emit("pin:delete-requested", { data: "other-note", sender: "pin-other-note" });
    expect(events).toEqual([]);

    // Addressed to this window: full flush → trash → close sequence.
    emit("pin:delete-requested", { data: "n1", sender: "pin-n1" });
    await waitFor(() => expect(events).toEqual(["flush", "trashNote", "closePinnedWindow"]));
  });
});

describe("PinWindow 更新屏障（pin:flush-requested）", () => {
  function emit(name: string, ev?: unknown) {
    (Events as unknown as { __emit: (n: string, e?: unknown) => void }).__emit(name, ev);
  }
  function acks() {
    return emitted.filter((e) => e.name === "pin:flushed");
  }

  it("干净落库后按 flush → 判空 → ack 回报 dirty=false", async () => {
    renderPinWindow();
    await waitFor(() => expect(screen.getByRole("button")).toBeTruthy());

    emit("pin:flush-requested", { data: { token: "t1" } });

    await waitFor(() => expect(events).toEqual(["flush", "discardIfEmpty"]));
    await waitFor(() =>
      expect(acks()).toEqual([{ name: "pin:flushed", data: { token: "t1", noteID: "n1", dirty: false } }]),
    );
  });

  // A note that still holds unsaved text must not be judged empty: the barrier
  // reads dirty=true and cancels the update round.
  it("flush 后仍 dirty 时不判空，ack 回报 dirty=true", async () => {
    mockDirty.mockReturnValue(true);
    renderPinWindow();
    await waitFor(() => expect(screen.getByRole("button")).toBeTruthy());

    emit("pin:flush-requested", { data: { token: "t2" } });

    await waitFor(() => expect(acks()).toEqual([{ name: "pin:flushed", data: { token: "t2", noteID: "n1", dirty: true } }]));
    expect(events).toEqual(["flush"]);
    expect(events).not.toContain("discardIfEmpty");
    mockDirty.mockReturnValue(false);
  });

  it("flush 抛错也必须有 ack，且按脏处理", async () => {
    mockFlush.mockRejectedValueOnce(new Error("db busy"));
    renderPinWindow();
    await waitFor(() => expect(screen.getByRole("button")).toBeTruthy());

    emit("pin:flush-requested", { data: { token: "t3" } });

    await waitFor(() => expect(acks()).toHaveLength(1));
    expect(acks()[0].data).toMatchObject({ token: "t3", dirty: true });
  });

  // 回收站占位视图没有编辑器：没有缓冲可言，不该让它一票否决更新。
  it("未挂编辑器（已删除占位）时回报 dirty=false", async () => {
    vi.mocked(api.listNotes).mockResolvedValue([]);
    vi.mocked(api.listTrash).mockResolvedValue([{ ...LIVE_NOTE, deletedAt: 100 }]);
    renderPinWindow();
    await waitFor(() => expect(screen.getByText("该笔记已被删除。")).toBeTruthy());

    emit("pin:flush-requested", { data: { token: "t4" } });

    await waitFor(() => expect(acks()).toHaveLength(1));
    expect(acks()[0].data).toMatchObject({ token: "t4", dirty: false });
    expect(events).toEqual([]);

    // clearAllMocks keeps implementations, so hand the defaults back.
    vi.mocked(api.listNotes).mockResolvedValue([LIVE_NOTE]);
    vi.mocked(api.listTrash).mockResolvedValue([]);
  });

  it("没有 token 时仍回报，缺字段不影响屏障", async () => {
    renderPinWindow();
    await waitFor(() => expect(screen.getByRole("button")).toBeTruthy());

    emit("pin:flush-requested", { data: null });

    await waitFor(() => expect(acks()).toHaveLength(1));
    expect(acks()[0].data).toMatchObject({ token: "", noteID: "n1", dirty: false });
  });
});

describe("PinWindow 生命周期", () => {
  it("笔记被彻底删除后窗口自动关闭", async () => {
    vi.mocked(api.listNotes).mockResolvedValue([]);
    renderPinWindow();

    await waitFor(() => expect(Window.Close).toHaveBeenCalledTimes(1));
  });

  it("回收站中的笔记显示已删除占位，不挂编辑器", async () => {
    vi.mocked(api.listNotes).mockResolvedValue([]);
    vi.mocked(api.listTrash).mockResolvedValue([{ ...LIVE_NOTE, deletedAt: 100 }]);
    renderPinWindow();

    await waitFor(() => expect(screen.getByText("该笔记已被删除。")).toBeTruthy());
    expect(events).toEqual([]);
  });
});
