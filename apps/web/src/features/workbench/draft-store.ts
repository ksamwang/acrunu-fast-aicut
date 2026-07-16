import type { WorkbenchDraft } from "../../shared/types/generation";

const draftStorageKey = "aicut.workbench.draft.v1";
const legacyDraftStorageKey = "aicut.workbench.prototype.draft.v1";

const emptyDraft: WorkbenchDraft = {
  product_id: "",
  selling_point_ids: [],
  custom_selling_points: [],
  voice_profile_id: "",
  variant_count: 3,
  variants: [],
  active_variant_id: ""
};

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function readJSON<T>(key: string, fallback: T): T {
  try {
    const value = window.localStorage.getItem(key);
    return value ? (JSON.parse(value) as T) : clone(fallback);
  } catch {
    return clone(fallback);
  }
}

function writeJSON<T>(key: string, value: T) {
  window.localStorage.setItem(key, JSON.stringify(value));
}

export function loadWorkbenchDraft(): WorkbenchDraft {
  const hasCurrentDraft = window.localStorage.getItem(draftStorageKey) !== null;
  const sourceKey = hasCurrentDraft ? draftStorageKey : legacyDraftStorageKey;
  const draft = { ...emptyDraft, ...readJSON<Partial<WorkbenchDraft>>(sourceKey, emptyDraft) };
  if (!hasCurrentDraft && window.localStorage.getItem(legacyDraftStorageKey) !== null) {
    writeJSON(draftStorageKey, draft);
  }
  return draft;
}

export function saveWorkbenchDraft(draft: WorkbenchDraft) {
  writeJSON(draftStorageKey, draft);
}

export function clearWorkbenchVariants(draft: WorkbenchDraft): WorkbenchDraft {
  return { ...draft, variants: [], active_variant_id: "" };
}
