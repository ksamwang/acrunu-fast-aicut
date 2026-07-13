import React, { useEffect, useState } from "react";
import ReactDOM from "react-dom/client";
import { ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { AppShell } from "./app/AppShell";
import { normalizeViewForRole, readHashView, writeHashView, type ViewKey } from "./app/routes";
import { AssetsPage } from "./features/assets/AssetsPage";
import { LoginPage } from "./features/auth/LoginPage";
import { PreprocessPage } from "./features/preprocess/PreprocessPage";
import { ProductManagementPage } from "./features/products/ProductsPage";
import { SettingsPage } from "./features/settings/SettingsPage";
import { TasksPage } from "./features/tasks/TasksPage";
import { roleLabels, translateValue } from "./shared/lib/labels";
import { clearStoredSession, readStoredSession, storeSession } from "./shared/lib/session-storage";
import type { Session } from "./shared/types/auth";
import "./styles.css";

function ConsoleApp({ session, onLogout }: { session: Session; onLogout: () => void }) {
  const [view, setView] = useState<ViewKey>(() => readHashView(session.user.role));

  useEffect(() => {
    const syncViewFromHash = () => {
      const nextView = readHashView(session.user.role);
      setView(nextView);
      if (window.location.hash !== `#/${nextView}`) {
        writeHashView(nextView);
      }
    };

    syncViewFromHash();
    window.addEventListener("hashchange", syncViewFromHash);
    return () => window.removeEventListener("hashchange", syncViewFromHash);
  }, [session.user.role]);

  const navigateView = (next: ViewKey) => {
    const normalized = normalizeViewForRole(next, session.user.role);
    setView(normalized);
    writeHashView(normalized);
  };

  return (
    <AppShell
      user={session.user}
      view={view}
      roleLabel={translateValue(session.user.role, roleLabels)}
      onNavigate={navigateView}
      onLogout={onLogout}
    >
      {view === "products" && <ProductManagementPage token={session.token} />}
      {view === "preprocess" && <PreprocessPage token={session.token} />}
      {view === "assets" && <AssetsPage token={session.token} />}
      {view === "tasks" && <TasksPage token={session.token} />}
      {view === "settings" && session.user.role === "admin" && <SettingsPage token={session.token} />}
    </AppShell>
  );
}

function App() {
  const [session, setSession] = useState<Session | null>(() => readStoredSession());

  const handleLogin = (nextSession: Session) => {
    storeSession(nextSession);
    setSession(nextSession);
    const nextView = readHashView(nextSession.user.role);
    writeHashView(nextView);
  };

  const handleLogout = () => {
    clearStoredSession();
    setSession(null);
  };

  return session ? (
    <ConsoleApp session={session} onLogout={handleLogout} />
  ) : (
    <LoginPage onLogin={handleLogin} />
  );
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN}>
      <App />
    </ConfigProvider>
  </React.StrictMode>
);
