import type { ScriptTargetDuration, ScriptVariant, WorkbenchDraft } from "../../shared/types/generation";

const draftStorageKey = "aicut.workbench.draft.v1";
const legacyDraftStorageKey = "aicut.workbench.prototype.draft.v1";

const emptyDraft: WorkbenchDraft = {
  product_id: "",
  selling_point_ids: [],
  custom_selling_points: [],
  voice_profile_id: "",
  output_ratio: "9:16",
  subtitle_preset_id: "",
  variant_count: 3,
  target_duration_seconds: 30,
  temperature: 0.75,
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
  draft.target_duration_seconds = normalizeTargetDuration(draft.target_duration_seconds);
  draft.temperature = normalizeTemperature(draft.temperature);
  draft.variants = (draft.variants ?? []).map(normalizeVariant);
  if (!draft.script_generation?.job_id || !draft.script_generation.base_revision) {
    delete draft.script_generation;
  }
  if (!hasCurrentDraft && window.localStorage.getItem(legacyDraftStorageKey) !== null) {
    writeJSON(draftStorageKey, draft);
  }
  return draft;
}

function normalizeTargetDuration(value: number): ScriptTargetDuration {
  return value === 15 || value === 20 || value === 30 || value === 45 || value === 60 ? value : 30;
}

function normalizeTemperature(value: number): number {
  return Number.isFinite(value) && value >= 0 && value <= 2 ? value : 0.75;
}

function normalizeVariant(variant: ScriptVariant): ScriptVariant {
  const gain = Number(variant.bgm?.gain_db);
  const mode = variant.bgm?.mode;
  return {
    ...variant,
    origin: variant.origin === "imported" ? "imported" : "generated",
    bgm: {
      mode: mode === "track" || mode === "none" ? mode : "random",
      track_id: variant.bgm?.track_id ?? "",
      gain_db: Number.isFinite(gain) && gain >= -30 && gain <= 0 ? gain : -12
    }
  };
}

export function saveWorkbenchDraft(draft: WorkbenchDraft) {
  writeJSON(draftStorageKey, draft);
}

export function clearWorkbenchVariants(draft: WorkbenchDraft): WorkbenchDraft {
  return { ...draft, variants: [], active_variant_id: "" };
}
