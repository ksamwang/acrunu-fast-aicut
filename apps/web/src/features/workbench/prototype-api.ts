import type {
  EditPlanBeat,
  EditingIntentBeat,
  FinishedWork,
  FinishedWorkStatus,
  NarrationSegment,
  PrototypeRun,
  ScriptVariant,
  WorkbenchDraft
} from "../../shared/types/generation";
import type { Product, SellingPoint } from "../../shared/types/product";
import type { VoiceProfile } from "../../shared/types/voice";
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

function defaultEditingIntent(productName: string) {
  return `以真实使用情境建立关注，再用产品动作和结果画面回应“${productName}”的核心价值。`;
}

function defaultBeats(productName: string): EditingIntentBeat[] {
  return [
    {
      id: createID("fallback-beat"),
      label: "开头",
      selling_point: "使用痛点",
      visual_goal: "用真实场景快速建立需要解决的问题。",
      source_type: "mixed"
    },
    {
      id: createID("fallback-beat"),
      label: "展示",
      selling_point: productName,
      visual_goal: "用产品特写和实际动作说明核心用法。",
      source_type: "visual_only"
    },
    {
      id: createID("fallback-beat"),
      label: "结果",
      selling_point: "使用体验",
      visual_goal: "展示使用后的状态与直观结果。",
      source_type: "mixed"
    }
  ];
}

function createNarrationSegments(scriptText: string, durationMs: number): NarrationSegment[] {
  const fragments = (scriptText.match(/[^。！？!?]+[。！？!?]?/g) ?? [])
    .map((fragment) => fragment.trim())
    .filter(Boolean);
  const lines = fragments.length > 0 ? fragments : [scriptText.trim() || "旁白内容待生成"];
  const totalWeight = lines.reduce((sum, line) => sum + Math.max(line.length, 1), 0);
  let cursor = 0;

  return lines.map((text, index) => {
    const remainingCount = lines.length - index - 1;
    const remainingDuration = Math.max(0, durationMs - cursor);
    const reservedDuration = remainingCount * 620;
    const idealDuration = Math.round((durationMs * Math.max(text.length, 1)) / totalWeight);
    const segmentDuration = index === lines.length - 1
      ? remainingDuration
      : Math.max(620, Math.min(idealDuration, Math.max(620, remainingDuration - reservedDuration)));
    const endMs = Math.min(durationMs, cursor + segmentDuration);
    const segment: NarrationSegment = {
      id: createID("narration"),
      start_ms: cursor,
      end_ms: endMs,
      text
    };
    cursor = endMs;
    return segment;
  });
}

function createEditPlan(beats: EditingIntentBeat[], durationMs: number): EditPlanBeat[] {
  const sliceDuration = Math.max(1, Math.round(durationMs / beats.length));
  return beats.map((beat, index) => ({
    id: createID("edit-plan"),
    start_ms: index * sliceDuration,
    end_ms: index === beats.length - 1 ? durationMs : Math.min(durationMs, (index + 1) * sliceDuration),
    label: beat.label,
    visual_goal: beat.visual_goal,
    source_type: beat.source_type
  }));
}

function withPrototypeDetail(work: FinishedWork): FinishedWork {
  const beats = work.edit_plan?.length
    ? undefined
    : defaultBeats(work.product_name);
  return {
    ...work,
    editing_intent: work.editing_intent || defaultEditingIntent(work.product_name),
    narration_segments: work.narration_segments?.length
      ? work.narration_segments
      : createNarrationSegments(work.script_text, work.duration_ms),
    edit_plan: work.edit_plan?.length ? work.edit_plan : createEditPlan(beats ?? defaultBeats(work.product_name), work.duration_ms)
  };
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

function createDemoWorks(): FinishedWork[] {
  const demos = [
    {
      id: "demo-finished-cuff-strap-01",
      title: "裤脚不蹭链条，骑行更利落",
      hook: "骑车时，裤脚总蹭链条怎么办？",
      scriptText: "骑车的时候最怕裤脚蹭到链条。把束裤带绕住裤脚，一贴就稳，骑行时更利落。大面积反光条，晚上出门也更安心。",
      durationMs: 22600,
      createdAt: "2026-07-15T08:40:00.000Z",
      completedAt: "2026-07-15T08:42:00.000Z",
      editingIntent: "先以裤脚蹭链条的骑行痛点切入，再展示束裤带的一贴固定和夜间反光效果。",
      beats: [
        { id: "demo-beat-01", label: "痛点", selling_point: "裤脚蹭链条", visual_goal: "用骑行中的裤脚和链条建立真实痛点。", source_type: "mixed" as const },
        { id: "demo-beat-02", label: "固定", selling_point: "一贴就稳", visual_goal: "近景展示绕住裤脚和贴合动作。", source_type: "visual_only" as const },
        { id: "demo-beat-03", label: "结果", selling_point: "骑行更利落", visual_goal: "展示整理后的骑行状态和夜间反光细节。", source_type: "mixed" as const }
      ]
    },
    {
      id: "demo-finished-cuff-strap-02",
      title: "一条束裤带，收好骑行细节",
      hook: "出门骑车，口袋里放这一条就够了。",
      scriptText: "它不只用来绑裤脚，水壶和修车工具也能随手固定。高弹力设计不会勒腿，收起来轻巧不占地方。需要的时候拿出来，骑行准备就更从容。",
      durationMs: 20400,
      createdAt: "2026-07-14T08:40:00.000Z",
      completedAt: "2026-07-14T08:42:00.000Z",
      editingIntent: "通过口袋收纳和多个使用场景，强调一条束裤带的轻量与多用途。",
      beats: [
        { id: "demo-beat-04", label: "收纳", selling_point: "轻巧不占地方", visual_goal: "从口袋中取出产品，建立轻量感。", source_type: "visual_only" as const },
        { id: "demo-beat-05", label: "多用", selling_point: "多个固定场景", visual_goal: "依次展示裤脚、水壶和工具固定动作。", source_type: "mixed" as const },
        { id: "demo-beat-06", label: "收束", selling_point: "骑行更从容", visual_goal: "以准备完成后出发的画面收束。", source_type: "visual_only" as const }
      ]
    }
  ];

  return demos.map((demo) => ({
    id: demo.id,
    run_id: demo.id,
    product_id: "demo-cuff-strap",
    product_name: "束裤带",
    title: demo.title,
    hook: demo.hook,
    voice_profile_id: "voice-prototype-warm-female",
    voice_profile_name: "温和女声",
    script_text: demo.scriptText,
    duration_ms: demo.durationMs,
    status: "completed" as const,
    progress: 100,
    stage_label: "已完成",
    created_at: demo.createdAt,
    completed_at: demo.completedAt,
    editing_intent: demo.editingIntent,
    narration_segments: createNarrationSegments(demo.scriptText, demo.durationMs),
    edit_plan: createEditPlan(demo.beats, demo.durationMs),
    is_demo: true
  }));
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
  const beats = run.beats?.length ? run.beats : defaultBeats(run.product_name);
  return {
    id: createID("finished"),
    run_id: run.id,
    product_id: run.product_id,
    product_name: run.product_name,
    product_cover_url: run.product_cover_url,
    title: run.hook,
    hook: run.hook,
    voice_profile_id: run.voice_profile_id,
    voice_profile_name: run.voice_profile_name,
    script_text: run.script_text,
    duration_ms: run.duration_ms,
    status: completed ? "completed" : "generating",
    progress: run.progress,
    stage_label: run.stage_label,
    created_at: run.started_at,
    completed_at: completed ? completedAt ?? nowISO() : undefined,
    editing_intent: run.editing_intent || defaultEditingIntent(run.product_name),
    narration_segments: createNarrationSegments(run.script_text, run.duration_ms),
    edit_plan: createEditPlan(beats, run.duration_ms)
  };
}

function normalizeStore(value: PersistedPrototypeStore): { store: PrototypeStore; changed: boolean } {
  const runs = Array.isArray(value.runs) ? value.runs : [];
  let changed = !Array.isArray(value.runs) || !Array.isArray(value.finished_works);
  const finishedWorks = (Array.isArray(value.finished_works) ? value.finished_works : []).map((work) => {
    const completed = work.status !== "generating";
    const normalized = withPrototypeDetail({
      ...work,
      status: completed ? "completed" : "generating",
      progress: completed ? 100 : Math.max(0, Math.min(99, Number(work.progress) || 0)),
      stage_label: completed ? "已完成" : work.stage_label || "准备生成",
      created_at: work.created_at,
      completed_at: completed ? work.completed_at ?? work.submitted_at ?? work.created_at : undefined
    });
    if (
      work.status !== normalized.status ||
      work.progress !== normalized.progress ||
      work.stage_label !== normalized.stage_label ||
      work.completed_at !== normalized.completed_at ||
      !work.editing_intent ||
      !work.narration_segments?.length ||
      !work.edit_plan?.length ||
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
  const shouldSeedDemo = normalized.store.runs.length === 0 && normalized.store.finished_works.length === 0;
  const seededStore = shouldSeedDemo
    ? { ...normalized.store, finished_works: createDemoWorks() }
    : normalized.store;
  const reconciled = reconcileStore(seededStore);
  if (normalized.changed || shouldSeedDemo || reconciled.changed) {
    writeJSON(storeStorageKey, reconciled.store);
  }
  return reconciled.store;
}

export function loadWorkbenchDraft(): WorkbenchDraft {
  return { ...emptyDraft, ...readJSON<Partial<WorkbenchDraft>>(draftStorageKey, emptyDraft) };
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

export function startPrototypeWorks(product: Product, variants: ScriptVariant[], voiceProfile: VoiceProfile): FinishedWork[] {
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
    voice_profile_id: voiceProfile.id,
    voice_profile_name: voiceProfile.name,
    hook: variant.hook,
    script_text: variant.script_text,
    duration_ms: variant.estimated_duration_ms,
    status: "preparing" as const,
    progress: 8,
    stage_label: "准备素材约束",
    started_at: startedAt,
    editing_intent: variant.editing_intent,
    beats: variant.beats
  }));
  const works = runs.map((run) => workFromRun(run));
  const existingWorks = store.finished_works.filter((work) => !work.is_demo);
  writeJSON(storeStorageKey, {
    ...store,
    runs: [...runs, ...store.runs],
    finished_works: [...works, ...existingWorks]
  });
  return works;
}

export function listPrototypeFinishedWorks(): FinishedWork[] {
  return readStore().finished_works;
}
