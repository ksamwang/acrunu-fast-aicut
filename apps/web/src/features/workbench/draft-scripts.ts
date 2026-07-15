import type { ScriptVariant, WorkbenchDraft } from "../../shared/types/generation";
import type { Product, SellingPoint } from "../../shared/types/product";

type GenerateScriptsInput = {
  product: Product;
  selling_points: SellingPoint[];
  custom_selling_points: string[];
  count: number;
};

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

function nowISO() {
  return new Date().toISOString();
}

function createID(prefix: string) {
  return `${prefix}-${crypto.randomUUID()}`;
}

function pointNames(input: GenerateScriptsInput) {
  return [...input.selling_points.map((point) => point.title.trim()), ...input.custom_selling_points.map((point) => point.trim())].filter(Boolean);
}

function durationFromText(text: string) {
  return Math.max(8000, Math.round((text.replace(/\s/g, "").length / 4.2) * 1000));
}

function generateVariant(input: GenerateScriptsInput, order: number): ScriptVariant {
  const points = pointNames(input);
  const primary = points[order % Math.max(points.length, 1)] || "日常使用体验";
  const secondary = points[(order + 1) % Math.max(points.length, 1)] || primary;
  const openings = [
    "有些小麻烦，真的会影响每天的使用感受。",
    "真正好用的东西，不需要花时间反复适应。",
    "别等到需要的时候，才发现准备得不够。",
    "日常场景里，顺手比复杂更重要。"
  ];
  const closings = [
    "把细节做好，出门和使用都会更从容。",
    "小小一件，刚好解决每天都会遇到的问题。",
    "不用改变习惯，也能让体验更舒服。",
    "需要的时候随手拿出来，就已经很够用。"
  ];
  const hook = `${primary}，从这一点开始更容易被看见`;
  const scriptText = `${openings[order % openings.length]}${input.product.name}围绕${primary}做了更直接的处理。使用时先感受到的是${primary}，接着会发现${secondary}也被兼顾到了。${closings[order % closings.length]}`;
  const updatedAt = nowISO();

  return {
    id: createID("script"),
    order,
    hook,
    script_text: scriptText,
    estimated_duration_ms: durationFromText(scriptText),
    editing_intent: `用一个真实使用瞬间建立关注，再依次让画面回应“${primary}”和“${secondary}”，最后以轻量收束完成记忆点。`,
    beats: [
      {
        id: createID("beat"),
        label: "开头",
        selling_point: primary,
        visual_goal: "用人物、场景或结果画面快速建立问题与关注点。",
        source_type: "mixed"
      },
      {
        id: createID("beat"),
        label: "展示",
        selling_point: primary,
        visual_goal: "用产品特写和实际动作说明核心卖点。",
        source_type: "visual_only"
      },
      {
        id: createID("beat"),
        label: "补充",
        selling_point: secondary,
        visual_goal: "补充使用结果或细节，增加可信度。",
        source_type: "mixed"
      },
      {
        id: createID("beat"),
        label: "收束",
        selling_point: "日常使用",
        visual_goal: "用轻量产品画面或使用后的状态完成收束。",
        source_type: "visual_only"
      }
    ],
    status: "draft",
    updated_at: updatedAt
  };
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

// Temporary adapter until the server-side LLM script generator is connected.
export async function generateDraftScripts(input: GenerateScriptsInput): Promise<ScriptVariant[]> {
  await new Promise((resolve) => window.setTimeout(resolve, 650));
  return Array.from({ length: input.count }, (_, index) => generateVariant(input, index + 1));
}
