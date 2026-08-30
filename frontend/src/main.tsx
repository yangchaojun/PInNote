import { StrictMode, Suspense, lazy } from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ConfigProvider, Spin, unstableSetRender, theme as antdTheme } from "antd";
import { createRoot } from "react-dom/client";
import zhCN from "antd/locale/zh_CN";
// Inline of @ant-design/v5-patch-for-react-19: importing the package pulls the
// whole antd barrel into the bundle; a named import keeps tree-shaking.
unstableSetRender((node, container) => {
  const el = container as HTMLElement & { _reactRoot?: ReturnType<typeof createRoot> };
  el._reactRoot ||= createRoot(el);
  const root = el._reactRoot;
  root.render(node);
  return async () => {
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    root.unmount();
  };
});
import "./style.css";
import { App } from "./App";

// The pinned-note window UI is only rendered in windows created for a pinned
// note; the main window never loads its code.
const PinWindow = lazy(() => import("./PinWindow").then((m) => ({ default: m.PinWindow })));

const queryClient = new QueryClient({
  defaultOptions: {
    // The backend broadcasts notes:changed on every mutation, so refetching
    // on a timer or window focus is redundant work.
    queries: { staleTime: 30_000, refetchOnWindowFocus: false, retry: 1 },
  },
});

// Pinned note windows load "/#/pin/<noteId>"; the main window loads "/".
function RouteRoot() {
  const match = window.location.hash.match(/^#\/pin\/([0-9a-f]+)$/);
  if (match) {
    return (
      <Suspense fallback={<Spin className="pin-boot" />}>
        <PinWindow noteId={match[1]} />
      </Suspense>
    );
  }
  return <App />;
}

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ConfigProvider
        locale={zhCN}
        theme={{
          algorithm: antdTheme.darkAlgorithm,
          token: {
            colorPrimary: "#6f8ffa",
            borderRadius: 8,
            fontSize: 13,
          },
        }}
      >
        <RouteRoot />
      </ConfigProvider>
    </QueryClientProvider>
  </StrictMode>,
);
