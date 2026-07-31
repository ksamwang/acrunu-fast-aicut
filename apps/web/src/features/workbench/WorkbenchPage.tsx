import { useEffect, useMemo, useRef, useState } from "react";
import { Button, Empty, Input, InputNumber, Modal, Popover, Segmented, Select, Slider, Tag, Tooltip, Typography, message } from "antd";
import { Captions, Check, CheckCircle2, Circle, Clapperboard, Copy, FileUp, ListChecks, Music2, Pause, Play, Plus, RefreshCw, RotateCcw, Sparkles, Volume2, X } from "lucide-react";
import { useResource } from "../../shared/hooks/use-resource";
import { formatDuration } from "../../shared/lib/format";
import { createUUID } from "../../shared/lib/uuid";
import type { ScriptGenerationJob, ScriptGenerationJobInput, ScriptGenerationJobMode, ScriptTargetDuration, ScriptVariant, WorkbenchDraft } from "../../shared/types/generation";
import type { BGMSelection, BGMTrack } from "../../shared/types/bgm";
import type { Product, SellingPoint } from "../../shared/types/product";
import type { VoiceAudition } from "../../shared/types/voice";
import type { OutputRatio, SubtitleStylePreset } from "../../shared/types/subtitle";
import { listProducts, listSellingPoints } from "../products/api";
import { listBGMTracks } from "../bgm/api";
import { VoiceProfilePicker } from "../voice-profiles/VoiceProfilePicker";
import { listSubtitleStylePresets } from "../subtitles/api";
import { SubtitleStylePreview } from "../subtitles/SubtitleStylePreview";
import { useVoiceProfiles } from "../voice-profiles/useVoiceProfiles";
import {
  clearWorkbenchVariants,
  loadWorkbenchDraft,
  saveWorkbenchDraft
} from "./draft-store";
import {
  cancelScriptGenerationJob,
  createScriptGenerationJob,
  createVoiceAudition,
  createVoiceoverTasks,
  getLatestScriptGenerationJob,
  getScriptGenerationJob,
  getVoiceAudition,
  resolveScriptGenerationJob
} from "./api";
import { ScriptImportModal } from "./ScriptImportModal";
import { deriveScriptHook, maxWorkbenchScripts, type ImportedScript } from "./script-import";
import "./styles.css";

const sourceTypeLabels = {
  visual_only: "纯画面",
  talking_head: "口播",
  mixed: "混合"
};

function estimateDuration(text: string) {
  let spokenCharacters = 0;
  let pauseMs = 0;
  for (const character of text) {
    if (!/[\p{White_Space}\p{Punctuation}\p{Symbol}]/u.test(character)) {
      spokenCharacters += 1;
    }
    if (/[。！？.!?；;]/u.test(character)) {
      pauseMs += 260;
    } else if (/[，,、：:]/u.test(character)) {
      pauseMs += 140;
    }
  }
  return Math.max(8000, Math.round((spokenCharacters / 5) * 1000) + pauseMs);
}

function activeSellingPoints(points: SellingPoint[]) {
  return points.filter((point) => point.status !== "archived");
}

function workbenchDraftRevision(draft: WorkbenchDraft) {
  const serialized = JSON.stringify({
    product_id: draft.product_id,
    selling_point_ids: draft.selling_point_ids,
    custom_selling_points: draft.custom_selling_points,
    variant_count: draft.variant_count,
    target_duration_seconds: draft.target_duration_seconds,
    temperature: draft.temperature,
    variants: draft.variants.map((variant) => ({
      id: variant.id,
      order: variant.order,
      hook: variant.hook,
      script_text: variant.script_text,
      editing_intent: variant.editing_intent,
      beats: variant.beats,
      status: variant.status,
      bgm: variant.bgm,
      updated_at: variant.updated_at
    }))
  });
  let hash = 2166136261;
  for (let index = 0; index < serialized.length; index += 1) {
    hash ^= serialized.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return `v1-${(hash >>> 0).toString(16).padStart(8, "0")}`;
}

function withoutScriptGeneration(draft: WorkbenchDraft): WorkbenchDraft {
  const next = { ...draft };
  delete next.script_generation;
  return next;
}

function applyScriptGenerationResult(
  draft: WorkbenchDraft,
  job: ScriptGenerationJob,
  strategy: "replace" | "append",
  defaultBGM: BGMSelection
): WorkbenchDraft {
  const generated = (job.result_variants ?? []).map<ScriptVariant>((variant) => ({
    ...variant,
    origin: "generated",
    bgm: { ...defaultBGM }
  }));
  if (generated.length === 0) {
    throw new Error("文案生成结果为空");
  }

  let variants: ScriptVariant[];
  let activeVariantID = draft.active_variant_id;
  if (strategy === "append") {
    variants = [...draft.variants, ...generated]
      .slice(0, maxWorkbenchScripts)
      .map((variant, index) => ({ ...variant, order: index + 1 }));
    activeVariantID = generated.find((variant) => variants.some((item) => item.id === variant.id))?.id ?? activeVariantID;
  } else if (job.mode === "replace_variant") {
    const replacement = generated[0];
    const targetIndex = draft.variants.findIndex((variant) => variant.id === job.target_variant_id);
    if (targetIndex >= 0) {
      const target = draft.variants[targetIndex];
      variants = draft.variants.map((variant, index) => index === targetIndex ? {
        ...replacement,
        id: target.id,
        order: target.order,
        bgm: target.bgm,
        status: "draft"
      } : variant);
      activeVariantID = target.id;
    } else {
      variants = [...draft.variants, replacement]
        .slice(0, maxWorkbenchScripts)
        .map((variant, index) => ({ ...variant, order: index + 1 }));
      activeVariantID = replacement.id;
    }
  } else {
    variants = generated.map((variant, index) => ({ ...variant, order: index + 1 }));
    activeVariantID = variants[0]?.id ?? "";
  }

  return {
    ...draft,
    product_id: strategy === "replace" ? job.product_id : draft.product_id,
    selling_point_ids: strategy === "replace" ? [...job.input.selling_point_ids] : draft.selling_point_ids,
    custom_selling_points: strategy === "replace" ? [...job.input.custom_selling_points] : draft.custom_selling_points,
    variant_count: strategy === "replace" && job.mode === "replace_all" ? job.input.variant_count : draft.variant_count,
    target_duration_seconds: strategy === "replace" ? (job.input.target_duration_seconds ?? 30) : draft.target_duration_seconds,
    temperature: strategy === "replace" ? (job.input.temperature ?? 0.75) : draft.temperature,
    variants,
    active_variant_id: activeVariantID,
    script_generation: {
      job_id: job.id,
      base_revision: job.base_revision,
      applied_locally: true
    }
  };
}

export function WorkbenchPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token, [], listProducts);
  const subtitlePresetsResource = useResource<SubtitleStylePreset[]>("/api/subtitle-presets", token, [], listSubtitleStylePresets);
  const [draft, setDraft] = useState<WorkbenchDraft>(() => loadWorkbenchDraft());
  const [customSellingPointInput, setCustomSellingPointInput] = useState("");
  const [scriptGenerationJob, setScriptGenerationJob] = useState<ScriptGenerationJob | null>(null);
  const [creatingScriptGeneration, setCreatingScriptGeneration] = useState<{ mode: ScriptGenerationJobMode; target_variant_id: string } | null>(null);
  const [scriptGenerationAction, setScriptGenerationAction] = useState<"cancel" | "resolve" | "retry" | null>(null);
  const [startingTasks, setStartingTasks] = useState(false);
  const [audition, setAudition] = useState<VoiceAudition | null>(null);
  const [creatingAudition, setCreatingAudition] = useState(false);
  const [playingBGM, setPlayingBGM] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const bgmAudioRef = useRef<HTMLAudioElement | null>(null);
  const draftRef = useRef(draft);
  const handlingScriptGenerationJobsRef = useRef(new Set<string>());
  const defaultedProductIDRef = useRef("");
  const voiceProfilesResource = useVoiceProfiles(token);
  const bgmTracksResource = useResource<BGMTrack[]>("/api/bgm-tracks", token, [], listBGMTracks);

  const sellingPoints = useResource<SellingPoint[]>(
    draft.product_id ? `/api/products/${draft.product_id}/selling-points` : null,
    token,
    [draft.product_id],
    listSellingPoints
  );

  const productList = products.data ?? [];
  const selectedProduct = productList.find((product) => product.id === draft.product_id) ?? null;
  const availableSellingPoints = activeSellingPoints(sellingPoints.data ?? []).filter((point) => point.product_id === draft.product_id);
  const selectedSellingPoints = availableSellingPoints.filter((point) => draft.selling_point_ids.includes(point.id));
  const activeVariant = draft.variants.find((variant) => variant.id === draft.active_variant_id) ?? draft.variants[0] ?? null;
  const confirmedVariants = draft.variants.filter((variant) => variant.status === "confirmed");
  const allVariantsConfirmed = draft.variants.length > 0 && confirmedVariants.length === draft.variants.length;
  const canGenerate = Boolean(selectedProduct) && (selectedSellingPoints.length > 0 || draft.custom_selling_points.length > 0);
  const availableVoiceProfiles = useMemo(
    () => voiceProfilesResource.profiles.filter((profile) => profile.status === "enabled" && profile.preview_status === "ready"),
    [voiceProfilesResource.profiles]
  );
  const selectedVoiceProfile = availableVoiceProfiles.find((profile) => profile.id === draft.voice_profile_id) ?? null;
  const subtitlePresets = subtitlePresetsResource.data ?? [];
  const selectedSubtitlePreset = subtitlePresets.find((preset) => preset.id === draft.subtitle_preset_id) ?? null;
  const enabledBGMTracks = bgmTracksResource.data ?? [];
  const activeBGM = activeVariant?.bgm ?? { mode: "none", track_id: "", gain_db: -12 };
  const selectedBGMTrack = enabledBGMTracks.find((track) => track.id === activeBGM.track_id) ?? null;
  const activeAudition = audition && activeVariant && selectedVoiceProfile
    && audition.voice_profile_id === selectedVoiceProfile.id
    && audition.text === activeVariant.script_text
    ? audition
    : null;
  const scriptGenerationActive = scriptGenerationJob?.status === "queued" || scriptGenerationJob?.status === "generating";
  const scriptGenerationBusy = Boolean(creatingScriptGeneration) || scriptGenerationActive;
  const generating = creatingScriptGeneration?.mode === "replace_all" || (scriptGenerationActive && scriptGenerationJob.mode === "replace_all");
  const regeneratingVariantID = creatingScriptGeneration?.mode === "replace_variant"
    ? creatingScriptGeneration.target_variant_id
    : scriptGenerationActive && scriptGenerationJob.mode === "replace_variant"
      ? scriptGenerationJob.target_variant_id ?? null
      : null;
  const scriptGenerationAppliedLocally = scriptGenerationJob
    && draft.script_generation?.job_id === scriptGenerationJob.id
    && draft.script_generation.applied_locally;
  const scriptGenerationConflict = Boolean(
    scriptGenerationJob?.status === "completed"
    && !scriptGenerationAppliedLocally
    && workbenchDraftRevision(draft) !== scriptGenerationJob.base_revision
  );

  const persistDraft = (next: WorkbenchDraft) => {
    draftRef.current = next;
    saveWorkbenchDraft(next);
    setDraft(next);
  };

  const clearScriptGenerationReference = (jobID: string) => {
    const current = draftRef.current;
    if (current.script_generation?.job_id !== jobID) {
      return;
    }
    persistDraft(withoutScriptGeneration(current));
  };

  const resolveAppliedScriptGeneration = async (job: ScriptGenerationJob) => {
    try {
      const resolved = await resolveScriptGenerationJob(job.id, "applied", token);
      setScriptGenerationJob(resolved);
      clearScriptGenerationReference(job.id);
    } catch {
      message.warning("文案已保存，服务端状态将在下次进入工作台时继续同步");
    }
  };

  const applyCompletedScriptGeneration = async (job: ScriptGenerationJob, strategy: "replace" | "append", automatic = false) => {
    if (handlingScriptGenerationJobsRef.current.has(job.id)) {
      return;
    }
    const current = draftRef.current;
    if (automatic && workbenchDraftRevision(current) !== job.base_revision) {
      return;
    }
    handlingScriptGenerationJobsRef.current.add(job.id);
    setScriptGenerationAction("resolve");
    try {
      const defaultBGM: BGMSelection = { mode: "random", track_id: "", gain_db: -12 };
      const next = applyScriptGenerationResult(current, job, strategy, defaultBGM);
      persistDraft(next);
      await resolveAppliedScriptGeneration(job);
      message.success(job.mode === "replace_variant" && strategy === "replace" ? "当前文案已重新生成" : `已生成 ${job.result_variants?.length ?? 0} 条文案`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "应用文案生成结果失败");
    } finally {
      handlingScriptGenerationJobsRef.current.delete(job.id);
      setScriptGenerationAction(null);
    }
  };

  useEffect(() => {
    draftRef.current = draft;
    saveWorkbenchDraft(draft);
  }, [draft]);

  useEffect(() => {
    let disposed = false;
    const recover = async () => {
      let recovered: ScriptGenerationJob | null = null;
      const referencedJobID = draftRef.current.script_generation?.job_id;
      if (referencedJobID) {
        try {
          recovered = await getScriptGenerationJob(referencedJobID, token);
        } catch {
          recovered = null;
        }
      }
      if (!recovered) {
        try {
          recovered = await getLatestScriptGenerationJob(token);
        } catch {
          return;
        }
      }
      if (disposed) {
        return;
      }
      setScriptGenerationJob(recovered);
      if (!recovered) {
        if (referencedJobID) {
          clearScriptGenerationReference(referencedJobID);
        }
        return;
      }
      if (recovered.status === "applied" || recovered.status === "discarded" || recovered.status === "cancelled") {
        clearScriptGenerationReference(recovered.id);
        return;
      }
      const current = draftRef.current;
      if (current.script_generation?.job_id !== recovered.id) {
        persistDraft({
          ...current,
          script_generation: { job_id: recovered.id, base_revision: recovered.base_revision }
        });
      }
    };
    void recover();
    return () => {
      disposed = true;
    };
  }, [token]);

  useEffect(() => {
    if (!scriptGenerationJob || (scriptGenerationJob.status !== "queued" && scriptGenerationJob.status !== "generating")) {
      return;
    }
    let disposed = false;
    const refresh = async () => {
      try {
        const next = await getScriptGenerationJob(scriptGenerationJob.id, token);
        if (!disposed) {
          setScriptGenerationJob(next);
        }
      } catch {
        // A temporary API interruption must not cancel the persistent generation job.
      }
    };
    const timer = window.setInterval(() => void refresh(), 1_500);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [scriptGenerationJob?.id, scriptGenerationJob?.status, token]);

  useEffect(() => {
    if (!scriptGenerationJob || scriptGenerationJob.status !== "completed") {
      return;
    }
    if (scriptGenerationAppliedLocally) {
      if (!handlingScriptGenerationJobsRef.current.has(scriptGenerationJob.id)) {
        handlingScriptGenerationJobsRef.current.add(scriptGenerationJob.id);
        void resolveAppliedScriptGeneration(scriptGenerationJob).finally(() => {
          handlingScriptGenerationJobsRef.current.delete(scriptGenerationJob.id);
        });
      }
      return;
    }
    if (!scriptGenerationConflict) {
      void applyCompletedScriptGeneration(scriptGenerationJob, "replace", true);
    }
  }, [scriptGenerationAppliedLocally, scriptGenerationConflict, scriptGenerationJob]);

  useEffect(() => () => bgmAudioRef.current?.pause(), []);

  useEffect(() => {
    bgmAudioRef.current?.pause();
    setPlayingBGM(false);
  }, [activeVariant?.id, activeBGM.track_id]);

  useEffect(() => {
    if (bgmTracksResource.loading) {
      return;
    }
    setDraft((current) => {
      let changed = false;
      const variants = current.variants.map((variant) => {
        if (enabledBGMTracks.length === 0 && variant.bgm.mode !== "none") {
          changed = true;
          return { ...variant, bgm: { ...variant.bgm, mode: "none" as const, track_id: "" } };
        }
        if (variant.bgm.mode === "track" && !enabledBGMTracks.some((track) => track.id === variant.bgm.track_id)) {
          changed = true;
          return { ...variant, bgm: { ...variant.bgm, mode: "random" as const, track_id: "" } };
        }
        return variant;
      });
      return changed ? { ...current, variants } : current;
    });
  }, [bgmTracksResource.loading, enabledBGMTracks]);

  useEffect(() => {
    if (!draft.product_id || sellingPoints.loading || defaultedProductIDRef.current === draft.product_id) {
      return;
    }
    const activePoints = activeSellingPoints(sellingPoints.data ?? []).filter((point) => point.product_id === draft.product_id);
    if (activePoints.length === 0) {
      return;
    }
    defaultedProductIDRef.current = draft.product_id;
    setDraft((current) =>
      current.product_id === draft.product_id
        ? { ...current, selling_point_ids: activePoints.map((point) => point.id) }
        : current
    );
  }, [draft.product_id, sellingPoints.data, sellingPoints.loading]);

  useEffect(() => {
    const fallbackProfile = availableVoiceProfiles.find((profile) => profile.is_default) ?? availableVoiceProfiles[0];
    if (!fallbackProfile) {
      return;
    }
    setDraft((current) => {
      const existingProfile = availableVoiceProfiles.find((profile) => profile.id === current.voice_profile_id);
      return existingProfile ? current : { ...current, voice_profile_id: fallbackProfile.id };
    });
  }, [availableVoiceProfiles]);

  useEffect(() => {
    const fallbackPreset = subtitlePresets.find((preset) => preset.is_default) ?? subtitlePresets[0];
    if (!fallbackPreset) {
      return;
    }
    setDraft((current) => {
      const existingPreset = subtitlePresets.find((preset) => preset.id === current.subtitle_preset_id);
      return existingPreset ? current : { ...current, subtitle_preset_id: fallbackPreset.id };
    });
  }, [subtitlePresets]);

  useEffect(() => {
    if (!audition || audition.status === "completed" || audition.status === "failed") {
      return;
    }
    let disposed = false;
    const refresh = async () => {
      try {
        const nextAudition = await getVoiceAudition(audition.id, token);
        if (!disposed) {
          setAudition(nextAudition);
        }
      } catch {
        // The next manual attempt can retry the audition if the task failed remotely.
      }
    };
    const timer = window.setInterval(() => void refresh(), 1_500);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [audition, token]);

  const setProduct = (productID: string) => {
    const apply = () => {
      defaultedProductIDRef.current = "";
      setDraft((current) => ({
        ...clearWorkbenchVariants(current),
        product_id: productID,
        selling_point_ids: [],
        custom_selling_points: []
      }));
    };
    if (draft.variants.length > 0) {
      Modal.confirm({
        title: "切换产品",
        content: "当前文案将被清空。",
        okText: "切换产品",
        cancelText: "取消",
        onOk: apply
      });
      return;
    }
    apply();
  };

  const addCustomSellingPoints = () => {
    const nextValues = customSellingPointInput
      .split(/[，,\n]/)
      .map((value) => value.trim())
      .filter(Boolean);
    if (nextValues.length === 0) {
      return;
    }
    setDraft((current) => ({
      ...current,
      custom_selling_points: Array.from(new Set([...current.custom_selling_points, ...nextValues]))
    }));
    setCustomSellingPointInput("");
  };

  const removeCustomSellingPoint = (value: string) => {
    setDraft((current) => ({
      ...current,
      custom_selling_points: current.custom_selling_points.filter((point) => point !== value)
    }));
  };

  const startScriptGeneration = async (
    mode: ScriptGenerationJobMode,
    targetVariantID = "",
    inputOverride?: ScriptGenerationJobInput
  ) => {
    const current = draftRef.current;
    const input = inputOverride ?? {
      product_id: current.product_id,
      selling_point_ids: current.selling_point_ids,
      custom_selling_points: current.custom_selling_points,
      variant_count: mode === "replace_variant" ? 1 : current.variant_count,
      target_duration_seconds: current.target_duration_seconds,
      temperature: current.temperature
    };
    const baseRevision = workbenchDraftRevision(current);
    setCreatingScriptGeneration({ mode, target_variant_id: targetVariantID });
    try {
      const job = await createScriptGenerationJob({
        ...input,
        variant_count: mode === "replace_variant" ? 1 : input.variant_count,
        mode,
        target_variant_id: targetVariantID || undefined,
        base_revision: baseRevision
      }, token);
      setScriptGenerationJob(job);
      const latest = draftRef.current;
      persistDraft({
        ...latest,
        script_generation: { job_id: job.id, base_revision: job.base_revision }
      });
      message.success(mode === "replace_variant" ? "当前文案已加入后台生成" : "文案已加入后台生成");
    } catch (error) {
      try {
        const existing = await getLatestScriptGenerationJob(token);
        if (existing) {
          setScriptGenerationJob(existing);
          persistDraft({
            ...draftRef.current,
            script_generation: { job_id: existing.id, base_revision: existing.base_revision }
          });
        }
      } catch {
        // Preserve the original create error.
      }
      message.error(error instanceof Error ? error.message : "创建文案生成任务失败");
    } finally {
      setCreatingScriptGeneration(null);
    }
  };

  const generateScripts = async () => {
    if (!selectedProduct) {
      message.warning("请选择产品");
      return;
    }
    if (selectedSellingPoints.length === 0 && draft.custom_selling_points.length === 0) {
      message.warning("请至少选择或补充一个卖点");
      return;
    }
    await startScriptGeneration("replace_all");
  };

  const cancelScriptGeneration = async () => {
    if (!scriptGenerationJob || !scriptGenerationActive) {
      return;
    }
    setScriptGenerationAction("cancel");
    try {
      await cancelScriptGenerationJob(scriptGenerationJob.id, token);
      clearScriptGenerationReference(scriptGenerationJob.id);
      setScriptGenerationJob(null);
      message.success("已取消文案生成");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "取消文案生成失败");
    } finally {
      setScriptGenerationAction(null);
    }
  };

  const discardScriptGeneration = async () => {
    if (!scriptGenerationJob) {
      return;
    }
    setScriptGenerationAction("resolve");
    try {
      await resolveScriptGenerationJob(scriptGenerationJob.id, "discarded", token);
      clearScriptGenerationReference(scriptGenerationJob.id);
      setScriptGenerationJob(null);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "丢弃文案生成结果失败");
    } finally {
      setScriptGenerationAction(null);
    }
  };

  const retryScriptGeneration = async () => {
    if (!scriptGenerationJob || scriptGenerationJob.status !== "failed") {
      return;
    }
    if (draftRef.current.product_id !== scriptGenerationJob.product_id) {
      message.warning("当前产品已变更，请先丢弃失败任务后重新生成");
      return;
    }
    if (scriptGenerationJob.mode === "replace_variant" && !draftRef.current.variants.some((variant) => variant.id === scriptGenerationJob.target_variant_id)) {
      message.warning("原文案已不存在，请先丢弃失败任务");
      return;
    }
    setScriptGenerationAction("retry");
    const failedJob = scriptGenerationJob;
    try {
      await resolveScriptGenerationJob(failedJob.id, "discarded", token);
      clearScriptGenerationReference(failedJob.id);
      setScriptGenerationJob(null);
      await startScriptGeneration(failedJob.mode, failedJob.target_variant_id, failedJob.input);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "重试文案生成失败");
    } finally {
      setScriptGenerationAction(null);
    }
  };

  const updateVariant = (variantID: string, update: (variant: ScriptVariant) => ScriptVariant) => {
    setDraft((current) => ({
      ...current,
      variants: current.variants.map((variant) => (variant.id === variantID ? update(variant) : variant))
    }));
  };

  const toggleVariantConfirmed = (variantID: string) => {
    updateVariant(variantID, (variant) => ({
      ...variant,
      status: variant.status === "confirmed" ? "draft" : "confirmed",
      updated_at: new Date().toISOString()
    }));
  };

  const toggleAllVariantsConfirmed = () => {
    const status = allVariantsConfirmed ? "draft" : "confirmed";
    const updatedAt = new Date().toISOString();
    setDraft((current) => ({
      ...current,
      variants: current.variants.map((variant) => ({ ...variant, status, updated_at: updatedAt }))
    }));
  };

  const importScripts = (scripts: ImportedScript[], mode: "append" | "replace") => {
    const defaultBGM: BGMSelection = { mode: enabledBGMTracks.length > 0 ? "random" : "none", track_id: "", gain_db: -12 };
    const now = new Date().toISOString();
    setDraft((current) => {
      const retained = mode === "replace" ? [] : current.variants;
      const available = Math.max(0, maxWorkbenchScripts - retained.length);
      const imported = scripts.slice(0, available).map<ScriptVariant>((script, index) => ({
        id: createUUID(),
        order: retained.length + index + 1,
        hook: deriveScriptHook(script.script_text, script.title),
        script_text: script.script_text,
        estimated_duration_ms: estimateDuration(script.script_text),
        editing_intent: "",
        beats: [],
        status: "draft",
        origin: "imported",
        bgm: { ...defaultBGM },
        updated_at: now
      }));
      const variants = [...retained, ...imported].map((variant, index) => ({ ...variant, order: index + 1 }));
      return {
        ...current,
        variants,
        active_variant_id: imported[0]?.id ?? variants[0]?.id ?? ""
      };
    });
    setImportOpen(false);
    message.success(`已导入 ${scripts.length} 条文案`);
  };

  const regenerateActiveVariant = async () => {
    if (!selectedProduct || !activeVariant) {
      return;
    }
    await startScriptGeneration("replace_variant", activeVariant.id);
  };

  const requestAudition = async () => {
    if (!activeVariant || !selectedVoiceProfile) {
      message.warning("请选择样音已就绪的音色");
      return;
    }
    setCreatingAudition(true);
    try {
      const nextAudition = await createVoiceAudition(selectedVoiceProfile.id, activeVariant.script_text, token);
      setAudition(nextAudition);
      message.success("试听已加入生成队列");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "创建试听失败");
    } finally {
      setCreatingAudition(false);
    }
  };

  const updateActiveBGM = (update: Partial<BGMSelection>) => {
    if (!activeVariant) {
      return;
    }
    updateVariant(activeVariant.id, (variant) => ({ ...variant, bgm: { ...variant.bgm, ...update } }));
  };

  const applyBGMToAll = () => {
    if (!activeVariant) {
      return;
    }
    const selection = { ...activeVariant.bgm };
    setDraft((current) => ({
      ...current,
      variants: current.variants.map((variant) => ({ ...variant, bgm: { ...selection } }))
    }));
    message.success("音乐设置已应用到全部文案");
  };

  const toggleBGMPreview = () => {
    const audio = bgmAudioRef.current;
    if (!audio || !selectedBGMTrack) {
      return;
    }
    if (!audio.paused) {
      audio.pause();
      setPlayingBGM(false);
      return;
    }
    void audio.play().then(() => setPlayingBGM(true)).catch(() => setPlayingBGM(false));
  };

  const startTasks = async () => {
    if (!selectedProduct || confirmedVariants.length === 0) {
      return;
    }
    if (!selectedVoiceProfile) {
      message.warning("请选择音色");
      return;
    }
    setStartingTasks(true);
    try {
      const works = await createVoiceoverTasks(
        selectedProduct.id,
        selectedVoiceProfile.id,
        draft.output_ratio,
        selectedSubtitlePreset?.id ?? "",
        confirmedVariants,
        token
      );
      const submittedIDs = new Set(confirmedVariants.map((variant) => variant.id));
      const remainingVariants = draft.variants
        .filter((variant) => !submittedIDs.has(variant.id))
        .map((variant, index) => ({ ...variant, order: index + 1 }));
      const nextDraft = {
        ...draft,
        variants: remainingVariants,
        active_variant_id: remainingVariants[0]?.id ?? ""
      };
      saveWorkbenchDraft(nextDraft);
      setDraft(nextDraft);
      message.success(`已开始 ${works.length} 条任务`);
      window.location.hash = "#/finished";
    } catch (error) {
      message.error(error instanceof Error ? error.message : "创建配音任务失败");
    } finally {
      setStartingTasks(false);
    }
  };

  const productOptions = useMemo(
    () => productList.filter((product) => product.status !== "archived").map((product) => ({ value: product.id, label: product.name })),
    [productList]
  );

  return (
    <div className="workbench-page" data-testid="workbench-page">
      <section className="workbench-brief" aria-label="任务输入">
        <div className="workbench-field workbench-product-field">
          <Typography.Text className="workbench-field-label">产品</Typography.Text>
          <Select
            data-testid="workbench-product-select"
            value={draft.product_id || undefined}
            placeholder="选择产品"
            options={productOptions}
            loading={products.loading}
            onChange={setProduct}
          />
        </div>
        <div className="workbench-field workbench-points-field">
          <Typography.Text className="workbench-field-label">卖点</Typography.Text>
          <Select
            data-testid="workbench-selling-points-select"
            mode="multiple"
            value={draft.selling_point_ids}
            placeholder={draft.product_id ? "选择卖点" : "先选择产品"}
            disabled={!draft.product_id}
            loading={sellingPoints.loading}
            options={availableSellingPoints.map((point) => ({ value: point.id, label: point.title }))}
            maxTagCount="responsive"
            onChange={(sellingPointIDs) => setDraft((current) => ({ ...current, selling_point_ids: sellingPointIDs }))}
          />
        </div>
        <div className="workbench-field workbench-custom-points-field">
          <Typography.Text className="workbench-field-label">补充卖点</Typography.Text>
          <div className="workbench-custom-point-input">
            <Input
              value={customSellingPointInput}
              placeholder="输入后回车"
              disabled={!draft.product_id}
              onChange={(event) => setCustomSellingPointInput(event.target.value)}
              onPressEnter={addCustomSellingPoints}
            />
            <Tooltip title="添加卖点">
              <Button
                type="text"
                aria-label="添加卖点"
                icon={<Plus size={17} />}
                disabled={!draft.product_id}
                onClick={addCustomSellingPoints}
              />
            </Tooltip>
          </div>
          {draft.custom_selling_points.length > 0 ? (
            <div className="workbench-custom-point-tags">
              {draft.custom_selling_points.map((point) => (
                <Tag
                  key={point}
                  closable
                  closeIcon={<X size={13} />}
                  onClose={() => removeCustomSellingPoint(point)}
                >
                  {point}
                </Tag>
              ))}
            </div>
          ) : null}
        </div>
        <div className="workbench-field workbench-count-field">
          <Typography.Text className="workbench-field-label">条数</Typography.Text>
          <InputNumber
            min={1}
            max={8}
            precision={0}
            value={draft.variant_count}
            onChange={(value) => setDraft((current) => ({ ...current, variant_count: Number(value ?? 1) }))}
          />
        </div>
        <div className="workbench-field workbench-duration-field">
          <Typography.Text className="workbench-field-label">时长</Typography.Text>
          <Select<ScriptTargetDuration>
            data-testid="workbench-target-duration"
            value={draft.target_duration_seconds}
            options={[15, 20, 30, 45, 60].map((seconds) => ({ value: seconds as ScriptTargetDuration, label: `${seconds} 秒` }))}
            onChange={(targetDuration) => setDraft((current) => ({ ...current, target_duration_seconds: targetDuration }))}
          />
        </div>
        <div className="workbench-field workbench-temperature-field">
          <Typography.Text className="workbench-field-label">温度</Typography.Text>
          <InputNumber
            data-testid="workbench-temperature"
            min={0}
            max={2}
            step={0.1}
            precision={2}
            value={draft.temperature}
            onChange={(value) => setDraft((current) => ({ ...current, temperature: Number(value ?? 0.75) }))}
          />
        </div>
        <div className="workbench-field workbench-ratio-field">
          <Typography.Text className="workbench-field-label">画幅</Typography.Text>
          <Segmented<OutputRatio>
            block
            value={draft.output_ratio}
            options={["9:16", "3:4"]}
            onChange={(outputRatio) => setDraft((current) => ({ ...current, output_ratio: outputRatio }))}
          />
        </div>
        <div className="workbench-field workbench-subtitle-field">
          <Typography.Text className="workbench-field-label">字幕样式</Typography.Text>
          <div className="workbench-subtitle-control">
            <Select
              data-testid="workbench-subtitle-preset"
              value={draft.subtitle_preset_id || undefined}
              placeholder="选择样式"
              loading={subtitlePresetsResource.loading}
              options={subtitlePresets.map((preset) => ({ value: preset.id, label: preset.name }))}
              onChange={(presetID) => setDraft((current) => ({ ...current, subtitle_preset_id: presetID }))}
            />
            <Popover
              placement="bottomRight"
              content={selectedSubtitlePreset ? <SubtitleStylePreview preset={selectedSubtitlePreset} ratio={draft.output_ratio} compact /> : null}
              trigger="click"
            >
              <Button
                type="text"
                aria-label="预览字幕样式"
                icon={<Captions size={16} />}
                disabled={!selectedSubtitlePreset}
              />
            </Popover>
          </div>
        </div>
        <div className="workbench-field workbench-voice-field">
          <Typography.Text className="workbench-field-label">音色</Typography.Text>
          <VoiceProfilePicker
            profiles={availableVoiceProfiles}
            value={draft.voice_profile_id}
            onChange={(voiceProfileID) => setDraft((current) => ({ ...current, voice_profile_id: voiceProfileID }))}
          />
        </div>
        <div className="workbench-primary-actions">
          <Button
            data-testid="workbench-import"
            icon={<FileUp size={17} />}
            disabled={!selectedProduct}
            onClick={() => setImportOpen(true)}
          >
            导入文案
          </Button>
          <Button
            type="primary"
            className="workbench-generate-button"
            data-testid="workbench-generate"
            icon={<Sparkles size={17} />}
            loading={generating}
            disabled={!canGenerate || scriptGenerationBusy || scriptGenerationJob?.status === "completed" || scriptGenerationJob?.status === "failed"}
            onClick={() => void generateScripts()}
          >
            生成文案
          </Button>
        </div>
        {scriptGenerationActive ? (
          <div className="workbench-generation-notice is-running" data-testid="workbench-generation-job">
            <span>
              <strong>{scriptGenerationJob?.mode === "replace_variant" ? "正在重新生成当前文案" : "正在后台生成文案"}</strong>
              <small>可以离开工作台，任务和结果会自动保留。</small>
            </span>
            <Button
              type="text"
              danger
              size="small"
              loading={scriptGenerationAction === "cancel"}
              onClick={() => void cancelScriptGeneration()}
            >
              取消生成
            </Button>
          </div>
        ) : scriptGenerationJob?.status === "failed" ? (
          <div className="workbench-generation-notice is-error" data-testid="workbench-generation-job">
            <span>
              <strong>文案生成失败</strong>
              <small>{scriptGenerationJob.error_message || "服务端未返回错误信息"}</small>
            </span>
            <div className="workbench-generation-notice-actions">
              <Button type="text" size="small" loading={scriptGenerationAction === "retry"} onClick={() => void retryScriptGeneration()}>重试</Button>
              <Button type="text" size="small" loading={scriptGenerationAction === "resolve"} onClick={() => void discardScriptGeneration()}>忽略</Button>
            </div>
          </div>
        ) : scriptGenerationJob?.status === "completed" && scriptGenerationConflict ? (
          <div className="workbench-generation-notice is-conflict" data-testid="workbench-generation-job">
            <span>
              <strong>后台文案已生成，但当前草稿已修改</strong>
              <small>请选择如何处理，系统不会自动覆盖现有文案。</small>
            </span>
            <div className="workbench-generation-notice-actions">
              <Button type="primary" size="small" loading={scriptGenerationAction === "resolve"} onClick={() => void applyCompletedScriptGeneration(scriptGenerationJob, "replace")}>替换</Button>
              <Button
                size="small"
                disabled={draft.product_id !== scriptGenerationJob.product_id || draft.variants.length >= maxWorkbenchScripts}
                onClick={() => void applyCompletedScriptGeneration(scriptGenerationJob, "append")}
              >
                追加
              </Button>
              <Button type="text" size="small" onClick={() => void discardScriptGeneration()}>丢弃</Button>
            </div>
          </div>
        ) : scriptGenerationJob?.status === "completed" ? (
          <div className="workbench-generation-notice is-running" data-testid="workbench-generation-job">
            <span>
              <strong>{scriptGenerationAppliedLocally ? "文案结果已保存" : "文案生成完成"}</strong>
              <small>{scriptGenerationAppliedLocally ? "正在同步服务端状态。" : "正在写入当前草稿。"}</small>
            </span>
          </div>
        ) : null}
      </section>

      <main className="workbench-main">
        {draft.variants.length === 0 ? (
          <div className="workbench-empty-state">
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未生成文案" />
          </div>
        ) : (
          <div className="workbench-editor">
            <aside className="workbench-variant-rail" aria-label="文案版本">
              <div className="workbench-rail-header">
                <Typography.Text>文案 {draft.variants.length}</Typography.Text>
                <span className="workbench-rail-actions">
                  <Tooltip title={allVariantsConfirmed ? "取消全部确认" : "确认全部文案"}>
                    <Button
                      type="text"
                      size="small"
                      aria-label={allVariantsConfirmed ? "取消全部确认" : "确认全部文案"}
                      icon={<ListChecks size={16} />}
                      onClick={toggleAllVariantsConfirmed}
                    />
                  </Tooltip>
                  <Tooltip title="清空当前文案">
                    <Button
                      type="text"
                      size="small"
                      aria-label="清空当前文案"
                      icon={<RotateCcw size={16} />}
                      onClick={() => setDraft((current) => clearWorkbenchVariants(current))}
                    />
                  </Tooltip>
                </span>
              </div>
              <div className="workbench-variant-list">
                {draft.variants.map((variant) => (
                  <button
                    type="button"
                    key={variant.id}
                    className={`workbench-variant-row${activeVariant?.id === variant.id ? " is-active" : ""}`}
                    onClick={() => setDraft((current) => ({ ...current, active_variant_id: variant.id }))}
                  >
                    <span className="workbench-variant-index">{String(variant.order).padStart(2, "0")}</span>
                    <span className="workbench-variant-copy">
                      <span className="workbench-variant-hook">{variant.hook}</span>
                      <span>{formatDuration(variant.estimated_duration_ms)}</span>
                    </span>
                    {variant.status === "confirmed" ? <CheckCircle2 size={16} /> : <Circle size={16} />}
                  </button>
                ))}
              </div>
            </aside>

            {activeVariant ? (
              <section className="workbench-script-editor" aria-label={`文案 ${activeVariant.order}`}>
                <div className="workbench-script-toolbar">
                  <div>
                    <Typography.Text className="workbench-script-index">文案 {String(activeVariant.order).padStart(2, "0")}</Typography.Text>
                    <Typography.Text type="secondary">
                      预计 {formatDuration(activeVariant.estimated_duration_ms)} · 目标 {draft.target_duration_seconds} 秒
                    </Typography.Text>
                  </div>
                  <div className="workbench-script-actions">
                    <Button
                      type="text"
                      aria-label="试听当前文案"
                      icon={<Volume2 size={17} />}
                      loading={creatingAudition || (activeAudition?.status === "queued" || activeAudition?.status === "synthesizing")}
                      disabled={!selectedVoiceProfile || activeAudition?.status === "queued" || activeAudition?.status === "synthesizing"}
                      onClick={() => void requestAudition()}
                    >
                      {activeAudition?.status === "queued" || activeAudition?.status === "synthesizing" ? "生成试听" : "试听当前文案"}
                    </Button>
                    {activeVariant.origin !== "imported" ? (
                      <Tooltip title="重新生成当前文案">
                        <Button
                          type="text"
                          aria-label="重新生成当前文案"
                          icon={<RefreshCw size={17} />}
                          loading={regeneratingVariantID === activeVariant.id}
                          disabled={scriptGenerationBusy || scriptGenerationJob?.status === "completed" || scriptGenerationJob?.status === "failed"}
                          onClick={() => void regenerateActiveVariant()}
                        />
                      </Tooltip>
                    ) : null}
                    <Button
                      type={activeVariant.status === "confirmed" ? "default" : "primary"}
                      icon={<Check size={16} />}
                      onClick={() => toggleVariantConfirmed(activeVariant.id)}
                    >
                      {activeVariant.status === "confirmed" ? "已确认" : "确认文案"}
                    </Button>
                  </div>
                </div>

                {activeAudition?.status === "completed" && activeAudition.audio_url ? (
                  <div className="workbench-audition-player">
                    <audio controls preload="none" src={activeAudition.audio_url} aria-label="当前文案试听" />
                  </div>
                ) : null}
                {activeAudition?.status === "failed" ? <Typography.Text type="danger">{activeAudition.error_message || "试听生成失败"}</Typography.Text> : null}

                <Input.TextArea
                  data-testid="workbench-script-editor"
                  className="workbench-script-text"
                  value={activeVariant.script_text}
                  autoSize={{ minRows: 7, maxRows: 14 }}
                  onChange={(event) => {
                    const scriptText = event.target.value;
                    updateVariant(activeVariant.id, (variant) => ({
                      ...variant,
                      script_text: scriptText,
                      estimated_duration_ms: estimateDuration(scriptText),
                      status: "draft",
                      intent_stale: variant.origin !== "imported",
                      updated_at: new Date().toISOString()
                    }));
                  }}
                />

                <section className="workbench-bgm" aria-label="背景音乐设置">
                  <div className="workbench-bgm-heading">
                    <span><Music2 size={15} />背景音乐</span>
                    <Button type="text" size="small" icon={<Copy size={14} />} onClick={applyBGMToAll}>应用到全部文案</Button>
                  </div>
                  <div className="workbench-bgm-controls">
                    <Segmented<BGMSelection["mode"]>
                      value={activeBGM.mode}
                      options={[{ value: "random", label: "随机" }, { value: "track", label: "指定" }, { value: "none", label: "无音乐" }]}
                      disabled={enabledBGMTracks.length === 0}
                      onChange={(mode) => updateActiveBGM({ mode, track_id: mode === "track" ? (activeBGM.track_id || enabledBGMTracks[0]?.id || "") : "" })}
                    />
                    {activeBGM.mode === "track" ? (
                      <div className="workbench-bgm-track">
                        <Tooltip title={playingBGM ? "暂停音乐" : "试听音乐"}>
                          <Button type="text" aria-label={playingBGM ? "暂停音乐" : "试听音乐"} icon={playingBGM ? <Pause size={16} /> : <Play size={16} />} disabled={!selectedBGMTrack} onClick={toggleBGMPreview} />
                        </Tooltip>
                        <Select
                          value={activeBGM.track_id || undefined}
                          placeholder="选择音乐"
                          options={enabledBGMTracks.map((track) => ({ value: track.id, label: `${track.name}${track.mood ? ` · ${track.mood}` : ""}${track.bpm ? ` · ${track.bpm} BPM` : ""}` }))}
                          onChange={(trackID) => updateActiveBGM({ track_id: trackID })}
                        />
                        <audio ref={bgmAudioRef} preload="none" src={selectedBGMTrack?.audio_url} onPause={() => setPlayingBGM(false)} onEnded={() => setPlayingBGM(false)} />
                      </div>
                    ) : activeBGM.mode === "random" ? <span className="workbench-bgm-available">{enabledBGMTracks.length} 首可用</span> : <span />}
                    <span className="workbench-bgm-gain-label">音量</span>
                    <Slider min={-30} max={0} step={1} value={activeBGM.gain_db} disabled={activeBGM.mode === "none"} onChange={(gainDB) => updateActiveBGM({ gain_db: gainDB })} />
                    <InputNumber min={-30} max={0} step={1} precision={0} addonAfter="dB" value={activeBGM.gain_db} disabled={activeBGM.mode === "none"} onChange={(value) => updateActiveBGM({ gain_db: Number(value ?? -12) })} />
                  </div>
                </section>

                {activeVariant.editing_intent || activeVariant.beats.length > 0 ? <section className="workbench-intent" aria-label="镜头意图">
                  <div className="workbench-intent-heading">
                    <Typography.Text>镜头意图</Typography.Text>
                    {activeVariant.intent_stale ? <Tag color="gold">待刷新</Tag> : null}
                  </div>
                  <Typography.Paragraph>{activeVariant.editing_intent}</Typography.Paragraph>
                  <ol className="workbench-beat-list">
                    {activeVariant.beats.map((beat) => (
                      <li key={beat.id}>
                        <span className="workbench-beat-label">{beat.label}</span>
                        <span className="workbench-beat-copy">
                          <strong>{beat.selling_point}</strong>
                          <span>{beat.visual_goal}</span>
                        </span>
                        <Tag>{sourceTypeLabels[beat.source_type]}</Tag>
                      </li>
                    ))}
                  </ol>
                </section> : null}
              </section>
            ) : null}
          </div>
        )}
      </main>

      <footer className="workbench-footer">
        <Typography.Text>
          已确认 {confirmedVariants.length} 条
          {selectedVoiceProfile ? ` · ${selectedVoiceProfile.name}` : ""}
          {selectedSubtitlePreset ? ` · ${draft.output_ratio} · ${selectedSubtitlePreset.name}` : ""}
        </Typography.Text>
        <Button
          type="primary"
          data-testid="workbench-start-tasks"
          icon={<Clapperboard size={17} />}
          loading={startingTasks}
          disabled={confirmedVariants.length === 0 || !selectedVoiceProfile}
          onClick={() => void startTasks()}
        >
          开始 {confirmedVariants.length} 条任务
        </Button>
      </footer>
      <ScriptImportModal
        open={importOpen}
        existingScripts={draft.variants.map((variant) => variant.script_text)}
        onCancel={() => setImportOpen(false)}
        onImport={importScripts}
      />
    </div>
  );
}
