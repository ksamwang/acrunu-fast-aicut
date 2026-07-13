import type { User } from "../shared/types/auth";

export type ViewKey = "products" | "preprocess" | "assets" | "tasks" | "settings";

const defaultView: ViewKey = "products";
const viewKeys: ViewKey[] = ["products", "preprocess", "assets", "tasks", "settings"];

function isViewKey(value: string): value is ViewKey {
  return viewKeys.includes(value as ViewKey);
}

export function normalizeViewForRole(view: ViewKey, role?: User["role"]): ViewKey {
  return view === "settings" && role !== "admin" ? defaultView : view;
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
