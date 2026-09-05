import { StrictMode } from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ConfigProvider, theme as antdTheme } from "antd";
import zhCN from "antd/locale/zh_CN";
import "./style.css";
import { PinWindow } from "./PinWindow";
import { initTheme, useTheme } from "./theme";

// The pin window is the app's only UI: every route is a pinned note.
const queryClient = new QueryClient({
  defaultOptions: {
    // The backend broadcasts notes:changed on every mutation, so refetching
    // on a timer or window focus is redundant work.
    queries: { staleTime: 30_000, refetchOnWindowFocus: false, retry: 1 },
  },
});

function Root() {
  const theme = useTheme();
  const match = window.location.hash.match(/^#\/pin\/([0-9a-f]+)$/);
  if (!match) return null;
  return (
    <ConfigProvider
      locale={zhCN}
      theme={{
        algorithm: theme === "dark" ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        token: {
          colorPrimary: "#6f8ffa",
          borderRadius: 8,
          fontSize: 13,
        },
      }}
    >
      <PinWindow noteId={match[1]} />
    </ConfigProvider>
  );
}

// The persisted theme (Go settings table) applies before first paint.
void initTheme().then(() => {
  ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <Root />
      </QueryClientProvider>
    </StrictMode>,
  );
});
