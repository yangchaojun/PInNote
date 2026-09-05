import { Events } from "@wailsio/runtime";
import { useSyncExternalStore } from "react";
import { getTheme, setTheme as setThemeOnGo } from "./lib/api";

export type ThemeMode = "light" | "dark";

/**
 * The Go side is the single source of truth for the theme (settings table,
 * WindowService.GetTheme/SetTheme). This module only mirrors it onto the DOM
 * and re-renders antd's ConfigProvider. SetTheme on Go persists, repaints the
 * native window backdrops, and broadcasts theme:changed — every window
 * (including the caller) switches from that single broadcast, so all pin
 * windows and their native backdrops change in the same frame.
 */

let current: ThemeMode = "dark";
const listeners = new Set<() => void>();

function applyTheme(t: ThemeMode) {
  current = t;
  document.documentElement.dataset.theme = t;
  document.documentElement.style.colorScheme = t === "dark" ? "dark" : "light";
  listeners.forEach((l) => l());
}

export function useTheme(): ThemeMode {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => current,
  );
}

/** Reads the persisted theme once at startup. Falls back to dark outside Wails. */
export async function initTheme(): Promise<void> {
  try {
    const t = await getTheme();
    if (t === "light" || t === "dark") applyTheme(t);
  } catch {
    applyTheme("dark");
  }
}

export async function toggleTheme(): Promise<void> {
  const next: ThemeMode = current === "dark" ? "light" : "dark";
  try {
    await setThemeOnGo(next);
    // The theme:changed broadcast applies it; this is just a fast path.
    applyTheme(next);
  } catch {
    // Plain browser dev (no Wails runtime): still switch the DOM.
    applyTheme(next);
  }
}

// Follow theme changes made in other windows (or the menu bar).
Events.On("theme:changed", (ev) => {
  // Go Emit with a single argument delivers the value itself; be tolerant of
  // a wrapped array just in case.
  const d = ev.data as unknown;
  const mode = Array.isArray(d) ? d[0] : d;
  if (mode === "light" || mode === "dark") applyTheme(mode);
});
