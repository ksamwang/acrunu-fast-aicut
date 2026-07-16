import type { User } from "../shared/types/auth";

export type ViewKey = "workbench" | "finished" | "products" | "preprocess" | "assets" | "settings" | "users";

const defaultView: ViewKey = "workbench";
const viewKeys: ViewKey[] = ["workbench", "finished", "products", "preprocess", "assets", "settings", "users"];

function isViewKey(value: string): value is ViewKey {
  return viewKeys.includes(value as ViewKey);
}

export function normalizeViewForRole(view: ViewKey, role?: User["role"]): ViewKey {
  return (view === "settings" || view === "users") && role !== "admin" ? defaultView : view;
}

export function readHashView(role?: User["role"]): ViewKey {
  const raw = window.location.hash.replace(/^#\/?/, "").split(/[/?&]/)[0];
  return isViewKey(raw) ? normalizeViewForRole(raw, role) : defaultView;
}

export function writeHashView(view: ViewKey) {
  const nextHash = `#/${view}`;
  if (window.location.hash !== nextHash) {
    window.location.hash = nextHash;
  }
}

export function hashTargetsView(view: ViewKey) {
  const raw = window.location.hash.replace(/^#\/?/, "");
  return raw === view || raw.startsWith(`${view}/`) || raw.startsWith(`${view}?`);
}
