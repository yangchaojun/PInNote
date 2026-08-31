import { create } from "zustand";
import { persist } from "zustand/middleware";
import { setTheme as setNativeTheme } from "./lib/api";

export type ThemeMode = "light" | "dark";

interface ThemeState {
  theme: ThemeMode;
  setTheme: (t: ThemeMode) => void;
  toggleTheme: () => void;
}

/** Reflects the theme on <html data-theme="..."> for the CSS variable switch. */
export function applyTheme(t: ThemeMode) {
  document.documentElement.dataset.theme = t;
  document.documentElement.style.colorScheme = t === "dark" ? "dark" : "light";
  // Mirror the theme onto the native window backdrops; no-op outside Wails.
  setNativeTheme(t).catch(() => undefined);
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set, get) => ({
      theme: "dark",
      setTheme: (theme) => {
        applyTheme(theme);
        set({ theme });
      },
      toggleTheme: () => get().setTheme(get().theme === "dark" ? "light" : "dark"),
    }),
    {
      name: "pinnote:theme",
      // Only the setting is persisted; applying happens via subscription below.
      partialize: (s) => ({ theme: s.theme }),
    },
  ),
);

applyTheme(useThemeStore.getState().theme);

// Pinned windows are separate webviews over the same localStorage: follow
// changes made in other windows. (setTheme writes the same value back, so
// this listener won't bounce between windows.)
window.addEventListener("storage", (e) => {
  if (e.key !== "pinnote:theme" || e.newValue == null) return;
  try {
    const theme = (JSON.parse(e.newValue)?.state?.theme ?? null) as ThemeMode | null;
    const cur = useThemeStore.getState();
    if (theme && theme !== cur.theme) cur.setTheme(theme);
  } catch {
    // Ignore malformed storage events.
  }
});
