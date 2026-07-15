import type { FinishedWork, FinishedWorkStatus, PrototypeRun, ScriptVariant, WorkbenchDraft } from "../../shared/types/generation";
import type { Product, SellingPoint } from "../../shared/types/product";
import { productReferenceImage } from "../products/product-reference";

type PrototypeStore = {
  runs: PrototypeRun[];
  finished_works: FinishedWork[];
};

type PersistedFinishedWork = Omit<FinishedWork, "status" | "progress" | "stage_label" | "completed_at"> & {
  status?: FinishedWorkStatus | "ready_to_submit" | "submitted";
  progress?: number;
  stage_label?: string;
  completed_at?: string;
  submitted_at?: string;
};

type PersistedPrototypeStore = {
  runs?: PrototypeRun[];
  finished_works?: PersistedFinishedWork[];
};

type GenerateScriptsInput = {
  product: Product;
  selling_points: SellingPoint[];
  custom_selling_points: string[];
  count: number;
};

const draftStorageKey = "aicut.workbench.prototype.draft.v1";
const storeStorageKey = "aicut.workbench.prototype.store.v1";
const runDurationMs = 9000;

const emptyDraft: WorkbenchDraft = {
  product_id: "",
  selling_point_ids: [],
  custom_selling_points: [],
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
    `有些小麻烦，真的会影响每天的使用感受。`,
    `真正好用的东西，不需要花时间反复适应。`,
    `别等到需要的时候，才发现准备得不够。`,
    `日常场景里，顺手比复杂更重要。`
  ];
  const closings = [
    `把细节做好，出门和使用都会更从容。`,
    `小小一件，刚好解决每天都会遇到的问题。`,
    `不用改变习惯，也能让体验更舒服。`,
    `需要的时候随手拿出来，就已经很够用。`
  ];
  const hook = `${primary}，从这一点开始更容易被看见`;
  const scriptText = `${openings[order % openings.length]}${input.product.name}围绕${primary}做了更直接的处理。使用时先感受到的是${primary}，接着会发现${secondary}也被兼顾到了。${closings[order % closings.length]}`;
  const textDuration = durationFromText(scriptText);
  const updatedAt = nowISO();

  return {
    id: createID("script"),
    order,
    hook,
    script_text: scriptText,
    estimated_duration_ms: textDuration,
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

function emptyStore(): PrototypeStore {
  return { runs: [], finished_works: [] };
}

function stageForElapsed(elapsedMs: number): Pick<PrototypeRun, "status" | "progress" | "stage_label"> {
  if (elapsedMs < 1800) {
    return { status: "preparing", progress: 18, stage_label: "准备素材约束" };
  }
  if (elapsedMs < 3900) {
    return { status: "voicing", progress: 42, stage_label: "生成口播" };
  }
  if (elapsedMs < 6500) {
    return { status: "planning", progress: 68, stage_label: "编排镜头" };
  }
  if (elapsedMs < runDurationMs) {
    return { status: "rendering", progress: 86, stage_label: "生成成品" };
  }
  return { status: "completed", progress: 100, stage_label: "已完成" };
}

function workFromRun(run: PrototypeRun, completedAt?: string): FinishedWork {
  const completed = run.status === "completed";
  return {
    id: createID("finished"),
    run_id: run.id,
    product_id: run.product_id,
    product_name: run.product_name,
    product_cover_url: run.product_cover_url,
    title: run.hook,
    hook: run.hook,
    script_text: run.script_text,
    duration_ms: run.duration_ms,
    status: completed ? "completed" : "generating",
    progress: run.progress,
    stage_label: run.stage_label,
    created_at: run.started_at,
    completed_at: completed ? completedAt ?? nowISO() : undefined
  };
}

function normalizeStore(value: PersistedPrototypeStore): { store: PrototypeStore; changed: boolean } {
  const runs = Array.isArray(value.runs) ? value.runs : [];
  let changed = !Array.isArray(value.runs) || !Array.isArray(value.finished_works);
  const finishedWorks = (Array.isArray(value.finished_works) ? value.finished_works : []).map((work) => {
    const completed = work.status !== "generating";
    const normalized: FinishedWork = {
      ...work,
      status: completed ? "completed" : "generating",
      progress: completed ? 100 : Math.max(0, Math.min(99, Number(work.progress) || 0)),
      stage_label: completed ? "已完成" : work.stage_label || "准备生成",
      created_at: work.created_at,
      completed_at: completed ? work.completed_at ?? work.submitted_at ?? work.created_at : undefined
    };
    if (
      work.status !== normalized.status ||
      work.progress !== normalized.progress ||
      work.stage_label !== normalized.stage_label ||
      work.completed_at !== normalized.completed_at ||
      "submitted_at" in work
    ) {
      changed = true;
    }
    return normalized;
  });
  return { store: { runs, finished_works: finishedWorks }, changed };
}

function reconcileStore(store: PrototypeStore) {
  const now = Date.now();
  let changed = false;
  const runs = store.runs.map((run) => {
    if (run.status === "completed") {
      return run;
    }
    const stage = stageForElapsed(now - new Date(run.started_at).getTime());
    if (stage.status === run.status && stage.progress === run.progress) {
      return run;
    }
    changed = true;
    return { ...run, ...stage };
  });
  const finishedWorks = store.finished_works.map((work) => {
    const run = runs.find((candidate) => candidate.id === work.run_id);
    if (!run) {
      return work;
    }
    const completed = run.status === "completed";
    const next: FinishedWork = {
      ...work,
      status: completed ? "completed" : "generating",
      progress: run.progress,
      stage_label: run.stage_label,
      completed_at: completed ? work.completed_at ?? nowISO() : undefined
    };
    if (
      next.status !== work.status ||
      next.progress !== work.progress ||
      next.stage_label !== work.stage_label ||
      next.completed_at !== work.completed_at
    ) {
      changed = true;
      return next;
    }
    return work;
  });
  for (const run of runs) {
    if (finishedWorks.some((work) => work.run_id === run.id)) {
      continue;
    }
    changed = true;
    finishedWorks.unshift(workFromRun(run));
  }
  return { store: { runs, finished_works: finishedWorks }, changed };
}

function readStore() {
  const saved = readJSON<PersistedPrototypeStore>(storeStorageKey, emptyStore());
  const normalized = normalizeStore(saved);
  const reconciled = reconcileStore(normalized.store);
  if (normalized.changed || reconciled.changed) {
    writeJSON(storeStorageKey, reconciled.store);
  }
  return reconciled.store;
}

export function loadWorkbenchDraft(): WorkbenchDraft {
  return readJSON<WorkbenchDraft>(draftStorageKey, emptyDraft);
}

export function saveWorkbenchDraft(draft: WorkbenchDraft) {
  writeJSON(draftStorageKey, draft);
}

export function clearWorkbenchVariants(draft: WorkbenchDraft): WorkbenchDraft {
  return { ...draft, variants: [], active_variant_id: "" };
}

export async function generatePrototypeScripts(input: GenerateScriptsInput): Promise<ScriptVariant[]> {
  await new Promise((resolve) => window.setTimeout(resolve, 650));
  return Array.from({ length: input.count }, (_, index) => generateVariant(input, index + 1));
}

export function startPrototypeWorks(product: Product, variants: ScriptVariant[]): FinishedWork[] {
  const store = readStore();
  const coverURL = productReferenceImage(product);
  const persistentCoverURL = coverURL.startsWith("data:") ? "" : coverURL;
  const startedAt = nowISO();
  const runs = variants.map((variant) => ({
    id: createID("run"),
    product_id: product.id,
    product_name: product.name,
    product_cover_url: persistentCoverURL || undefined,
    script_variant_id: variant.id,
    hook: variant.hook,
    script_text: variant.script_text,
    duration_ms: variant.estimated_duration_ms,
    status: "preparing" as const,
    progress: 8,
    stage_label: "准备素材约束",
    started_at: startedAt
  }));
  const works = runs.map((run) => workFromRun(run));
  writeJSON(storeStorageKey, {
    ...store,
    runs: [...runs, ...store.runs],
    finished_works: [...works, ...store.finished_works]
  });
  return works;
}

export function listPrototypeFinishedWorks(): FinishedWork[] {
  return readStore().finished_works;
}
