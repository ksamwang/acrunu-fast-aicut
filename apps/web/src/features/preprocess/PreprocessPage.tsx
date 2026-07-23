import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Descriptions,
  Dropdown,
  Empty,
  Form,
  Image,
  Input,
  InputNumber,
  Modal,
  Pagination,
  Select,
  Space,
  Spin,
  Switch,
  Tag,
  Typography,
  message
} from "antd";
import { Check, Download, MonitorCog, Power, RefreshCw, ScanSearch, Send, Trash2 } from "lucide-react";
import { VideoTrimEditor } from "./VideoTrimEditor";
import {
  launchLocalAgent,
  localAgentRequest,
  probeLocalAgent,
  type LocalAgentProbeResult
} from "../../shared/api/local-agent-api";
import { apiRequest, authenticatedApiRequest, type LocalAgentRelease } from "../../shared/api/server-api";
import { formatDateTime, formatDuration, formatTimestamp } from "../../shared/lib/format";
import type { Product, SellingPoint } from "../../shared/types/product";
import type {
  UploadToken,
  WorkspaceImportResponse,
  WorkspaceItem,
  WorkspaceItemResponse,
  WorkspaceListResponse,
  WorkspaceProbe,
  WorkspaceTranscriptSegment
} from "../../shared/types/workspace";
import { loadLastSubmitProductID, persistLastSubmitProductID } from "./storage";
import "./styles.css";

type ImportPreview = {
  id: string;
  file: File;
  status: "waiting" | "importing" | "completed" | "failed";
  error?: string;
};

const workspacePageSize = 50;
const importQueuePageSize = 50;
const importConcurrency = 16;
const batchRequestConcurrency = 2;

type BatchAction = "vlm" | "submit" | "delete";

type BatchOperationState = {
  action: BatchAction;
  total: number;
  completed: number;
  succeeded: number;
  failed: number;
  skipped: number;
  running: boolean;
};

type BatchSubmitVLMBlocker =
  | "not_started"
  | "queued"
  | "running"
  | "failed"
  | "missing_result"
  | "stale_selection"
  | "product_mismatch";

type MarqueeRect = {
  left: number;
  top: number;
  width: number;
  height: number;
};

type MarqueeStart = {
  pointerID: number;
  clientX: number;
  clientY: number;
  latestClientX: number;
  latestClientY: number;
  boardX: number;
  boardY: number;
  append: boolean;
  initialIDs: Set<string>;
  dragged: boolean;
};

type WorkspaceStatsState = {
  pending: number;
  saved: number;
  ready: number;
  submitted: number;
};

const workspaceStatKeys: Record<WorkspaceItem["status"], keyof WorkspaceStatsState> = {
  pending: "pending",
  saved: "saved",
  ready_to_submit: "ready",
  submitted: "submitted"
};

const importStatusLabels: Record<ImportPreview["status"], string> = {
  waiting: "等待导入",
  importing: "复制并分析中",
  completed: "导入完成",
  failed: "导入失败"
};

const importStatusColors: Record<ImportPreview["status"], string> = {
  waiting: "default",
  importing: "processing",
  completed: "success",
  failed: "error"
};

const workspaceVLMStatusLabels: Record<NonNullable<WorkspaceItem["vlm_status"]>, string> = {
  idle: "",
  queued: "VLM 排队中",
  running: "VLM 识别中",
  ready: "VLM 已完成",
  failed: "VLM 失败"
};

const batchActionLabels: Record<BatchAction, string> = {
  vlm: "批量 VLM",
  submit: "正式提交",
  delete: "批量删除"
};

async function runWithConcurrency<T>(
  entries: T[],
  concurrency: number,
  worker: (entry: T) => Promise<void>
) {
  let cursor = 0;
  const workers = Array.from({ length: Math.min(concurrency, entries.length) }, async () => {
    while (cursor < entries.length) {
      const index = cursor;
      cursor += 1;
      await worker(entries[index]);
    }
  });
  await Promise.all(workers);
}

function workspaceSavePayload(item: WorkspaceItem) {
  const sourceInMs = Math.max(0, Math.round(item.source_in_ms ?? 0));
  const durationMs = Math.max(item.probe.duration_ms ?? 0, sourceInMs + 1);
  const sourceOutMs = Math.max(sourceInMs + 1, Math.min(Math.round(item.source_out_ms || durationMs), durationMs));
  return {
    asset_name: item.asset_name || item.original_file_name,
    source_type: item.source_type || defaultSourceType,
    use_original_audio: Boolean(item.use_original_audio),
    source_in_ms: sourceInMs,
    source_out_ms: sourceOutMs,
    interpret_fps_enabled: Boolean(item.interpret_fps_enabled),
    playback_fps: item.playback_fps || 25,
    transcript: item.transcript ?? "",
    transcript_segments: item.transcript_segments ?? [],
    reviewer_notes: item.reviewer_notes ?? ""
  };
}

function isWorkspaceItemBusy(item: WorkspaceItem) {
  return item.vlm_status === "queued" || item.vlm_status === "running";
}

function batchSubmitVLMBlocker(item: WorkspaceItem, productID: string): BatchSubmitVLMBlocker | null {
  if (item.status === "submitted") {
    return null;
  }
  switch (item.vlm_status || "idle") {
    case "queued":
      return "queued";
    case "running":
      return "running";
    case "failed":
      return "failed";
    case "ready":
      break;
    default:
      return "not_started";
  }
  if (!item.analysis) {
    return "missing_result";
  }
  if (
    item.vlm_source_type !== (item.source_type || defaultSourceType) ||
    item.vlm_source_in_ms !== item.source_in_ms ||
    item.vlm_source_out_ms !== item.source_out_ms
  ) {
    return "stale_selection";
  }
  if (productID && item.vlm_product_id !== productID) {
    return "product_mismatch";
  }
  return null;
}

function SvgIcon({
  children,
  viewBox = "0 0 24 24"
}: {
  children: React.ReactNode;
  viewBox?: string;
}) {
  return (
    <svg className="ui-icon" viewBox={viewBox} aria-hidden="true" focusable="false">
      {children}
    </svg>
  );
}

function RefreshIcon() {
  return (
    <SvgIcon>
      <path d="M20 6v5h-5" />
      <path d="M4 18v-5h5" />
      <path d="M6.1 9a7 7 0 0 1 11.5-2.6L20 8.7" />
      <path d="M17.9 15a7 7 0 0 1-11.5 2.6L4 15.3" />
    </SvgIcon>
  );
}

function TrashIcon() {
  return (
    <SvgIcon>
      <path d="M3 6h18" />
      <path d="M8 6V4h8v2" />
      <path d="M7 6l1 14h8l1-14" />
      <path d="M10 10v6" />
      <path d="M14 10v6" />
    </SvgIcon>
  );
}

function UploadIcon() {
  return (
    <SvgIcon>
      <path d="M12 16V4" />
      <path d="M7 9l5-5 5 5" />
      <path d="M5 16v4h14v-4" />
    </SvgIcon>
  );
}

function PlusIcon() {
  return (
    <SvgIcon>
      <path d="M12 5v14" />
      <path d="M5 12h14" />
    </SvgIcon>
  );
}

function CloseIcon() {
  return (
    <SvgIcon>
      <path d="M6 6l12 12" />
      <path d="M18 6L6 18" />
    </SvgIcon>
  );
}

const workspaceStatusLabels: Record<WorkspaceItem["status"], string> = {
  pending: "待处理",
  saved: "本地已保存",
  ready_to_submit: "待提交",
  submitted: "已入库"
};

const workspaceStatusColors: Record<WorkspaceItem["status"], string> = {
  pending: "default",
  saved: "blue",
  ready_to_submit: "orange",
  submitted: "green"
};

const sourceTypeLabels: Record<string, string> = {
  visual_only: "纯画面",
  talking_head: "口播"
};

const sourceTypeOptions: Array<{ value: NonNullable<WorkspaceItem["source_type"]>; label: string }> = [
  { value: "visual_only", label: sourceTypeLabels.visual_only },
  { value: "talking_head", label: sourceTypeLabels.talking_head }
];

const defaultSourceType: NonNullable<WorkspaceItem["source_type"]> = sourceTypeOptions[0]?.value ?? "visual_only";

const sourceTypeColors: Record<string, string> = {
  visual_only: "cyan",
  talking_head: "purple"
};

const shotSizeLabels: Record<string, string> = {
  close_up: "特写",
  medium_close_up: "近景",
  medium_shot: "中景",
  full_shot: "全景",
  wide_shot: "远景"
};

const cameraMovementLabels: Record<string, string> = {
  static: "固定机位",
  pan: "水平摇镜",
  tilt: "垂直摇镜",
  push_in: "推进",
  pull_out: "拉远",
  tracking: "跟拍/平移",
  orbit: "环绕",
  zoom: "变焦",
  handheld: "手持",
  mixed: "复合运镜",
  unknown: "无法判断",
  slow_push_in: "推进"
};

function formatProbeDuration(durationMs?: number) {
  if (durationMs === undefined || durationMs === null) {
    return "待读取";
  }
  const totalSeconds = durationMs / 1000;
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds - minutes * 60;
  return `${minutes}:${seconds.toFixed(3).padStart(6, "0")} / ${totalSeconds.toFixed(3)} 秒`;
}

function formatResolution(width?: number, height?: number) {
  if (!width || !height) {
    return "待读取";
  }
  return `${width} x ${height}`;
}

function formatProbeFPS(fps?: number) {
  if (!fps || !Number.isFinite(fps)) {
    return "待读取";
  }
  return `${fps.toFixed(3)} fps`;
}

function formatFileSize(size: number) {
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}

function transcriptTextFromSegments(segments: WorkspaceTranscriptSegment[]) {
  return segments.map((segment) => segment.text.trim()).filter(Boolean).join("");
}

function getWorkspacePreviewUrl(item: WorkspaceItem) {
  return item.thumbnail_url || item.frame_snapshots[0]?.image_url;
}

function sourceIdentityKey(item: WorkspaceItem) {
  const sourceURL = item.source_url.split("?")[0];
  const durationMs = item.probe.duration_ms || 0;
  const fps = item.probe.fps || 0;
  const sourceMode = item.interpret_fps_enabled ? "interpreted" : "source";
  return `${item.id}:${sourceURL}:${durationMs}:${fps}:${sourceMode}`;
}

function productReferenceImage(product?: Product | null) {
  const image = product?.metadata?.reference_image;
  return typeof image === "string" ? image : "";
}

function disableButtonTabStops(root: HTMLElement | null) {
  root?.querySelectorAll("button").forEach((button) => {
    button.tabIndex = -1;
  });
}

function isButtonElementTarget(target: EventTarget | null) {
  return target instanceof Element ? target.closest("button") : null;
}

function currentSourceDurationMs(item: WorkspaceItem) {
  return item.probe.duration_ms && item.probe.duration_ms > 0
    ? item.probe.duration_ms
    : Math.max(item.source_out_ms ?? 0, 0);
}

function clampCurrentSourceRange(sourceInMs: number, sourceOutMs: number, item: WorkspaceItem) {
  const durationMs = currentSourceDurationMs(item);
  const safeDurationMs = durationMs > 0 ? durationMs : sourceOutMs;
  const safeInMs = Math.max(0, Math.min(Math.round(sourceInMs), Math.max(safeDurationMs - 1, 0)));
  const safeOutMs = Math.max(safeInMs + 1, Math.min(Math.round(sourceOutMs), safeDurationMs));
  return {
    sourceInMs: safeInMs,
    sourceOutMs: safeOutMs
  };
}

export function PreprocessPage({ token }: { token: string }) {
  const [localAgent, setLocalAgent] = useState<LocalAgentProbeResult | { state: "checking" }>({ state: "checking" });
  const [localAgentRelease, setLocalAgentRelease] = useState<LocalAgentRelease | null>(null);
  const [localAgentReleaseError, setLocalAgentReleaseError] = useState("");
  const [launchingLocalAgent, setLaunchingLocalAgent] = useState(false);
  const [items, setItems] = useState<WorkspaceItem[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [sellingPoints, setSellingPoints] = useState<SellingPoint[]>([]);
  const [loading, setLoading] = useState(false);
  const [importing, setImporting] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [previewingFrames, setPreviewingFrames] = useState(false);
  const [transcribingItemID, setTranscribingItemID] = useState<string | null>(null);
  const [startingVLMLabel, setStartingVLMLabel] = useState(false);
  const [duplicating, setDuplicating] = useState(false);
  const [applyingInterpretFPS, setApplyingInterpretFPS] = useState(false);
  const [saving, setSaving] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [importPreviews, setImportPreviews] = useState<ImportPreview[]>([]);
  const [importQueuePage, setImportQueuePage] = useState(1);
  const [workspacePage, setWorkspacePage] = useState(1);
  const [workspaceTotal, setWorkspaceTotal] = useState(0);
  const [selectingAllItems, setSelectingAllItems] = useState(false);
  const [workspaceStats, setWorkspaceStats] = useState<WorkspaceStatsState>({ pending: 0, saved: 0, ready: 0, submitted: 0 });
  const [hasRunningVLMLabel, setHasRunningVLMLabel] = useState(false);
  const [selectedItemID, setSelectedItemID] = useState<string | null>(null);
  const [selectedItemIDs, setSelectedItemIDs] = useState<Set<string>>(() => new Set());
  const [selectedItemCache, setSelectedItemCache] = useState<Record<string, WorkspaceItem>>({});
  const [marqueeRect, setMarqueeRect] = useState<MarqueeRect | null>(null);
  const [batchVLMModalOpen, setBatchVLMModalOpen] = useState(false);
  const [batchVLMProductID, setBatchVLMProductID] = useState("");
  const [batchVLMUseReference, setBatchVLMUseReference] = useState(false);
  const [batchSubmitModalOpen, setBatchSubmitModalOpen] = useState(false);
  const [batchSubmitProductID, setBatchSubmitProductID] = useState("");
  const [batchSubmitValidationItems, setBatchSubmitValidationItems] = useState<WorkspaceItem[]>([]);
  const [batchSubmitChecking, setBatchSubmitChecking] = useState(false);
  const [batchSubmitCheckError, setBatchSubmitCheckError] = useState("");
  const [batchOperation, setBatchOperation] = useState<BatchOperationState | null>(null);
  const lastSubmitProductIDRef = useRef(loadLastSubmitProductID());
  const [submitProductID, setSubmitProductID] = useState<string>(lastSubmitProductIDRef.current);
  const [submitSellingPointIDs, setSubmitSellingPointIDs] = useState<string[]>([]);
  const [useProductReferenceImage, setUseProductReferenceImage] = useState(true);
  const [framesPreviewOpen, setFramesPreviewOpen] = useState(false);
  const [notesModalOpen, setNotesModalOpen] = useState(false);
  const [interpretFPSModalOpen, setInterpretFPSModalOpen] = useState(false);
  const [subtitlesVisible, setSubtitlesVisible] = useState(true);
  const [activeSubtitleSegmentIndex, setActiveSubtitleSegmentIndex] = useState<number | null>(null);
  const [subtitleEditingIndex, setSubtitleEditingIndex] = useState<number | null>(null);
  const [subtitleEditingText, setSubtitleEditingText] = useState("");
  const [savingSubtitle, setSavingSubtitle] = useState(false);
  const [notesDraft, setNotesDraft] = useState("");
  const cancelledImportIDsRef = useRef(new Set<string>());
  const initializedEditorItemIDRef = useRef<string | null>(null);
  const selectedItemIDRef = useRef<string | null>(null);
  const selectedItemIDsRef = useRef<Set<string>>(new Set());
  const selectedItemCacheRef = useRef<Record<string, WorkspaceItem>>({});
  const selectionAnchorIDRef = useRef<string | null>(null);
  const marqueeStartRef = useRef<MarqueeStart | null>(null);
  const marqueeAutoScrollFrameRef = useRef<number | null>(null);
  const batchSubmitCheckingRef = useRef(false);
  const preprocessBoardRef = useRef<HTMLElement | null>(null);
  const assetCardRefs = useRef(new Map<string, HTMLButtonElement>());
  const preprocessWorkbenchRef = useRef<HTMLDivElement | null>(null);
  const localAgentProbeGenerationRef = useRef(0);
  const [form] = Form.useForm();
  const watchedSourceType = Form.useWatch("source_type", form);
  const watchedSourceInMs = Form.useWatch("source_in_ms", form) ?? 0;
  const watchedSourceOutMs = Form.useWatch("source_out_ms", form) ?? 0;
  const watchedInterpretFPS = Boolean(Form.useWatch("interpret_fps_enabled", form));
  const watchedUseOriginalAudio = Boolean(Form.useWatch("use_original_audio", form));
  const watchedPlaybackFPS = Number(Form.useWatch("playback_fps", form) ?? 25);
  const watchedTranscriptSegments = (Form.useWatch("transcript_segments", form) ?? []) as WorkspaceTranscriptSegment[];
  const selectedSubmitProduct = useMemo(
    () => products.find((product) => product.id === submitProductID) ?? null,
    [products, submitProductID]
  );
  const selectedProductReferenceImage = productReferenceImage(selectedSubmitProduct);
  const selectedBatchItems = useMemo(
    () => Array.from(selectedItemIDs).map((id) => selectedItemCache[id]).filter((item): item is WorkspaceItem => !!item),
    [selectedItemCache, selectedItemIDs]
  );
  const selectedBatchVLMProduct = useMemo(
    () => products.find((product) => product.id === batchVLMProductID) ?? null,
    [batchVLMProductID, products]
  );
  const selectedBatchVLMReferenceImage = productReferenceImage(selectedBatchVLMProduct);
  const batchSubmitItems = batchSubmitValidationItems.length > 0 ? batchSubmitValidationItems : selectedBatchItems;
  const batchSubmitBlockers = useMemo(
    () => batchSubmitItems.map((item) => ({ item, reason: batchSubmitVLMBlocker(item, batchSubmitProductID) }))
      .filter((entry): entry is { item: WorkspaceItem; reason: BatchSubmitVLMBlocker } => entry.reason !== null),
    [batchSubmitItems, batchSubmitProductID]
  );
  const batchSubmitVLMStats = useMemo(() => ({
    ready: batchSubmitItems.filter((item) => item.status !== "submitted" && batchSubmitVLMBlocker(item, batchSubmitProductID) === null).length,
    notStarted: batchSubmitBlockers.filter((entry) => entry.reason === "not_started").length,
    processing: batchSubmitBlockers.filter((entry) => entry.reason === "queued" || entry.reason === "running").length,
    failedOrStale: batchSubmitBlockers.filter((entry) => !["not_started", "queued", "running"].includes(entry.reason)).length
  }), [batchSubmitBlockers, batchSubmitItems, batchSubmitProductID]);
  const importQueueStats = useMemo(() => ({
    waiting: importPreviews.filter((item) => item.status === "waiting").length,
    importing: importPreviews.filter((item) => item.status === "importing").length,
    completed: importPreviews.filter((item) => item.status === "completed").length,
    failed: importPreviews.filter((item) => item.status === "failed").length
  }), [importPreviews]);
  const pagedImportPreviews = useMemo(() => {
    const start = (importQueuePage - 1) * importQueuePageSize;
    return importPreviews.slice(start, start + importQueuePageSize);
  }, [importPreviews, importQueuePage]);

  useEffect(() => {
    const pageCount = Math.max(1, Math.ceil(importPreviews.length / importQueuePageSize));
    if (importQueuePage > pageCount) {
      setImportQueuePage(pageCount);
    }
  }, [importPreviews.length, importQueuePage]);

  useEffect(() => {
    selectedItemIDsRef.current = selectedItemIDs;
  }, [selectedItemIDs]);

  useEffect(() => {
    selectedItemCacheRef.current = selectedItemCache;
  }, [selectedItemCache]);

  const applyWorkspaceSelection = (nextIDs: Set<string>, visibleItems = items) => {
    const normalized = new Set(nextIDs);
    selectedItemIDsRef.current = normalized;
    setSelectedItemIDs(normalized);
    setSelectedItemCache((current) => {
      const next = { ...current };
      visibleItems.forEach((item) => {
        if (normalized.has(item.id)) {
          next[item.id] = item;
        }
      });
      Object.keys(next).forEach((id) => {
        if (!normalized.has(id)) {
          delete next[id];
        }
      });
      selectedItemCacheRef.current = next;
      return next;
    });
  };

  const toggleSelectAllWorkspaceItems = async () => {
    if (selectingAllItems || workspaceTotal === 0) {
      return;
    }
    if (selectedItemIDsRef.current.size === workspaceTotal) {
      applyWorkspaceSelection(new Set());
      selectionAnchorIDRef.current = null;
      return;
    }

    setSelectingAllItems(true);
    try {
      const allItems: WorkspaceItem[] = [];
      const seen = new Set<string>();
      let page = 1;
      let total = workspaceTotal;
      while (seen.size < total) {
        const response = await localAgentRequest<WorkspaceListResponse>(`/workspace/items?page=${page}&page_size=100`);
        total = response.total ?? total;
        const pageItems = response.items ?? [];
        if (pageItems.length === 0) {
          break;
        }
        pageItems.forEach((item) => {
          if (!seen.has(item.id)) {
            seen.add(item.id);
            allItems.push(item);
          }
        });
        page += 1;
      }
      if (seen.size !== total) {
        throw new Error(`只读取到 ${seen.size}/${total} 项素材`);
      }
      setWorkspaceTotal(total);
      applyWorkspaceSelection(new Set(seen), allItems);
      selectionAnchorIDRef.current = null;
    } catch (error) {
      message.error(error instanceof Error ? `全选失败：${error.message}` : "全选素材失败");
    } finally {
      setSelectingAllItems(false);
    }
  };

  const replaceWorkspaceItem = (nextItem: WorkspaceItem, previousItem?: WorkspaceItem | null) => {
    setItems((current) => current.map((item) => (item.id === nextItem.id ? nextItem : item)));
    if (selectedItemIDsRef.current.has(nextItem.id)) {
      setSelectedItemCache((current) => {
        const next = { ...current, [nextItem.id]: nextItem };
        selectedItemCacheRef.current = next;
        return next;
      });
    }
    if (previousItem && previousItem.status !== nextItem.status) {
      const previousKey = workspaceStatKeys[previousItem.status];
      const nextKey = workspaceStatKeys[nextItem.status];
      setWorkspaceStats((current) => ({
        ...current,
        [previousKey]: Math.max(0, current[previousKey] - 1),
        [nextKey]: current[nextKey] + 1
      }));
    }
  };

  useEffect(() => {
    if (watchedSourceType === "talking_head" && watchedInterpretFPS) {
      form.setFieldsValue({
        interpret_fps_enabled: false,
        playback_fps: 25
      });
    }
  }, [form, watchedInterpretFPS, watchedSourceType]);

  useEffect(() => {
    setUseProductReferenceImage(watchedSourceType === "visual_only" && !!selectedProductReferenceImage);
  }, [selectedProductReferenceImage, watchedSourceType]);

  const loadItems = async (page = workspacePage) => {
    setLoading(true);
    try {
      const response = await localAgentRequest<WorkspaceListResponse>(`/workspace/items?page=${page}&page_size=${workspacePageSize}`);
      const total = response.total ?? response.items?.length ?? 0;
      const pageCount = Math.max(1, Math.ceil(total / workspacePageSize));
      if (page > pageCount) {
        setWorkspacePage(pageCount);
        return;
      }
      const loadedItems = response.items ?? [];
      setItems(loadedItems);
      setSelectedItemCache((current) => {
        const next = { ...current };
        loadedItems.forEach((item) => {
          if (selectedItemIDsRef.current.has(item.id)) {
            next[item.id] = item;
          }
        });
        selectedItemCacheRef.current = next;
        return next;
      });
      setWorkspaceTotal(total);
      setWorkspaceStats(response.stats ? {
        pending: response.stats.pending,
        saved: response.stats.saved,
        ready: response.stats.ready_to_submit,
        submitted: response.stats.submitted
      } : {
        pending: (response.items ?? []).filter((item) => item.status === "pending").length,
        saved: (response.items ?? []).filter((item) => item.status === "saved").length,
        ready: (response.items ?? []).filter((item) => item.status === "ready_to_submit").length,
        submitted: (response.items ?? []).filter((item) => item.status === "submitted").length
      });
      setHasRunningVLMLabel(Boolean(response.has_running_vlm ?? (response.items ?? []).some(
        (item) => item.vlm_status === "queued" || item.vlm_status === "running"
      )));
    } catch {
      const probe = await probeLocalAgent();
      setLocalAgent(probe);
      if (probe.state === "ready") {
        message.error("加载预处理工作区失败");
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (localAgent.state === "ready") {
      void loadItems(workspacePage);
    }
  }, [localAgent.state, workspacePage]);

  const loadLocalAgentRelease = async () => {
    try {
      const release = await apiRequest<LocalAgentRelease>("/api/client-releases/local-agent/latest");
      setLocalAgentRelease(release);
      setLocalAgentReleaseError("");
    } catch (error) {
      setLocalAgentRelease(null);
      setLocalAgentReleaseError(error instanceof Error ? error.message : "安装包暂不可用");
    }
  };

  const checkLocalAgent = async (showChecking = true) => {
    const generation = ++localAgentProbeGenerationRef.current;
    setLaunchingLocalAgent(false);
    if (showChecking) {
      setLocalAgent({ state: "checking" });
    }
    const probe = await probeLocalAgent();
    if (generation !== localAgentProbeGenerationRef.current) {
      return false;
    }
    setLocalAgent(probe);
    if (probe.state !== "ready" && !localAgentRelease) {
      void loadLocalAgentRelease();
    }
    return probe.state === "ready";
  };

  const pollForLocalAgent = async (durationMs: number) => {
    const generation = ++localAgentProbeGenerationRef.current;
    setLaunchingLocalAgent(true);
    const deadline = Date.now() + durationMs;
    try {
      while (Date.now() < deadline) {
        await new Promise((resolve) => window.setTimeout(resolve, 1500));
        if (generation !== localAgentProbeGenerationRef.current) {
          return;
        }
        const probe = await probeLocalAgent();
        if (generation !== localAgentProbeGenerationRef.current) {
          return;
        }
        if (probe.state === "ready") {
          setLocalAgent(probe);
          return;
        }
        if (probe.state === "incompatible" || probe.state === "incomplete") {
          setLocalAgent(probe);
          return;
        }
      }
      const probe = await probeLocalAgent();
      if (generation === localAgentProbeGenerationRef.current) {
        setLocalAgent(probe);
      }
    } finally {
      if (generation === localAgentProbeGenerationRef.current) {
        setLaunchingLocalAgent(false);
      }
    }
  };

  const startLocalAgent = () => {
    launchLocalAgent();
    void pollForLocalAgent(60_000);
  };

  const downloadLocalAgent = () => {
    void pollForLocalAgent(5 * 60_000);
  };

  useEffect(() => {
    void checkLocalAgent();
    return () => {
      localAgentProbeGenerationRef.current += 1;
    };
  }, []);

  useEffect(() => {
    void (async () => {
      try {
        const result = await authenticatedApiRequest<Product[]>("/api/products", token);
        setProducts(result ?? []);
      } catch (error) {
        message.error(error instanceof Error ? error.message : "加载产品列表失败");
      }
    })();
  }, [token]);

  const selectedIndex = useMemo(
    () => items.findIndex((item) => item.id === selectedItemID),
    [items, selectedItemID]
  );
  const selectedItem = selectedIndex >= 0 ? items[selectedIndex] : null;
  useEffect(() => {
    selectedItemIDRef.current = selectedItemID;
  }, [selectedItemID]);

  useEffect(() => {
    if (!hasRunningVLMLabel) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadItems(workspacePage);
    }, 2000);
    return () => window.clearInterval(timer);
  }, [hasRunningVLMLabel, workspacePage]);

  useEffect(() => {
    if (!selectedItem) {
      initializedEditorItemIDRef.current = null;
      return;
    }
    const editorSyncKey = sourceIdentityKey(selectedItem);
    if (initializedEditorItemIDRef.current === editorSyncKey) {
      return;
    }
    initializedEditorItemIDRef.current = editorSyncKey;
    const sourceOutMs =
      selectedItem.source_out_ms > 0
        ? selectedItem.source_out_ms
        : selectedItem.probe.duration_ms ?? 0;
    form.setFieldsValue({
      asset_name: selectedItem.asset_name ?? "",
      source_type: selectedItem.source_type || defaultSourceType,
      use_original_audio: Boolean(selectedItem.use_original_audio),
      source_in_ms: selectedItem.source_in_ms ?? 0,
      source_out_ms: sourceOutMs,
      interpret_fps_enabled: Boolean(selectedItem.interpret_fps_enabled),
      playback_fps: selectedItem.playback_fps || 25,
      transcript: selectedItem.transcript ?? "",
      transcript_segments: selectedItem.transcript_segments ?? [],
      reviewer_notes: selectedItem.reviewer_notes ?? ""
    });
    setSubmitProductID(selectedItem.product_id || lastSubmitProductIDRef.current);
    setSubmitSellingPointIDs([]);
  }, [form, selectedItem]);

  useEffect(() => {
    setSubtitlesVisible(true);
    setActiveSubtitleSegmentIndex(null);
    setSubtitleEditingIndex(null);
    setSubtitleEditingText("");
  }, [selectedItem?.id]);

  useEffect(() => {
    const root = preprocessWorkbenchRef.current;
    if (!root || !selectedItem) {
      return;
    }

    disableButtonTabStops(root);
    const observer = new MutationObserver(() => disableButtonTabStops(root));
    observer.observe(root, {
      childList: true,
      subtree: true
    });
    return () => observer.disconnect();
  }, [selectedItem]);

  const preventPreprocessButtonFocus = (event: React.MouseEvent<HTMLElement>) => {
    if (isButtonElementTarget(event.target)) {
      event.preventDefault();
    }
  };

  const blurPreprocessButtonFocus = (event: React.FocusEvent<HTMLElement>) => {
    const button = isButtonElementTarget(event.target);
    if (button instanceof HTMLElement) {
      button.blur();
    }
  };

  useEffect(() => {
    if (!submitProductID) {
      setSellingPoints([]);
      setSubmitSellingPointIDs([]);
      return;
    }

    void (async () => {
      try {
        const result = await authenticatedApiRequest<SellingPoint[]>(`/api/products/${submitProductID}/selling-points`, token);
        setSellingPoints(result ?? []);
        setSubmitSellingPointIDs((current) =>
          current.filter((item) => (result ?? []).some((sellingPoint) => sellingPoint.id === item))
        );
      } catch (error) {
        setSellingPoints([]);
        setSubmitSellingPointIDs([]);
        message.error(error instanceof Error ? error.message : "加载卖点列表失败");
      }
    })();
  }, [submitProductID, token]);

  const clearImportPreviews = () => {
    setImportPreviews((current) => {
      current.forEach((item) => cancelledImportIDsRef.current.add(item.id));
      return [];
    });
    setImportQueuePage(1);
  };

  const closeImportModal = () => {
    setImportModalOpen(false);
  };

  const selectImportFiles = (files: File[]) => {
    cancelledImportIDsRef.current.clear();
    setImportQueuePage(1);
    setImportPreviews(files.map((file, index) => ({
      id: `${file.name}-${file.size}-${file.lastModified}-${index}`,
      file,
      status: "waiting"
    })));
  };

  const removeImportPreview = (id: string) => {
    cancelledImportIDsRef.current.add(id);
    setImportPreviews((current) => current.filter((item) => item.id !== id || item.status === "importing"));
  };

  const importFiles = async (onlyIDs?: string[]) => {
    if (importing) {
      return;
    }
    const selectedIDs = onlyIDs ? new Set(onlyIDs) : null;
    const queue = importPreviews.filter((item) =>
      (item.status === "waiting" || item.status === "failed") && (!selectedIDs || selectedIDs.has(item.id))
    );
    if (queue.length === 0) {
      message.warning("请先选择原始视频文件");
      return;
    }
    setImporting(true);
    let cursor = 0;
    let completed = 0;
    let failed = 0;
    let importedTotal = workspaceTotal;
    const updateQueueItem = (id: string, values: Partial<ImportPreview>) => {
      setImportPreviews((current) => current.map((item) => item.id === id ? { ...item, ...values } : item));
    };
    const worker = async () => {
      while (cursor < queue.length) {
        const preview = queue[cursor];
        cursor += 1;
        if (cancelledImportIDsRef.current.has(preview.id)) {
          continue;
        }
        updateQueueItem(preview.id, { status: "importing", error: undefined });
        const body = new FormData();
        body.append("files", preview.file);
        try {
          const response = await localAgentRequest<WorkspaceImportResponse>("/workspace/import", {
            method: "POST",
            body
          });
          const importedItem = response.items?.[0];
          if (!importedItem) {
            throw new Error("Local Agent 未返回导入结果");
          }
          completed += 1;
          importedTotal += 1;
          updateQueueItem(preview.id, { status: "completed" });
          if (Math.ceil(importedTotal / workspacePageSize) === workspacePage) {
            setItems((current) => current.some((item) => item.id === importedItem.id)
              ? current
              : [...current, importedItem].slice(0, workspacePageSize));
          }
          setWorkspaceTotal((current) => current + 1);
          setWorkspaceStats((current) => ({ ...current, pending: current.pending + 1 }));
        } catch (error) {
          failed += 1;
          updateQueueItem(preview.id, {
            status: "failed",
            error: error instanceof Error ? error.message : "导入失败"
          });
        }
      }
    };
    try {
      await Promise.all(Array.from({ length: Math.min(importConcurrency, queue.length) }, () => worker()));
      const targetPage = Math.max(1, Math.ceil(importedTotal / workspacePageSize));
      if (targetPage !== workspacePage) {
        setWorkspacePage(targetPage);
      } else {
        await loadItems(targetPage);
      }
      if (failed > 0) {
        message.warning(`已导入 ${completed} 个，失败 ${failed} 个`);
      } else {
        message.success(`已导入 ${completed} 个原始视频`);
      }
    } finally {
      setImporting(false);
    }
  };

  const saveDraft = async () => {
    if (!selectedItem) {
      return;
    }
    const values = await form.validateFields();
    setSaving(true);
    try {
      const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(values)
      });
      replaceWorkspaceItem(response.item, selectedItem);
      message.success("本地草稿已保存");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存草稿失败");
    } finally {
      setSaving(false);
    }
  };

  const syncASRSelection = async (item: WorkspaceItem) => {
    const values = form.getFieldsValue([
      "asset_name",
      "source_type",
      "use_original_audio",
      "source_in_ms",
      "source_out_ms",
      "interpret_fps_enabled",
      "playback_fps",
      "transcript",
      "transcript_segments",
      "reviewer_notes"
    ]);
    const range = clampCurrentSourceRange(
      Number(values.source_in_ms ?? item.source_in_ms ?? 0),
      Number(values.source_out_ms ?? item.source_out_ms ?? item.probe.duration_ms ?? 0),
      item
    );
    const sourceType = values.source_type || item.source_type || defaultSourceType;
    if (sourceType !== "talking_head") {
      throw new Error("仅口播素材支持转写");
    }
    if (!item.probe.has_audio) {
      throw new Error("当前素材未检测到音频，无法转写");
    }
    if (range.sourceOutMs <= range.sourceInMs) {
      throw new Error("请先设置有效的裁切入点和出点");
    }

    form.setFieldsValue({
      source_type: sourceType,
      source_in_ms: range.sourceInMs,
      source_out_ms: range.sourceOutMs
    });
    const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${item.id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        ...values,
        source_type: sourceType,
        source_in_ms: range.sourceInMs,
        source_out_ms: range.sourceOutMs
      })
    });
    replaceWorkspaceItem(response.item, item);
    return response.item;
  };

  const persistASRSubtitleSegments = async (item: WorkspaceItem, segments: WorkspaceTranscriptSegment[]) => {
    const transcript = transcriptTextFromSegments(segments);
    const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${item.id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        asset_name: item.asset_name ?? "",
        source_type: item.source_type ?? defaultSourceType,
        use_original_audio: Boolean(item.use_original_audio),
        source_in_ms: item.source_in_ms,
        source_out_ms: item.source_out_ms,
        interpret_fps_enabled: Boolean(item.interpret_fps_enabled),
        playback_fps: item.playback_fps || 25,
        transcript,
        transcript_segments: segments,
        reviewer_notes: item.reviewer_notes ?? ""
      })
    });
    replaceWorkspaceItem(response.item, item);
    if (selectedItemIDRef.current === response.item.id) {
      form.setFieldsValue({
        transcript: response.item.transcript ?? transcript,
        transcript_segments: response.item.transcript_segments ?? segments
      });
    }
    return response.item;
  };

  const runTranscription = async () => {
    if (!selectedItem || transcribingItemID) {
      return;
    }

    const itemID = selectedItem.id;
    setTranscribingItemID(itemID);
    try {
      const syncedItem = await syncASRSelection(selectedItem);
      const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${itemID}/transcribe`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          source_in_ms: syncedItem.source_in_ms,
          source_out_ms: syncedItem.source_out_ms,
          server_base_url: window.location.origin,
          auth_token: token
        })
      });
      replaceWorkspaceItem(response.item, syncedItem);
      const segments = response.item.asr_draft?.segments ?? [];
      if (segments.length === 0) {
        throw new Error("语音识别未返回可用句段");
      }
      await persistASRSubtitleSegments(response.item, segments);
      if (selectedItemIDRef.current === itemID) {
        setActiveSubtitleSegmentIndex(0);
        setSubtitlesVisible(true);
      }
      message.success("当前选区转写完成，字幕已叠加到预览画面");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "当前选区转写失败");
    } finally {
      setTranscribingItemID((current) => (current === itemID ? null : current));
    }
  };

  const startTranscription = () => {
    if (!selectedItem || transcribingItemID) {
      return;
    }
    void runTranscription();
  };

  const prepareItem = async () => {
    if (!selectedItem) {
      return;
    }
    const values = await form.validateFields();
    setPreparing(true);
    try {
      await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(values)
      });
      const prepared = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}/prepare`, {
        method: "POST"
      });
      replaceWorkspaceItem(prepared.item, selectedItem);
      setSelectedItemID(prepared.item.id);
      message.success("本地预处理已完成，当前状态为待提交");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "执行预处理失败");
    } finally {
      setPreparing(false);
    }
  };

  const previewFrames = async () => {
    if (!selectedItem) {
      return;
    }
    const values = form.getFieldsValue(["source_in_ms", "source_out_ms"]);
    const range = clampCurrentSourceRange(
      Number(values.source_in_ms ?? selectedItem.source_in_ms ?? 0),
      Number(values.source_out_ms ?? selectedItem.source_out_ms ?? selectedItem.probe.duration_ms ?? 0),
      selectedItem
    );
    const { sourceInMs, sourceOutMs } = range;
    if (!Number.isFinite(sourceInMs) || !Number.isFinite(sourceOutMs) || sourceOutMs <= sourceInMs) {
      message.warning("请先设置有效的裁切入点和出点");
      return;
    }

    form.setFieldsValue({
      source_in_ms: sourceInMs,
      source_out_ms: sourceOutMs
    });

    setPreviewingFrames(true);
    try {
      const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}/preview-frames`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          source_in_ms: Math.round(sourceInMs),
          source_out_ms: Math.round(sourceOutMs)
        })
      });
      replaceWorkspaceItem(response.item, selectedItem);
      setFramesPreviewOpen(true);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "三帧抽样失败");
    } finally {
      setPreviewingFrames(false);
    }
  };

  const startVLMLabel = async () => {
    if (!selectedItem) {
      return;
    }
    const values = form.getFieldsValue(["source_type", "source_in_ms", "source_out_ms"]);
    const range = clampCurrentSourceRange(
      Number(values.source_in_ms ?? selectedItem.source_in_ms ?? 0),
      Number(values.source_out_ms ?? selectedItem.source_out_ms ?? selectedItem.probe.duration_ms ?? 0),
      selectedItem
    );
    const { sourceInMs, sourceOutMs } = range;
    const sourceType = values.source_type || selectedItem.source_type || defaultSourceType;
    const productName =
      sourceType === "visual_only"
        ? selectedSubmitProduct?.name ?? ""
        : "";
    const productReferenceImageDataURL =
      sourceType === "visual_only" && useProductReferenceImage ? selectedProductReferenceImage : "";
    if (!Number.isFinite(sourceInMs) || !Number.isFinite(sourceOutMs) || sourceOutMs <= sourceInMs) {
      message.warning("请先设置有效的裁切入点和出点");
      return;
    }
    form.setFieldsValue({
      source_in_ms: sourceInMs,
      source_out_ms: sourceOutMs
    });

    setStartingVLMLabel(true);
    try {
      const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}/vlm-label`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          product_id: selectedSubmitProduct?.id ?? "",
          source_type: sourceType,
          product_name: productName,
          product_reference_image_data_url: productReferenceImageDataURL,
          source_in_ms: Math.round(sourceInMs),
          source_out_ms: Math.round(sourceOutMs),
          server_base_url: window.location.origin,
          auth_token: token
        })
      });
      replaceWorkspaceItem(response.item, selectedItem);
      setHasRunningVLMLabel(true);
      message.success("VLM 标注已开始，可继续处理其他视频");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "VLM 标注启动失败");
    } finally {
      setStartingVLMLabel(false);
    }
  };

  const duplicateItem = async () => {
    if (!selectedItem) {
      return;
    }
    const values = await form.validateFields();
    setDuplicating(true);
    try {
      const saved = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(values)
      });
      replaceWorkspaceItem(saved.item, selectedItem);
      const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}/duplicate`, {
        method: "POST"
      });
      const total = workspaceTotal + 1;
      const targetPage = Math.max(1, Math.ceil(total / workspacePageSize));
      setWorkspaceTotal(total);
      setWorkspaceStats((current) => ({
        ...current,
        [workspaceStatKeys[response.item.status]]: current[workspaceStatKeys[response.item.status]] + 1
      }));
      if (targetPage === workspacePage) {
        setItems((current) => [...current, response.item].slice(0, workspacePageSize));
      } else {
        setWorkspacePage(targetPage);
      }
      setSelectedItemID(response.item.id);
      message.success("已从当前原始视频派生一个新的 clean shot 条目");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "派生新片段失败");
    } finally {
      setDuplicating(false);
    }
  };

  const clearWorkspace = async () => {
    setClearing(true);
    try {
      await localAgentRequest("/workspace/clear", { method: "POST" });
      if (workspacePage !== 1) {
        setWorkspacePage(1);
      } else {
        await loadItems(1);
      }
      setSelectedItemID(null);
      applyWorkspaceSelection(new Set(), []);
      setSubmitSellingPointIDs([]);
      message.success("本地预处理工作区已清空");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "清空工作区失败");
    } finally {
      setClearing(false);
    }
  };

  const submitItem = async () => {
    if (!selectedItem) {
      return;
    }
    if (selectedItem.status !== "ready_to_submit") {
      message.warning("只有待提交项才能正式入库");
      return;
    }
    if (!submitProductID) {
      message.warning("请先选择产品");
      return;
    }

    setSubmitting(true);
    try {
      const uploadToken = await authenticatedApiRequest<UploadToken>("/api/uploads/tokens", token, {
        method: "POST",
        body: JSON.stringify({ product_id: submitProductID })
      });

      const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}/submit`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          product_id: submitProductID,
          upload_url: `${window.location.origin}/api/uploads/clean-shot`,
          upload_token: uploadToken.token,
          selling_point_ids: submitSellingPointIDs,
          use_original_audio: watchedUseOriginalAudio
        })
      });

      replaceWorkspaceItem(response.item, selectedItem);
      setSelectedItemID(response.item.id);
      message.success("素材已正式提交入库");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "提交入库失败");
    } finally {
      setSubmitting(false);
    }
  };

  const selectSubmitProduct = (productID: string) => {
    setSubmitProductID(productID);
    setSubmitSellingPointIDs([]);
    lastSubmitProductIDRef.current = productID;
    persistLastSubmitProductID(productID);
  };

  const selectedItemsSnapshot = () => Array.from(selectedItemIDsRef.current)
    .map((id) => selectedItemCacheRef.current[id])
    .filter((item): item is WorkspaceItem => !!item);

  const commonSelectedProductID = (selected: WorkspaceItem[]) => {
    const productIDs = Array.from(new Set(selected.map((item) => item.product_id).filter(Boolean)));
    return productIDs.length === 1 ? productIDs[0] ?? "" : "";
  };

  const fetchWorkspaceItem = async (itemID: string) => {
    const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${itemID}`);
    return response.item;
  };

  const refreshBatchSubmitValidation = async (notifyError = false) => {
    if (batchSubmitCheckingRef.current) {
      return null;
    }
    const itemIDs = Array.from(selectedItemIDsRef.current);
    if (itemIDs.length === 0) {
      setBatchSubmitValidationItems([]);
      return [];
    }
    batchSubmitCheckingRef.current = true;
    setBatchSubmitChecking(true);
    setBatchSubmitCheckError("");
    const refreshed: WorkspaceItem[] = [];
    let firstError = "";
    await runWithConcurrency(itemIDs, batchRequestConcurrency, async (itemID) => {
      try {
        const item = await fetchWorkspaceItem(itemID);
        refreshed.push(item);
        replaceWorkspaceItem(item);
      } catch (error) {
        firstError ||= error instanceof Error ? error.message : "读取素材状态失败";
      }
    });
    refreshed.sort((left, right) => itemIDs.indexOf(left.id) - itemIDs.indexOf(right.id));
    setBatchSubmitValidationItems(refreshed);
    setBatchSubmitCheckError(firstError);
    batchSubmitCheckingRef.current = false;
    setBatchSubmitChecking(false);
    if (firstError && notifyError) {
      message.error(`无法校验全部素材：${firstError}`);
    }
    return firstError ? null : refreshed;
  };

  const updateBatchOutcome = (outcome: "succeeded" | "failed" | "skipped") => {
    setBatchOperation((current) => current ? {
      ...current,
      completed: current.completed + 1,
      [outcome]: current[outcome] + 1
    } : current);
  };

  const openBatchVLM = () => {
    if (batchOperation?.running) {
      message.warning("请等待当前批量操作完成");
      return;
    }
    const selected = selectedItemsSnapshot();
    if (selected.length === 0) {
      message.warning("请先选择素材");
      return;
    }
    const productID = commonSelectedProductID(selected) || lastSubmitProductIDRef.current;
    const product = products.find((item) => item.id === productID);
    setBatchVLMProductID(productID);
    setBatchVLMUseReference(!!productReferenceImage(product));
    setBatchVLMModalOpen(true);
  };

  const startBatchVLM = async () => {
    const product = products.find((item) => item.id === batchVLMProductID);
    if (!product) {
      message.warning("请选择产品");
      return;
    }
    const itemIDs = Array.from(selectedItemIDsRef.current);
    if (itemIDs.length === 0) {
      setBatchVLMModalOpen(false);
      return;
    }

    setBatchVLMModalOpen(false);
    setBatchOperation({ action: "vlm", total: itemIDs.length, completed: 0, succeeded: 0, failed: 0, skipped: 0, running: true });
    let succeeded = 0;
    let failed = 0;
    let firstError = "";
    await runWithConcurrency(itemIDs, batchRequestConcurrency, async (itemID) => {
      try {
        const item = await fetchWorkspaceItem(itemID);
        if (isWorkspaceItemBusy(item)) {
          throw new Error("素材正在进行 VLM 识别");
        }
        const sourceType = item.source_type || defaultSourceType;
        const range = clampCurrentSourceRange(
          item.source_in_ms ?? 0,
          item.source_out_ms || item.probe.duration_ms || 0,
          item
        );
        const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${itemID}/vlm-label`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            product_id: product.id,
            source_type: sourceType,
            product_name: sourceType === "visual_only" ? product.name : "",
            product_reference_image_data_url:
              sourceType === "visual_only" && batchVLMUseReference ? productReferenceImage(product) : "",
            source_in_ms: range.sourceInMs,
            source_out_ms: range.sourceOutMs,
            server_base_url: window.location.origin,
            auth_token: token
          })
        });
        replaceWorkspaceItem(response.item, item);
        succeeded += 1;
        updateBatchOutcome("succeeded");
      } catch (error) {
        failed += 1;
        firstError ||= error instanceof Error ? error.message : "VLM 标注启动失败";
        updateBatchOutcome("failed");
      }
    });
    setBatchOperation((current) => current ? { ...current, running: false } : current);
    setHasRunningVLMLabel(succeeded > 0);
    await loadItems(workspacePage);
    if (failed > 0) {
      message.warning(`批量 VLM 已排队 ${succeeded} 项，失败 ${failed} 项${firstError ? `：${firstError}` : ""}`);
    } else {
      message.success(`已将 ${succeeded} 项素材加入 VLM 队列`);
    }
  };

  const openBatchSubmit = () => {
    if (batchOperation?.running) {
      message.warning("请等待当前批量操作完成");
      return;
    }
    const selected = selectedItemsSnapshot();
    if (selected.length === 0) {
      message.warning("请先选择素材");
      return;
    }
    setBatchSubmitProductID(commonSelectedProductID(selected) || lastSubmitProductIDRef.current);
    setBatchSubmitValidationItems(selected);
    setBatchSubmitCheckError("");
    setBatchSubmitModalOpen(true);
    void refreshBatchSubmitValidation();
  };

  const startBatchSubmit = async () => {
    if (!batchSubmitProductID) {
      message.warning("请选择产品");
      return;
    }
    const itemIDs = Array.from(selectedItemIDsRef.current);
    if (itemIDs.length === 0) {
      setBatchSubmitModalOpen(false);
      return;
    }

    const verifiedItems = await refreshBatchSubmitValidation(true);
    if (!verifiedItems) {
      return;
    }
    const blockers = verifiedItems.filter((item) => batchSubmitVLMBlocker(item, batchSubmitProductID) !== null);
    if (blockers.length > 0) {
      message.warning(`有 ${blockers.length} 项素材的 VLM 未完成或已过期，不能正式提交`);
      return;
    }

    lastSubmitProductIDRef.current = batchSubmitProductID;
    persistLastSubmitProductID(batchSubmitProductID);
    setBatchSubmitModalOpen(false);
    setBatchOperation({ action: "submit", total: itemIDs.length, completed: 0, succeeded: 0, failed: 0, skipped: 0, running: true });
    let succeeded = 0;
    let failed = 0;
    let skipped = 0;
    let firstError = "";
    await runWithConcurrency(itemIDs, batchRequestConcurrency, async (itemID) => {
      try {
        let item = await fetchWorkspaceItem(itemID);
        if (item.status === "submitted") {
          skipped += 1;
          updateBatchOutcome("skipped");
          return;
        }
        if (isWorkspaceItemBusy(item)) {
          throw new Error("素材正在进行 VLM 识别，请完成后再提交");
        }
        if (item.status === "pending" || item.status === "saved") {
          const saved = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${itemID}`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(workspaceSavePayload(item))
          });
          item = saved.item;
          replaceWorkspaceItem(item);
          const prepared = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${itemID}/prepare`, {
            method: "POST"
          });
          item = prepared.item;
          replaceWorkspaceItem(item);
        }
        if (item.status !== "ready_to_submit") {
          throw new Error("素材未进入待提交状态");
        }

        const uploadToken = await authenticatedApiRequest<UploadToken>("/api/uploads/tokens", token, {
          method: "POST",
          body: JSON.stringify({ product_id: batchSubmitProductID })
        });
        const submitted = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${itemID}/submit`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            product_id: batchSubmitProductID,
            upload_url: `${window.location.origin}/api/uploads/clean-shot`,
            upload_token: uploadToken.token,
            selling_point_ids: [],
            use_original_audio: Boolean(item.use_original_audio),
            require_vlm_ready: true
          })
        });
        replaceWorkspaceItem(submitted.item, item);
        succeeded += 1;
        updateBatchOutcome("succeeded");
      } catch (error) {
        failed += 1;
        firstError ||= error instanceof Error ? error.message : "正式提交失败";
        updateBatchOutcome("failed");
      }
    });
    setBatchOperation((current) => current ? { ...current, running: false } : current);
    await loadItems(workspacePage);
    if (failed > 0) {
      message.warning(`正式提交完成：成功 ${succeeded}，失败 ${failed}，跳过 ${skipped}${firstError ? `。${firstError}` : ""}`);
    } else {
      message.success(`正式提交完成：成功 ${succeeded}，跳过 ${skipped}`);
    }
  };

  useEffect(() => {
    if (!batchSubmitModalOpen || batchSubmitVLMStats.processing === 0) {
      return;
    }
    const timer = window.setInterval(() => {
      void refreshBatchSubmitValidation();
    }, 2000);
    return () => window.clearInterval(timer);
  }, [batchSubmitModalOpen, batchSubmitVLMStats.processing, selectedItemIDs.size]);

  const openBatchVLMFromSubmit = () => {
    const product = products.find((item) => item.id === batchSubmitProductID);
    setBatchSubmitModalOpen(false);
    setBatchVLMProductID(batchSubmitProductID);
    setBatchVLMUseReference(!!productReferenceImage(product));
    setBatchVLMModalOpen(true);
  };

  const deleteSelectedItems = () => {
    if (batchOperation?.running) {
      message.warning("请等待当前批量操作完成");
      return;
    }
    const selected = selectedItemsSnapshot();
    if (selected.length === 0) {
      message.warning("请先选择素材");
      return;
    }
    const busy = selected.filter(isWorkspaceItemBusy);
    const deletable = selected.filter((item) => !isWorkspaceItemBusy(item));
    if (deletable.length === 0) {
      message.warning("所选素材正在处理，暂时不能删除");
      return;
    }
    const submittedCount = deletable.filter((item) => item.status === "submitted").length;
    Modal.confirm({
      title: `删除 ${deletable.length} 项本地素材？`,
      content: (
        <div className="preprocess-batch-confirm-copy">
          <p>将删除 Local Agent 中的视频副本、缩略图、抽帧和 clean shot。</p>
          {submittedCount > 0 ? <p>其中 {submittedCount} 项已正式提交，只删除本地副本，不影响服务端素材库。</p> : null}
          {busy.length > 0 ? <p>{busy.length} 项正在处理，将跳过。</p> : null}
        </div>
      ),
      okText: "删除",
      okButtonProps: { danger: true },
      cancelText: "取消",
      async onOk() {
        setBatchOperation({
          action: "delete",
          total: selected.length,
          completed: busy.length,
          succeeded: 0,
          failed: 0,
          skipped: busy.length,
          running: true
        });
        const deletedIDs = new Set<string>();
        let failed = 0;
        let firstError = "";
        await runWithConcurrency(deletable, batchRequestConcurrency, async (item) => {
          try {
            await localAgentRequest(`/workspace/items/${item.id}`, { method: "DELETE" });
            deletedIDs.add(item.id);
            updateBatchOutcome("succeeded");
          } catch (error) {
            failed += 1;
            firstError ||= error instanceof Error ? error.message : "删除失败";
            updateBatchOutcome("failed");
          }
        });
        const nextSelection = new Set(Array.from(selectedItemIDsRef.current).filter((id) => !deletedIDs.has(id)));
        applyWorkspaceSelection(nextSelection, []);
        setItems((current) => current.filter((item) => !deletedIDs.has(item.id)));
        if (selectedItemIDRef.current && deletedIDs.has(selectedItemIDRef.current)) {
          setSelectedItemID(null);
        }
        setBatchOperation((current) => current ? { ...current, running: false } : current);
        await loadItems(workspacePage);
        if (failed > 0) {
          message.warning(`已删除 ${deletedIDs.size} 项，失败 ${failed} 项${firstError ? `：${firstError}` : ""}`);
        } else {
          message.success(`已删除 ${deletedIDs.size} 项本地素材${busy.length > 0 ? `，跳过 ${busy.length} 项` : ""}`);
        }
      }
    });
  };

  const handleBatchContextAction = (key: string) => {
    if (key === "vlm") {
      openBatchVLM();
    } else if (key === "submit") {
      openBatchSubmit();
    } else if (key === "delete") {
      deleteSelectedItems();
    }
  };

  const selectAssetCard = (event: React.MouseEvent<HTMLButtonElement>, item: WorkspaceItem, index: number) => {
    const additive = event.ctrlKey || event.metaKey;
    let next = new Set(selectedItemIDsRef.current);
    if (event.shiftKey && selectionAnchorIDRef.current) {
      const anchorIndex = items.findIndex((candidate) => candidate.id === selectionAnchorIDRef.current);
      if (anchorIndex >= 0) {
        const start = Math.min(anchorIndex, index);
        const end = Math.max(anchorIndex, index);
        if (!additive) {
          next = new Set();
        }
        items.slice(start, end + 1).forEach((candidate) => next.add(candidate.id));
      } else {
        next.add(item.id);
      }
    } else if (additive) {
      if (next.has(item.id)) {
        next.delete(item.id);
      } else {
        next.add(item.id);
      }
    } else {
      next = new Set([item.id]);
    }
    selectionAnchorIDRef.current = item.id;
    applyWorkspaceSelection(next);
  };

  const selectAssetCardForContextMenu = (item: WorkspaceItem) => {
    if (!selectedItemIDsRef.current.has(item.id)) {
      selectionAnchorIDRef.current = item.id;
      applyWorkspaceSelection(new Set([item.id]));
    }
  };

  const stopMarqueeAutoScroll = () => {
    if (marqueeAutoScrollFrameRef.current !== null) {
      window.cancelAnimationFrame(marqueeAutoScrollFrameRef.current);
      marqueeAutoScrollFrameRef.current = null;
    }
  };

  const updateMarqueeSelection = (clientX: number, clientY: number) => {
    const start = marqueeStartRef.current;
    const board = preprocessBoardRef.current;
    if (!start || !board) {
      return;
    }
    const boardBounds = board.getBoundingClientRect();
    const currentX = clientX - boardBounds.left + board.scrollLeft;
    const currentY = clientY - boardBounds.top + board.scrollTop;
    const selectionBounds = {
      left: Math.min(start.boardX, currentX),
      right: Math.max(start.boardX, currentX),
      top: Math.min(start.boardY, currentY),
      bottom: Math.max(start.boardY, currentY)
    };
    setMarqueeRect({
      left: selectionBounds.left,
      top: selectionBounds.top,
      width: selectionBounds.right - selectionBounds.left,
      height: selectionBounds.bottom - selectionBounds.top
    });

    const next = start.append ? new Set(start.initialIDs) : new Set<string>();
    items.forEach((item) => {
      const card = assetCardRefs.current.get(item.id);
      if (!card) {
        return;
      }
      const bounds = card.getBoundingClientRect();
      const cardBounds = {
        left: bounds.left - boardBounds.left + board.scrollLeft,
        right: bounds.right - boardBounds.left + board.scrollLeft,
        top: bounds.top - boardBounds.top + board.scrollTop,
        bottom: bounds.bottom - boardBounds.top + board.scrollTop
      };
      if (
        cardBounds.left < selectionBounds.right && cardBounds.right > selectionBounds.left &&
        cardBounds.top < selectionBounds.bottom && cardBounds.bottom > selectionBounds.top
      ) {
        next.add(item.id);
      }
    });
    applyWorkspaceSelection(next);
  };

  const ensureMarqueeAutoScroll = () => {
    if (marqueeAutoScrollFrameRef.current !== null) {
      return;
    }
    const step = () => {
      marqueeAutoScrollFrameRef.current = null;
      const start = marqueeStartRef.current;
      const board = preprocessBoardRef.current;
      if (!start?.dragged || !board) {
        return;
      }
      const bounds = board.getBoundingClientRect();
      const edgeSize = 56;
      const maxStep = 24;
      let deltaY = 0;
      if (start.latestClientY < bounds.top + edgeSize) {
        deltaY = -Math.ceil(maxStep * Math.min(1, (bounds.top + edgeSize - start.latestClientY) / edgeSize));
      } else if (start.latestClientY > bounds.bottom - edgeSize) {
        deltaY = Math.ceil(maxStep * Math.min(1, (start.latestClientY - (bounds.bottom - edgeSize)) / edgeSize));
      }
      if (deltaY === 0) {
        return;
      }
      const previousScrollTop = board.scrollTop;
      board.scrollTop += deltaY;
      if (board.scrollTop === previousScrollTop) {
        return;
      }
      updateMarqueeSelection(start.latestClientX, start.latestClientY);
      marqueeAutoScrollFrameRef.current = window.requestAnimationFrame(step);
    };
    marqueeAutoScrollFrameRef.current = window.requestAnimationFrame(step);
  };

  useEffect(() => () => stopMarqueeAutoScroll(), []);

  const handleBoardPointerDown = (event: React.PointerEvent<HTMLElement>) => {
    if (event.button !== 0) {
      return;
    }
    const board = preprocessBoardRef.current;
    const target = event.target instanceof Element ? event.target : null;
    if (!board || !target || !board.contains(target)) {
      return;
    }
    if (target?.closest(".preprocess-asset-card, .preprocess-import-fab, .ant-pagination")) {
      return;
    }
    const boardBounds = board.getBoundingClientRect();
    stopMarqueeAutoScroll();
    marqueeStartRef.current = {
      pointerID: event.pointerId,
      clientX: event.clientX,
      clientY: event.clientY,
      latestClientX: event.clientX,
      latestClientY: event.clientY,
      boardX: event.clientX - boardBounds.left + board.scrollLeft,
      boardY: event.clientY - boardBounds.top + board.scrollTop,
      append: event.ctrlKey || event.metaKey || event.shiftKey,
      initialIDs: new Set(selectedItemIDsRef.current),
      dragged: false
    };
    event.currentTarget.setPointerCapture(event.pointerId);
  };

  const handleBoardPointerMove = (event: React.PointerEvent<HTMLElement>) => {
    const start = marqueeStartRef.current;
    const board = preprocessBoardRef.current;
    if (!start || start.pointerID !== event.pointerId || !board) {
      return;
    }
    start.latestClientX = event.clientX;
    start.latestClientY = event.clientY;
    const deltaX = event.clientX - start.clientX;
    const deltaY = event.clientY - start.clientY;
    if (!start.dragged && Math.hypot(deltaX, deltaY) < 4) {
      return;
    }
    start.dragged = true;
    updateMarqueeSelection(event.clientX, event.clientY);
    ensureMarqueeAutoScroll();
  };

  const finishBoardPointerSelection = (event: React.PointerEvent<HTMLElement>) => {
    const start = marqueeStartRef.current;
    if (!start || start.pointerID !== event.pointerId) {
      return;
    }
    stopMarqueeAutoScroll();
    if (!start.dragged && !start.append) {
      applyWorkspaceSelection(new Set());
      selectionAnchorIDRef.current = null;
    }
    marqueeStartRef.current = null;
    setMarqueeRect(null);
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  };

  const cancelBoardPointerSelection = (event: React.PointerEvent<HTMLElement>) => {
    if (marqueeStartRef.current?.pointerID !== event.pointerId) {
      return;
    }
    stopMarqueeAutoScroll();
    marqueeStartRef.current = null;
    setMarqueeRect(null);
  };

  const openNeighbor = (offset: number) => {
    if (selectedIndex < 0) {
      return;
    }
    const target = items[selectedIndex + offset];
    if (target) {
      setSelectedItemID(target.id);
    }
  };

  const updateTrimRange = (startMs: number, endMs: number) => {
    const currentInMs = Number(form.getFieldValue("source_in_ms") ?? 0);
    const currentOutMs = Number(form.getFieldValue("source_out_ms") ?? 0);
    const selectionChanged = currentInMs !== startMs || currentOutMs !== endMs;
    const hasConfirmedSegments = watchedTranscriptSegments.length > 0;
    form.setFieldsValue({
      source_in_ms: startMs,
      source_out_ms: endMs,
      ...(selectionChanged && hasConfirmedSegments
        ? {
            transcript: "",
            transcript_segments: []
          }
        : {})
    });
    if (selectionChanged && hasConfirmedSegments) {
      setActiveSubtitleSegmentIndex(null);
      setSubtitleEditingIndex(null);
      message.warning("调整 I/O 后已清除当前字幕句段，请重新识别并确认");
    }
  };

  const updateSubtitleSegmentRange = (index: number, startMs: number, endMs: number) => {
    const next = watchedTranscriptSegments.map((segment, segmentIndex) =>
      segmentIndex === index ? { ...segment, start_ms: startMs, end_ms: endMs } : segment
    );
    form.setFieldsValue({
      transcript: transcriptTextFromSegments(next),
      transcript_segments: next
    });
    setActiveSubtitleSegmentIndex(index);
  };

  const splitSubtitleSegment = (index: number, splitMs: number) => {
    const segment = watchedTranscriptSegments[index];
    const splitAtMs = Math.round(splitMs);
    if (!segment || splitAtMs <= segment.start_ms || splitAtMs >= segment.end_ms) {
      return;
    }
    const next = [...watchedTranscriptSegments];
    next.splice(
      index,
      1,
      { ...segment, end_ms: splitAtMs },
      { ...segment, start_ms: splitAtMs }
    );
    form.setFieldsValue({
      transcript: transcriptTextFromSegments(next),
      transcript_segments: next
    });
    setActiveSubtitleSegmentIndex(index + 1);
    void persistCurrentSubtitleSegments(next);
  };

  const persistCurrentSubtitleSegments = async (segmentsOverride?: WorkspaceTranscriptSegment[]) => {
    if (!selectedItem || savingSubtitle) {
      return;
    }
    const segments = segmentsOverride ?? ((form.getFieldValue("transcript_segments") ?? []) as WorkspaceTranscriptSegment[]);
    if (segments.length === 0) {
      return;
    }
    const values = form.getFieldsValue([
      "asset_name",
      "source_type",
      "use_original_audio",
      "source_in_ms",
      "source_out_ms",
      "interpret_fps_enabled",
      "playback_fps",
      "reviewer_notes"
    ]);
    const transcript = transcriptTextFromSegments(segments);
    form.setFieldsValue({ transcript, transcript_segments: segments });
    setSavingSubtitle(true);
    try {
      const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...values,
          transcript,
          transcript_segments: segments
        })
      });
      replaceWorkspaceItem(response.item, selectedItem);
      if (selectedItemIDRef.current === response.item.id) {
        form.setFieldsValue({
          transcript: response.item.transcript ?? transcript,
          transcript_segments: response.item.transcript_segments ?? segments
        });
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存字幕失败");
    } finally {
      setSavingSubtitle(false);
    }
  };

  const startInlineSubtitleEdit = (index: number) => {
    const segment = watchedTranscriptSegments[index];
    if (!segment) {
      return;
    }
    setActiveSubtitleSegmentIndex(index);
    setSubtitleEditingIndex(index);
    setSubtitleEditingText(segment.text);
  };

  const cancelInlineSubtitleEdit = () => {
    setSubtitleEditingIndex(null);
    setSubtitleEditingText("");
  };

  const commitInlineSubtitleEdit = () => {
    if (subtitleEditingIndex === null) {
      return;
    }
    const text = subtitleEditingText.trim();
    if (!text) {
      message.warning("字幕文字不能为空");
      return;
    }
    const next = watchedTranscriptSegments.map((segment, index) =>
      index === subtitleEditingIndex ? { ...segment, text } : segment
    );
    form.setFieldsValue({
      transcript: transcriptTextFromSegments(next),
      transcript_segments: next
    });
    setSubtitleEditingIndex(null);
    setSubtitleEditingText("");
    void persistCurrentSubtitleSegments(next);
  };

  const openNotesModal = () => {
    setNotesDraft(form.getFieldValue("reviewer_notes") ?? "");
    setNotesModalOpen(true);
  };

  const saveNotesDraft = () => {
    form.setFieldValue("reviewer_notes", notesDraft);
    setNotesModalOpen(false);
  };

  const openInterpretFPSModal = () => {
    if (watchedSourceType !== "visual_only") {
      message.warning("升格只支持无口播的纯画面素材");
      return;
    }
    if (sourceFPS <= 25) {
      message.warning("源素材帧率需要高于 25fps 才能升格");
      return;
    }
    setInterpretFPSModalOpen(true);
  };

  const sourceFPS = selectedItem?.original_probe?.fps ?? selectedItem?.probe.fps ?? 0;
  const workingFPS = selectedItem?.probe.fps ?? 0;
  const selectedDurationMs = Math.max(0, Number(watchedSourceOutMs) - Number(watchedSourceInMs));
  const interpretFPSAvailable = watchedSourceType === "visual_only" && sourceFPS > 25;
  const interpretSpeedRatio = watchedInterpretFPS && sourceFPS > 0 ? watchedPlaybackFPS / sourceFPS : 1;
  const interpretBaseDurationMs = selectedItem?.interpret_fps_enabled
    ? selectedItem.original_probe?.duration_ms ?? selectedDurationMs
    : selectedDurationMs;
  const interpretedDurationMs =
    watchedInterpretFPS && interpretSpeedRatio > 0 ? Math.round(interpretBaseDurationMs / interpretSpeedRatio) : selectedDurationMs;

  const updateInterpretFPS = (enabled: boolean, playbackFPS = watchedPlaybackFPS) => {
    form.setFieldsValue({
      interpret_fps_enabled: enabled,
      playback_fps: playbackFPS
    });
  };

  const applyInterpretFPSSettings = async () => {
    if (!selectedItem) {
      return;
    }
    if (watchedInterpretFPS) {
      if (!interpretFPSAvailable) {
        message.warning("升格只支持高于 25fps 的纯画面素材");
        return;
      }
      if (watchedPlaybackFPS < 25 || watchedPlaybackFPS >= sourceFPS) {
        message.warning("播放帧率需要大于等于 25fps，且低于源素材帧率");
        return;
      }
    }

    const values = await form.validateFields();
    setApplyingInterpretFPS(true);
    try {
      const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(values)
      });
      replaceWorkspaceItem(response.item, selectedItem);
      const sourceOutMs = response.item.source_out_ms > 0 ? response.item.source_out_ms : response.item.probe.duration_ms ?? 0;
      form.setFieldsValue({
        source_in_ms: response.item.source_in_ms ?? 0,
        source_out_ms: sourceOutMs,
        interpret_fps_enabled: Boolean(response.item.interpret_fps_enabled),
        playback_fps: response.item.playback_fps || 25
      });
      setInterpretFPSModalOpen(false);
      message.success(response.item.interpret_fps_enabled ? "升格工作源已生成" : "已恢复原始工作源");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "应用升格设置失败");
    } finally {
      setApplyingInterpretFPS(false);
    }
  };

  if (localAgent.state !== "ready") {
    const isChecking = localAgent.state === "checking";
    const needsUpdate = localAgent.state === "incompatible" || localAgent.state === "incomplete";
    const installedVersion = "health" in localAgent ? localAgent.health?.version : undefined;
    return (
      <Space direction="vertical" size="middle" className="page-stack preprocess-page-stack">
        <section className="preprocess-workspace-board preprocess-agent-gate">
          <div className="preprocess-agent-gate-content">
            <div className={`preprocess-agent-gate-icon${isChecking ? " is-checking" : ""}`}>
              {isChecking ? <Spin size="small" /> : <MonitorCog size={27} />}
            </div>
            <div className="preprocess-agent-gate-copy">
              <Typography.Title level={4}>
                {isChecking ? "正在连接 Local Agent" : needsUpdate ? "需要更新 Local Agent" : "Local Agent 未启动"}
              </Typography.Title>
              <Typography.Text type="secondary">
                {isChecking
                  ? "正在检查本机预处理服务"
                  : localAgent.state === "incomplete"
                    ? "本地媒体组件不完整，请重新安装完整版本"
                    : needsUpdate
                      ? `当前版本${installedVersion ? ` ${installedVersion}` : ""}与系统不兼容`
                      : launchingLocalAgent
                        ? "等待 Local Agent 启动"
                        : "启动已安装的程序，或下载完整安装包"}
              </Typography.Text>
            </div>
            {!isChecking ? (
              <div className="preprocess-agent-gate-actions">
                {!needsUpdate ? (
                  <Button type="primary" icon={<Power size={16} />} loading={launchingLocalAgent} onClick={startLocalAgent}>
                    启动 Local Agent
                  </Button>
                ) : null}
                <Button
                  type={needsUpdate ? "primary" : "default"}
                  icon={<Download size={16} />}
                  href={localAgentRelease?.download_url}
                  disabled={!localAgentRelease}
                  onClick={downloadLocalAgent}
                >
                  {needsUpdate ? "下载更新" : "下载安装"}
                </Button>
                <Button icon={<RefreshCw size={16} />} onClick={() => void checkLocalAgent()}>
                  重新检测
                </Button>
              </div>
            ) : null}
            {localAgentReleaseError ? (
              <Typography.Text className="preprocess-agent-release-error">{localAgentReleaseError}</Typography.Text>
            ) : null}
            {localAgentRelease ? (
              <Typography.Text className="preprocess-agent-release-version">
                Windows x64 · {localAgentRelease.version}
              </Typography.Text>
            ) : null}
          </div>
        </section>
      </Space>
    );
  }

  return (
    <Space direction="vertical" size="middle" className="page-stack preprocess-page-stack">
      <div className="preprocess-workspace-toolbar">
        <div className="preprocess-workspace-status-group">
          <Typography.Text className="preprocess-workspace-status">
            待处理 {workspaceStats.pending} / 已保存 {workspaceStats.saved} / 待提交 {workspaceStats.ready} / 已入库 {workspaceStats.submitted}
          </Typography.Text>
          {selectedItemIDs.size > 0 ? <Tag color="blue">已选 {selectedItemIDs.size} 项</Tag> : null}
          {batchOperation ? (
            <Typography.Text className={`preprocess-batch-progress${batchOperation.failed > 0 ? " has-error" : ""}`}>
              {batchActionLabels[batchOperation.action]} {batchOperation.completed}/{batchOperation.total}
              {batchOperation.succeeded > 0 ? ` · 成功 ${batchOperation.succeeded}` : ""}
              {batchOperation.failed > 0 ? ` · 失败 ${batchOperation.failed}` : ""}
              {batchOperation.skipped > 0 ? ` · 跳过 ${batchOperation.skipped}` : ""}
            </Typography.Text>
          ) : null}
        </div>
        <Space wrap>
          <Button
            icon={<Check size={16} />}
            onClick={() => void toggleSelectAllWorkspaceItems()}
            loading={selectingAllItems}
            disabled={workspaceTotal === 0}
          >
            {selectedItemIDs.size === workspaceTotal && workspaceTotal > 0 ? "取消全选" : `全选全部 (${workspaceTotal})`}
          </Button>
          <Button icon={<RefreshIcon />} onClick={() => void loadItems()} loading={loading}>
            刷新
          </Button>
          <Button danger icon={<TrashIcon />} loading={clearing} onClick={() => void clearWorkspace()}>
            清空工作区
          </Button>
        </Space>
      </div>

      <section
        ref={preprocessBoardRef}
        className={`preprocess-workspace-board${marqueeRect ? " is-selecting" : ""}`}
        onPointerDown={handleBoardPointerDown}
        onPointerMove={handleBoardPointerMove}
        onPointerUp={finishBoardPointerSelection}
        onPointerCancel={cancelBoardPointerSelection}
      >
        {items.length === 0 ? (
          <div className="preprocess-workspace-empty">
            <Empty description={loading ? "正在加载本地工作区" : "还没有导入视频"} />
            <Typography.Text type="secondary">点击右下角按钮导入原始视频，处理完成前不会进入服务端素材库。</Typography.Text>
          </div>
        ) : (
          <div className="preprocess-asset-list-shell">
            <div className="preprocess-asset-grid">
              {items.map((item, index) => {
                const previewUrl = getWorkspacePreviewUrl(item);
                const selected = selectedItemIDs.has(item.id);
                const vlmStatus = item.vlm_status || "idle";
                return (
                  <Dropdown
                    key={item.id}
                    trigger={["contextMenu"]}
                    menu={{
                      items: [
                        { key: "vlm", icon: <ScanSearch size={15} />, label: "批量VLM" },
                        { key: "submit", icon: <Send size={15} />, label: "正式提交" },
                        { type: "divider" },
                        { key: "delete", icon: <Trash2 size={15} />, label: "删除", danger: true }
                      ],
                      onClick: ({ key }) => handleBatchContextAction(key)
                    }}
                  >
                    <button
                      ref={(element) => {
                        if (element) {
                          assetCardRefs.current.set(item.id, element);
                        } else {
                          assetCardRefs.current.delete(item.id);
                        }
                      }}
                      type="button"
                      className={`preprocess-asset-card${selected ? " is-selected" : ""}`}
                      aria-pressed={selected}
                      onClick={(event) => selectAssetCard(event, item, index)}
                      onDoubleClick={() => setSelectedItemID(item.id)}
                      onContextMenu={() => selectAssetCardForContextMenu(item)}
                      title={item.asset_name || item.original_file_name}
                    >
                      <div className="preprocess-asset-preview">
                        {previewUrl ? (
                          <img loading="lazy" decoding="async" draggable={false} src={previewUrl} alt={item.asset_name || item.original_file_name} />
                        ) : (
                          <span className="preprocess-asset-preview-placeholder"><UploadIcon /></span>
                        )}
                        <Tag color={workspaceStatusColors[item.status]} className="preprocess-asset-status">
                          {workspaceStatusLabels[item.status]}
                        </Tag>
                        <Tag color={sourceTypeColors[item.source_type || defaultSourceType] ?? "default"} className="preprocess-asset-type">
                          {sourceTypeLabels[item.source_type || defaultSourceType] ?? "-"}
                        </Tag>
                        {selected ? <span className="preprocess-asset-selection-mark"><Check size={14} /></span> : null}
                        {vlmStatus !== "idle" ? (
                          <span className={`preprocess-asset-vlm-status is-${vlmStatus}`}>{workspaceVLMStatusLabels[vlmStatus]}</span>
                        ) : null}
                      </div>
                      <div className="preprocess-asset-meta">
                        <Typography.Text className="preprocess-asset-name">
                          {item.asset_name || item.original_file_name}
                        </Typography.Text>
                        <Typography.Text className="preprocess-asset-detail">
                          {formatDuration(item.probe.duration_ms)} · {formatResolution(item.probe.width, item.probe.height)}
                        </Typography.Text>
                        <Typography.Text className="preprocess-asset-detail">{formatDateTime(item.updated_at)}</Typography.Text>
                      </div>
                      {item.last_error ? <div className="preprocess-asset-error">{item.last_error}</div> : null}
                    </button>
                  </Dropdown>
                );
              })}
            </div>
            {workspaceTotal > workspacePageSize ? (
              <Pagination
                className="preprocess-workspace-pagination"
                size="small"
                current={workspacePage}
                pageSize={workspacePageSize}
                total={workspaceTotal}
                showSizeChanger={false}
                onChange={setWorkspacePage}
              />
            ) : null}
          </div>
        )}

        {marqueeRect ? (
          <div
            className="preprocess-selection-marquee"
            style={{ left: marqueeRect.left, top: marqueeRect.top, width: marqueeRect.width, height: marqueeRect.height }}
          />
        ) : null}

        <Button
          type="primary"
          className="preprocess-import-fab"
          onClick={() => setImportModalOpen(true)}
          aria-label="导入原始视频"
          icon={<PlusIcon />}
        >
        </Button>
      </section>

      <Modal
        open={batchVLMModalOpen}
        onCancel={() => setBatchVLMModalOpen(false)}
        title="批量 VLM"
        width={520}
        className="preprocess-batch-modal"
        okText="开始"
        cancelText="取消"
        okButtonProps={{ disabled: !batchVLMProductID }}
        onOk={() => void startBatchVLM()}
      >
        <div className="preprocess-batch-form">
          <div className="preprocess-batch-summary-row">
            <span>已选素材</span>
            <strong>{selectedItemIDs.size} 项</strong>
          </div>
          <label className="preprocess-batch-field">
            <span>产品</span>
            <Select
              value={batchVLMProductID || undefined}
              placeholder="选择用于识别的产品"
              showSearch
              optionFilterProp="label"
              options={products.map((product) => ({ value: product.id, label: product.name }))}
              onChange={(productID) => {
                setBatchVLMProductID(productID);
                const product = products.find((item) => item.id === productID);
                setBatchVLMUseReference(!!productReferenceImage(product));
              }}
            />
          </label>
          <Checkbox
            checked={batchVLMUseReference}
            disabled={!selectedBatchVLMReferenceImage}
            onChange={(event) => setBatchVLMUseReference(event.target.checked)}
          >
            使用产品参考图
          </Checkbox>
          <Typography.Text type="secondary">
            参考图仅用于纯画面素材；口播素材按自身画面识别。VLM 并发由服务端运行控制统一调度。
          </Typography.Text>
        </div>
      </Modal>

      <Modal
        open={batchSubmitModalOpen}
        onCancel={() => setBatchSubmitModalOpen(false)}
        title="正式提交"
        width={520}
        className="preprocess-batch-modal"
        footer={[
          <Button key="cancel" onClick={() => setBatchSubmitModalOpen(false)}>取消</Button>,
          batchSubmitCheckError || batchSubmitBlockers.length > 0 ? (
            <Button
              key="vlm"
              icon={<ScanSearch size={15} />}
              onClick={() => {
                if (batchSubmitCheckError) {
                  void refreshBatchSubmitValidation(true);
                  return;
                }
                const onlyProcessing = batchSubmitBlockers.every((entry) => entry.reason === "queued" || entry.reason === "running");
                if (onlyProcessing) {
                  void refreshBatchSubmitValidation(true);
                } else {
                  openBatchVLMFromSubmit();
                }
              }}
            >
              {batchSubmitCheckError || batchSubmitBlockers.every((entry) => entry.reason === "queued" || entry.reason === "running") ? "刷新状态" : "批量VLM"}
            </Button>
          ) : null,
          <Button
            key="submit"
            type="primary"
            loading={batchSubmitChecking}
            disabled={
              !batchSubmitProductID ||
              batchSubmitChecking ||
              !!batchSubmitCheckError ||
              batchSubmitValidationItems.length !== selectedItemIDs.size ||
              batchSubmitBlockers.length > 0
            }
            onClick={() => void startBatchSubmit()}
          >
            确认提交
          </Button>
        ]}
      >
        <div className="preprocess-batch-form">
          <div className="preprocess-batch-summary-grid">
            <div><span>已选</span><strong>{selectedItemIDs.size}</strong></div>
            <div><span>VLM 已完成</span><strong>{batchSubmitVLMStats.ready}</strong></div>
            <div><span>VLM 处理中</span><strong>{batchSubmitVLMStats.processing}</strong></div>
            <div><span>VLM 需处理</span><strong>{batchSubmitVLMStats.notStarted + batchSubmitVLMStats.failedOrStale}</strong></div>
          </div>
          <label className="preprocess-batch-field">
            <span>产品</span>
            <Select
              value={batchSubmitProductID || undefined}
              placeholder="选择正式提交的产品"
              showSearch
              optionFilterProp="label"
              options={products.map((product) => ({ value: product.id, label: product.name }))}
              onChange={setBatchSubmitProductID}
            />
          </label>
          <div className="preprocess-batch-submit-breakdown">
            <span>自动完成处理 {batchSubmitItems.filter((item) => item.status === "pending" || item.status === "saved").length}</span>
            <span>直接提交 {batchSubmitItems.filter((item) => item.status === "ready_to_submit").length}</span>
            <span>已入库跳过 {batchSubmitItems.filter((item) => item.status === "submitted").length}</span>
          </div>
          {batchSubmitCheckError ? (
            <Alert className="preprocess-batch-vlm-alert" type="error" showIcon message={`状态校验失败：${batchSubmitCheckError}`} />
          ) : batchSubmitBlockers.length > 0 ? (
            <Alert
              className="preprocess-batch-vlm-alert"
              type="warning"
              showIcon
              message={(
                <span className="preprocess-batch-vlm-alert-content">
                  <strong>VLM 未就绪 {batchSubmitBlockers.length}</strong>
                  <span>未执行 {batchSubmitVLMStats.notStarted}</span>
                  <span>处理中 {batchSubmitVLMStats.processing}</span>
                  <span>失败/过期 {batchSubmitVLMStats.failedOrStale}</span>
                </span>
              )}
            />
          ) : batchSubmitChecking ? (
            <Alert className="preprocess-batch-vlm-alert" type="info" showIcon message="正在校验 VLM 状态" />
          ) : (
            <Alert className="preprocess-batch-vlm-alert" type="success" showIcon message="VLM 状态有效，可以正式提交" />
          )}
          <Typography.Text type="secondary">
            未完成处理的素材会自动执行完成处理，再正式提交；保留各素材自己的原声音轨设置。
          </Typography.Text>
        </div>
      </Modal>

      <Modal
        open={importModalOpen}
        onCancel={closeImportModal}
        footer={null}
        width={860}
        title="导入原始视频"
        destroyOnClose={false}
        className="preprocess-import-modal"
      >
        <div className="preprocess-import-panel">
          <label className="preprocess-import-dropzone">
            <input
              type="file"
              accept="video/*"
              multiple
              disabled={importing}
              onChange={(event) => {
                selectImportFiles(Array.from(event.target.files ?? []));
                event.currentTarget.value = "";
              }}
            />
            <UploadIcon />
            <span>选择视频文件</span>
            <Typography.Text type="secondary">支持一次选择多个原始视频，确认后导入本地预处理工作区。</Typography.Text>
          </label>

          {importPreviews.length > 0 ? (
            <div className="preprocess-import-queue">
              <div className="preprocess-import-preview-grid">
                {pagedImportPreviews.map((preview) => (
                  <div key={preview.id} className="preprocess-import-preview-card">
                    <div className="preprocess-import-thumbnail">
                      <UploadIcon />
                    </div>
                    <div className="preprocess-import-info">
                      <Typography.Text className="preprocess-import-name">{preview.file.name}</Typography.Text>
                      <Typography.Text type="secondary">{formatFileSize(preview.file.size)}</Typography.Text>
                      <Tag color={importStatusColors[preview.status]}>{importStatusLabels[preview.status]}</Tag>
                      {preview.error ? <Typography.Text type="danger" ellipsis={{ tooltip: preview.error }}>{preview.error}</Typography.Text> : null}
                    </div>
                    {preview.status === "failed" ? (
                      <Button size="small" disabled={importing} onClick={() => void importFiles([preview.id])}>重试</Button>
                    ) : (
                      <Button
                        size="small"
                        icon={<CloseIcon />}
                        loading={preview.status === "importing"}
                        disabled={preview.status === "importing"}
                        onClick={() => removeImportPreview(preview.id)}
                      >
                        移除
                      </Button>
                    )}
                  </div>
                ))}
              </div>
              {importPreviews.length > importQueuePageSize ? (
                <Pagination
                  className="preprocess-import-pagination"
                  size="small"
                  current={importQueuePage}
                  pageSize={importQueuePageSize}
                  total={importPreviews.length}
                  showSizeChanger={false}
                  onChange={setImportQueuePage}
                />
              ) : null}
            </div>
          ) : (
            <div className="preprocess-import-empty">选择文件后将在这里显示导入队列。</div>
          )}

          <div className="preprocess-import-actions">
            <Typography.Text type="secondary" className="preprocess-import-summary">
              等待 {importQueueStats.waiting} / 进行中 {importQueueStats.importing} / 完成 {importQueueStats.completed} / 失败 {importQueueStats.failed}
            </Typography.Text>
            <Button onClick={clearImportPreviews} disabled={importPreviews.length === 0}>
              清空队列
            </Button>
            <Button onClick={closeImportModal}>
              关闭
            </Button>
            <Button
              type="primary"
              icon={<UploadIcon />}
              loading={importing}
              disabled={importQueueStats.waiting + importQueueStats.failed === 0}
              onClick={() => void importFiles()}
            >
              开始导入 {importQueueStats.waiting + importQueueStats.failed > 0 ? `(${importQueueStats.waiting + importQueueStats.failed})` : ""}
            </Button>
          </div>
        </div>
      </Modal>

      <Modal
        open={!!selectedItem}
        onCancel={() => setSelectedItemID(null)}
        footer={null}
        width="92vw"
        style={{ top: 20 }}
        className="preprocess-modal"
        title={null}
        destroyOnClose={false}
      >
        {selectedItem ? (
          <div
            ref={preprocessWorkbenchRef}
            className="preprocess-workbench-shell"
            onMouseDownCapture={preventPreprocessButtonFocus}
            onFocusCapture={blurPreprocessButtonFocus}
          >
            <Form form={form} layout="vertical" className="preprocess-workbench">
            <Form.Item name="source_in_ms" hidden>
              <Input type="hidden" />
            </Form.Item>
            <Form.Item name="source_out_ms" hidden>
              <Input type="hidden" />
            </Form.Item>
            <Form.Item name="interpret_fps_enabled" hidden valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="use_original_audio" hidden valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="playback_fps" hidden>
              <Input type="hidden" />
            </Form.Item>
            <Form.Item
              name="transcript"
              hidden
              rules={watchedSourceType === "talking_head" ? [{ required: true, message: "口播素材必须填写转写内容" }] : undefined}
            >
              <Input type="hidden" />
            </Form.Item>
            <Form.Item name="transcript_segments" hidden>
              <Input type="hidden" />
            </Form.Item>
            <Form.Item name="reviewer_notes" hidden>
              <Input type="hidden" />
            </Form.Item>

            <div className="preprocess-modal-header">
              <div className="preprocess-header-title">
                <Typography.Text className="preprocess-title-eyebrow">预处理</Typography.Text>
                <Typography.Text className="preprocess-file-name" title={selectedItem.asset_name || selectedItem.original_file_name}>
                  {selectedItem.asset_name || selectedItem.original_file_name}
                </Typography.Text>
                <Tag>{workspaceStatusLabels[selectedItem.status]}</Tag>
                <Tag>{sourceTypeLabels[selectedItem.source_type || defaultSourceType] ?? "-"}</Tag>
                {selectedItem.submitted_asset_id ? <Tag color="success">已入库</Tag> : null}
              </div>

              <div className="preprocess-header-fields">
                <Form.Item name="asset_name" className="preprocess-header-field">
                  <Input className="preprocess-asset-name-input" placeholder="素材名称" />
                </Form.Item>

                <Form.Item
                  name="source_type"
                  className="preprocess-header-field"
                  initialValue={defaultSourceType}
                  rules={[{ required: true, message: "请选择素材类型" }]}
                >
                  <Select
                    popupClassName="preprocess-select-dropdown"
                    placeholder="素材类型"
                    options={sourceTypeOptions}
                  />
                </Form.Item>

                <div className="preprocess-header-submit">
                  <Typography.Text type="secondary">正式提交</Typography.Text>
                  <Select
                    popupClassName="preprocess-select-dropdown"
                    placeholder="请选择产品"
                    status={selectedItem.status === "ready_to_submit" && !submitProductID ? "error" : undefined}
                    value={submitProductID || undefined}
                    onChange={selectSubmitProduct}
                    options={products.map((product) => ({ value: product.id, label: product.name }))}
                  />
                  <Select
                    popupClassName="preprocess-select-dropdown"
                    mode="multiple"
                    allowClear
                    placeholder="卖点"
                    value={submitSellingPointIDs}
                    disabled={!submitProductID}
                    onChange={setSubmitSellingPointIDs}
                    options={sellingPoints.map((item) => ({ value: item.id, label: item.title }))}
                  />
                  <Space size={4} className="preprocess-original-audio-toggle">
                    <Switch
                      size="small"
                      checked={watchedUseOriginalAudio}
                      disabled={!selectedItem.probe.has_audio}
                      onChange={(checked) => form.setFieldsValue({ use_original_audio: checked })}
                    />
                    <Typography.Text type="secondary">保留原声</Typography.Text>
                  </Space>
                </div>
              </div>

              <div className="preprocess-header-actions">
                <Space wrap>
                  <Button onClick={() => openNeighbor(-1)} disabled={selectedIndex <= 0}>
                    上一条
                  </Button>
                  <Button onClick={() => openNeighbor(1)} disabled={selectedIndex < 0 || selectedIndex >= items.length - 1}>
                    下一条
                  </Button>
                  <Button loading={saving} onClick={() => void saveDraft()}>
                    仅保存
                  </Button>
                  <Button loading={duplicating} onClick={() => void duplicateItem()}>
                    新建片段
                  </Button>
                  <Button loading={preparing} onClick={() => void prepareItem()}>
                    完成处理
                  </Button>
                  <Button
                    type="primary"
                    loading={submitting}
                    disabled={selectedItem.status !== "ready_to_submit" || !submitProductID}
                    onClick={() => void submitItem()}
                  >
                    正式提交
                  </Button>
                </Space>
              </div>
            </div>

            {selectedItem.last_error ? <Alert type="error" showIcon message={selectedItem.last_error} /> : null}

            <div className="preprocess-main-stage preprocess-main-stage-single">
              <Card className="preprocess-video-stage" bordered={false}>
                <VideoTrimEditor
                  src={selectedItem.source_url}
                  durationMs={
                    selectedItem.probe.duration_ms && selectedItem.probe.duration_ms > 0
                      ? selectedItem.probe.duration_ms
                      : Math.max(watchedSourceOutMs, selectedItem.source_out_ms, 0)
                  }
                  fps={selectedItem.probe.fps}
                  trimInMs={watchedSourceInMs}
                  trimOutMs={watchedSourceOutMs}
                  hotkeysEnabled={!!selectedItem && !framesPreviewOpen}
                  onTrimChange={(range) => updateTrimRange(range.inMs, range.outMs)}
                  subtitleSegments={watchedSourceType === "talking_head" ? watchedTranscriptSegments : []}
                  subtitlesVisible={subtitlesVisible}
                  activeSubtitleSegmentIndex={activeSubtitleSegmentIndex}
                  onSubtitlesVisibleChange={setSubtitlesVisible}
                  onSubtitleSegmentChange={updateSubtitleSegmentRange}
                  onSubtitleSegmentCommit={() => void persistCurrentSubtitleSegments()}
                  onSubtitleSegmentSelect={setActiveSubtitleSegmentIndex}
                  onSubtitleSegmentSplit={splitSubtitleSegment}
                  editingSubtitleSegmentIndex={subtitleEditingIndex}
                  editingSubtitleText={subtitleEditingText}
                  onSubtitleEditStart={startInlineSubtitleEdit}
                  onSubtitleEditChange={setSubtitleEditingText}
                  onSubtitleEditCommit={commitInlineSubtitleEdit}
                  onSubtitleEditCancel={cancelInlineSubtitleEdit}
                  extraControls={
                    <>
                      <Button size="small" loading={previewingFrames} onClick={() => void previewFrames()}>
                        三帧
                      </Button>
                      <Button
                        size="small"
                        disabled={!interpretFPSAvailable}
                        type={watchedInterpretFPS ? "primary" : "default"}
                        onClick={openInterpretFPSModal}
                      >
                        升格
                      </Button>
                      <Button
                        size="small"
                        loading={transcribingItemID === selectedItem.id}
                        disabled={watchedSourceType !== "talking_head" || !selectedItem.probe.has_audio}
                        onClick={startTranscription}
                      >
                        转写
                      </Button>
                      <Button size="small" onClick={openNotesModal}>
                        备注
                      </Button>
                      <Space size={4}>
                        <Switch
                          size="small"
                          checked={useProductReferenceImage}
                          disabled={watchedSourceType !== "visual_only" || !selectedProductReferenceImage}
                          onChange={setUseProductReferenceImage}
                        />
                        <Typography.Text type="secondary">参考图</Typography.Text>
                      </Space>
                      <Button size="small" loading={startingVLMLabel} onClick={() => void startVLMLabel()}>
                        VLM标注
                      </Button>
                    </>
                  }
                  analysisOverlay={
                    <div className="preprocess-analysis-overlay">
                      <Typography.Text className="preprocess-overlay-title">本地分析结果</Typography.Text>
                      <Descriptions size="small" column={1}>
                        <Descriptions.Item label="状态">{workspaceStatusLabels[selectedItem.status]}</Descriptions.Item>
                        <Descriptions.Item label="时长">{formatProbeDuration(selectedItem.probe.duration_ms)}</Descriptions.Item>
                        <Descriptions.Item label="分辨率">{formatResolution(selectedItem.probe.width, selectedItem.probe.height)}</Descriptions.Item>
                        <Descriptions.Item label="帧率">{formatProbeFPS(selectedItem.probe.fps)}</Descriptions.Item>
                        {watchedInterpretFPS ? (
                          <Descriptions.Item label="升格">
                            {formatProbeFPS(sourceFPS)} → {formatProbeFPS(watchedPlaybackFPS)} / 慢放{" "}
                            {(sourceFPS / watchedPlaybackFPS).toFixed(2)}x
                          </Descriptions.Item>
                        ) : null}
                        <Descriptions.Item label="VLM状态">{selectedItem.vlm_status || "idle"}</Descriptions.Item>
                        <Descriptions.Item label="画面描述">{selectedItem.analysis?.scene_description || "-"}</Descriptions.Item>
                        <Descriptions.Item label="景别">
                          {selectedItem.analysis?.shot_size
                            ? shotSizeLabels[selectedItem.analysis.shot_size] ?? selectedItem.analysis.shot_size
                            : "-"}
                        </Descriptions.Item>
                        <Descriptions.Item label="运镜">
                          {selectedItem.analysis?.camera_movement
                            ? cameraMovementLabels[selectedItem.analysis.camera_movement] ??
                              selectedItem.analysis.camera_movement
                            : "-"}
                        </Descriptions.Item>
                        <Descriptions.Item label="标签">{selectedItem.analysis?.visual_tags?.join("、") || "-"}</Descriptions.Item>
                        <Descriptions.Item label="质量">{selectedItem.analysis?.quality_tags?.join("、") || "-"}</Descriptions.Item>
                        <Descriptions.Item label="产品">{selectedItem.analysis?.visible_product ? "可见" : "-"}</Descriptions.Item>
                        <Descriptions.Item label="位置">{selectedItem.analysis?.product_position || "-"}</Descriptions.Item>
                        <Descriptions.Item label="场景">{selectedItem.analysis?.scene_context || "-"}</Descriptions.Item>
                        <Descriptions.Item label="动作">{selectedItem.analysis?.action_description || "-"}</Descriptions.Item>
                        <Descriptions.Item label="人物">{selectedItem.analysis?.people_presence ? "有人" : "无人"}</Descriptions.Item>
                        <Descriptions.Item label="露脸">{selectedItem.analysis?.face_visible ? "是" : "否"}</Descriptions.Item>
                        <Descriptions.Item label="光线">{selectedItem.analysis?.lighting_condition || "-"}</Descriptions.Item>
                        {selectedItem.vlm_error ? <Descriptions.Item label="VLM错误">{selectedItem.vlm_error}</Descriptions.Item> : null}
                      </Descriptions>
                    </div>
                  }
                />
              </Card>
            </div>
            </Form>
          </div>
        ) : null}
      </Modal>

      <Modal
        open={framesPreviewOpen}
        onCancel={() => setFramesPreviewOpen(false)}
        footer={null}
        width={920}
        title="当前区间三帧抽样"
        destroyOnClose={false}
      >
        {selectedItem?.preview_frame_snapshots.length ? (
          <Image.PreviewGroup>
            <div className="frame-grid">
              {selectedItem.preview_frame_snapshots.map((frame) => (
                <div key={frame.frame_index} className="frame-card">
                  <Image className="frame-image" src={frame.image_url} alt={`frame-${frame.frame_index}`} />
                  <Typography.Text type="secondary">
                    第 {frame.frame_index + 1} 帧 | {formatTimestamp(frame.timestamp_ms)}
                  </Typography.Text>
                </div>
              ))}
            </div>
          </Image.PreviewGroup>
        ) : (
          <Empty description="当前还没有三帧抽样结果" />
        )}
      </Modal>

      <Modal
        open={interpretFPSModalOpen}
        onCancel={() => setInterpretFPSModalOpen(false)}
        onOk={() => void applyInterpretFPSSettings()}
        confirmLoading={applyingInterpretFPS}
        width={620}
        title="升格 / 解释帧率"
        okText="应用"
        cancelText="取消"
        destroyOnClose={false}
        className="preprocess-text-modal preprocess-interpret-fps-modal"
      >
        <Space direction="vertical" size="middle" className="preprocess-interpret-panel">
          <Alert
            type="info"
            showIcon
            message="升格只改变播放帧率，不做补帧、不做光流、不做快动作。"
            description="适合 50fps、100fps 等高帧率纯画面素材。口播素材不开启，避免音画不同步。"
          />
          <Descriptions size="small" column={1}>
            <Descriptions.Item label="素材类型">
              {sourceTypeLabels[watchedSourceType || selectedItem?.source_type || defaultSourceType] ?? "-"}
            </Descriptions.Item>
            <Descriptions.Item label="源帧率">{formatProbeFPS(sourceFPS)}</Descriptions.Item>
            <Descriptions.Item label="当前工作源帧率">{formatProbeFPS(workingFPS)}</Descriptions.Item>
            <Descriptions.Item label="当前选区">{formatDuration(selectedDurationMs)}</Descriptions.Item>
          </Descriptions>
          <div className="preprocess-interpret-row">
            <Typography.Text>启用升格</Typography.Text>
            <Switch
              checked={watchedInterpretFPS}
              disabled={!interpretFPSAvailable}
              onChange={(checked) => updateInterpretFPS(checked, checked ? Math.min(watchedPlaybackFPS || 25, sourceFPS - 1) : 25)}
            />
          </div>
          <div className="preprocess-interpret-row">
            <Typography.Text>播放帧率</Typography.Text>
            <InputNumber
              min={25}
              max={Math.max(25, Math.floor(sourceFPS - 1))}
              step={1}
              precision={0}
              disabled={!watchedInterpretFPS}
              value={watchedPlaybackFPS}
              addonAfter="fps"
              onChange={(value) => updateInterpretFPS(watchedInterpretFPS, Number(value ?? 25))}
            />
          </div>
          <div className="preprocess-interpret-result">
            <Typography.Text>
              慢放倍率：{watchedInterpretFPS ? (sourceFPS / watchedPlaybackFPS).toFixed(2) : "1.00"}x
            </Typography.Text>
            <Typography.Text>
              预计时长：{formatDuration(selectedDurationMs)} → {formatDuration(interpretedDurationMs)}
            </Typography.Text>
          </div>
        </Space>
      </Modal>

      <Modal
        open={notesModalOpen}
        onCancel={() => setNotesModalOpen(false)}
        onOk={saveNotesDraft}
        width={720}
        title="本地预处理备注"
        okText="保存"
        cancelText="取消"
        destroyOnClose={false}
        className="preprocess-text-modal"
      >
        <Input.TextArea
          value={notesDraft}
          onChange={(event) => setNotesDraft(event.target.value)}
          rows={8}
          placeholder="记录本地预处理备注"
        />
      </Modal>

    </Space>
  );
}
