import { useEffect, useState } from "react";
import { AppShell } from "./AppShell";
import { hashTargetsView, normalizeViewForRole, readHashView, writeHashView, type ViewKey } from "./routes";
import { AssetsPage } from "../features/assets/AssetsPage";
import { BGMLibraryPage } from "../features/bgm/BGMLibraryPage";
import { LoginPage } from "../features/auth/LoginPage";
import { FinishedLibraryPage } from "../features/finished/FinishedLibraryPage";
import { PreprocessPage } from "../features/preprocess/PreprocessPage";
import { ProductManagementPage } from "../features/products/ProductsPage";
import { SettingsPage } from "../features/settings/SettingsPage";
import { UsersPage } from "../features/users/UsersPage";
import { WorkbenchPage } from "../features/workbench/WorkbenchPage";
import { apiRequest } from "../shared/api/server-api";
import { roleLabels, translateValue } from "../shared/lib/labels";
import { clearStoredSession, readStoredSession, storeSession } from "../shared/lib/session-storage";
import type { Session, User } from "../shared/types/auth";

function ConsoleApp({ session, onLogout, onUserRefresh }: { session: Session; onLogout: () => void; onUserRefresh: (user: User) => void }) {
  const [view, setView] = useState<ViewKey>(() => readHashView(session.user.role));

  useEffect(() => {
    let cancelled = false;
    void apiRequest<User>("/api/auth/me", {}, session.token)
      .then((user) => {
        if (!cancelled) {
          onUserRefresh(user);
        }
      })
      .catch(() => {
        if (!cancelled) {
          onLogout();
        }
      });
    return () => {
      cancelled = true;
    };
  }, [session.token]);

  useEffect(() => {
    const syncViewFromHash = () => {
      const nextView = readHashView(session.user.role);
      setView(nextView);
      if (!hashTargetsView(nextView)) {
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
      {view === "workbench" && <WorkbenchPage token={session.token} />}
      {view === "finished" && <FinishedLibraryPage token={session.token} />}
      {view === "products" && <ProductManagementPage token={session.token} />}
      {view === "preprocess" && <PreprocessPage token={session.token} />}
      {view === "assets" && <AssetsPage token={session.token} />}
      {view === "bgm" && <BGMLibraryPage token={session.token} />}
      {view === "settings" && session.user.role === "admin" && <SettingsPage token={session.token} />}
      {view === "users" && session.user.role === "admin" && <UsersPage token={session.token} currentUser={session.user} />}
    </AppShell>
  );
}

export function App() {
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

  const handleUserRefresh = (user: User) => {
    setSession((current) => {
      if (!current) {
        return current;
      }
      if (
        current.user.id === user.id &&
        current.user.username === user.username &&
        current.user.display_name === user.display_name &&
        current.user.role === user.role
      ) {
        return current;
      }
      const next = { ...current, user };
      storeSession(next);
      return next;
    });
  };

  return session ? (
    <ConsoleApp session={session} onLogout={handleLogout} onUserRefresh={handleUserRefresh} />
  ) : (
    <LoginPage onLogin={handleLogin} />
  );
}
