import type { Session } from "../types/auth";

const storedSessionKey = "aicut.session";

export function readStoredSession(): Session | null {
  try {
    const raw = window.localStorage.getItem(storedSessionKey);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as Session;
    return parsed?.token && parsed?.user?.role ? parsed : null;
  } catch {
    return null;
  }
}

export function storeSession(session: Session) {
  window.localStorage.setItem(storedSessionKey, JSON.stringify(session));
}

export function clearStoredSession() {
  window.localStorage.removeItem(storedSessionKey);
}
