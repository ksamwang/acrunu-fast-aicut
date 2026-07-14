import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  Form,
  Image,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
  message
} from "antd";
import { VideoTrimEditor } from "./VideoTrimEditor";
import { localAgentRequest } from "../../shared/api/local-agent-api";
import { authenticatedApiRequest } from "../../shared/api/server-api";
import { formatDateTime, formatDuration, formatTimestamp } from "../../shared/lib/format";
import type { Product, SellingPoint } from "../../shared/types/product";
import type {
  UploadToken,
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
  objectUrl: string;
  thumbnailUrl?: string;
  durationMs?: number;
};

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
  return item.frame_snapshots[0]?.image_url;
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

function createImportThumbnail(objectUrl: string): Promise<{ thumbnailUrl?: string; durationMs?: number }> {
  return new Promise((resolve) => {
    const video = document.createElement("video");
    video.preload = "metadata";
    video.muted = true;
    video.playsInline = true;
    video.src = objectUrl;

    const finish = (result: { thumbnailUrl?: string; durationMs?: number }) => {
      video.removeAttribute("src");
      video.load();
      resolve(result);
    };

    video.onerror = () => finish({});
    video.onloadedmetadata = () => {
      const durationMs = Number.isFinite(video.duration) ? Math.round(video.duration * 1000) : undefined;
      const seekTime = Math.min(Math.max(video.duration * 0.08, 0.2), 1);
      if (!Number.isFinite(video.duration) || video.duration <= 0) {
        finish({ durationMs });
        return;
      }
      video.currentTime = seekTime;
      video.onseeked = () => {
        const canvas = document.createElement("canvas");
        canvas.width = video.videoWidth || 320;
        canvas.height = video.videoHeight || 180;
        const context = canvas.getContext("2d");
        if (!context) {
          finish({ durationMs });
          return;
        }
        context.drawImage(video, 0, 0, canvas.width, canvas.height);
        finish({ durationMs, thumbnailUrl: canvas.toDataURL("image/jpeg", 0.76) });
      };
    };
  });
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
  const [selectedItemID, setSelectedItemID] = useState<string | null>(null);
  const lastSubmitProductIDRef = useRef(loadLastSubmitProductID());
  const [submitProductID, setSubmitProductID] = useState<string>(lastSubmitProductIDRef.current);
  const [submitSellingPointIDs, setSubmitSellingPointIDs] = useState<string[]>([]);
  const [useProductReferenceImage, setUseProductReferenceImage] = useState(true);
  const [framesPreviewOpen, setFramesPreviewOpen] = useState(false);
  const [transcriptModalOpen, setTranscriptModalOpen] = useState(false);
  const [notesModalOpen, setNotesModalOpen] = useState(false);
  const [interpretFPSModalOpen, setInterpretFPSModalOpen] = useState(false);
  const [transcriptDraft, setTranscriptDraft] = useState("");
  const [transcriptSegmentsDraft, setTranscriptSegmentsDraft] = useState<WorkspaceTranscriptSegment[]>([]);
  const [subtitlesVisible, setSubtitlesVisible] = useState(true);
  const [activeSubtitleSegmentIndex, setActiveSubtitleSegmentIndex] = useState<number | null>(null);
  const [notesDraft, setNotesDraft] = useState("");
  const importPreviewsRef = useRef<ImportPreview[]>([]);
  const initializedEditorItemIDRef = useRef<string | null>(null);
  const preprocessWorkbenchRef = useRef<HTMLDivElement | null>(null);
  const [form] = Form.useForm();
  const watchedSourceType = Form.useWatch("source_type", form);
  const watchedSourceInMs = Form.useWatch("source_in_ms", form) ?? 0;
  const watchedSourceOutMs = Form.useWatch("source_out_ms", form) ?? 0;
  const watchedInterpretFPS = Boolean(Form.useWatch("interpret_fps_enabled", form));
  const watchedPlaybackFPS = Number(Form.useWatch("playback_fps", form) ?? 25);
  const watchedTranscriptSegments = (Form.useWatch("transcript_segments", form) ?? []) as WorkspaceTranscriptSegment[];
  const selectedSubmitProduct = useMemo(
    () => products.find((product) => product.id === submitProductID) ?? null,
    [products, submitProductID]
  );
  const selectedProductReferenceImage = productReferenceImage(selectedSubmitProduct);

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

  const loadItems = async () => {
    setLoading(true);
    try {
      const response = await localAgentRequest<WorkspaceListResponse>("/workspace/items");
      setItems(response.items ?? []);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "加载预处理工作区失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadItems();
  }, []);

  useEffect(() => {
    importPreviewsRef.current = importPreviews;
  }, [importPreviews]);

  useEffect(() => {
    return () => {
      importPreviewsRef.current.forEach((preview) => URL.revokeObjectURL(preview.objectUrl));
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
  const activeASRDraft =
    selectedItem?.asr_draft &&
    selectedItem.asr_draft.source_in_ms === Number(watchedSourceInMs) &&
    selectedItem.asr_draft.source_out_ms === Number(watchedSourceOutMs)
      ? selectedItem.asr_draft
      : undefined;
  const workspaceStats = useMemo(
    () => ({
      pending: items.filter((item) => item.status === "pending").length,
      saved: items.filter((item) => item.status === "saved").length,
      ready: items.filter((item) => item.status === "ready_to_submit").length,
      submitted: items.filter((item) => item.status === "submitted").length
    }),
    [items]
  );
  const hasRunningVLMLabel = items.some((item) => item.vlm_status === "queued" || item.vlm_status === "running");

  useEffect(() => {
    if (!hasRunningVLMLabel) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadItems();
    }, 2000);
    return () => window.clearInterval(timer);
  }, [hasRunningVLMLabel]);

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
    importPreviewsRef.current.forEach((preview) => URL.revokeObjectURL(preview.objectUrl));
    setImportPreviews([]);
  };

  const closeImportModal = () => {
    if (importing) {
      return;
    }
    clearImportPreviews();
    setImportModalOpen(false);
  };

  const selectImportFiles = (files: File[]) => {
    clearImportPreviews();
    const nextPreviews = files.map((file, index) => ({
      id: `${file.name}-${file.size}-${file.lastModified}-${index}`,
      file,
      objectUrl: URL.createObjectURL(file)
    }));
    setImportPreviews(nextPreviews);

    nextPreviews.forEach((preview) => {
      void createImportThumbnail(preview.objectUrl).then((result) => {
        setImportPreviews((current) =>
          current.map((item) => (item.id === preview.id ? { ...item, ...result } : item))
        );
      });
    });
  };

  const removeImportPreview = (id: string) => {
    setImportPreviews((current) => {
      const target = current.find((item) => item.id === id);
      if (target) {
        URL.revokeObjectURL(target.objectUrl);
      }
      return current.filter((item) => item.id !== id);
    });
  };

  const importFiles = async () => {
    if (importPreviews.length === 0) {
      message.warning("请先选择原始视频文件");
      return;
    }

    const body = new FormData();
    importPreviews.forEach((preview) => body.append("files", preview.file));

    setImporting(true);
    try {
      await localAgentRequest("/workspace/import", {
        method: "POST",
        body
      });
      clearImportPreviews();
      setImportModalOpen(false);
      await loadItems();
      message.success("原始视频已导入预处理工作区");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "导入原始视频失败");
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
      setItems((current) => current.map((item) => (item.id === response.item.id ? response.item : item)));
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
    setItems((current) => current.map((currentItem) => (currentItem.id === response.item.id ? response.item : currentItem)));
    return response.item;
  };

  const startTranscription = async () => {
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
      setItems((current) => current.map((item) => (item.id === response.item.id ? response.item : item)));
      message.success("当前选区转写完成，请核对草稿后应用到编辑区");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "当前选区转写失败");
    } finally {
      setTranscribingItemID((current) => (current === itemID ? null : current));
    }
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
      setItems((current) => current.map((item) => (item.id === prepared.item.id ? prepared.item : item)));
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
      setItems((current) => current.map((item) => (item.id === response.item.id ? response.item : item)));
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
          source_type: sourceType,
          product_name: productName,
          product_reference_image_data_url: productReferenceImageDataURL,
          source_in_ms: Math.round(sourceInMs),
          source_out_ms: Math.round(sourceOutMs),
          server_base_url: window.location.origin,
          auth_token: token
        })
      });
      setItems((current) => current.map((item) => (item.id === response.item.id ? response.item : item)));
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
      await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(values)
      });
      const response = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}/duplicate`, {
        method: "POST"
      });
      setItems((current) => [...current, response.item]);
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
      await loadItems();
      setSelectedItemID(null);
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
          selling_point_ids: submitSellingPointIDs
        })
      });

      setItems((current) => current.map((item) => (item.id === response.item.id ? response.item : item)));
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
      setTranscriptDraft("");
      setTranscriptSegmentsDraft([]);
      setActiveSubtitleSegmentIndex(null);
      message.warning("调整 I/O 后已清除当前字幕句段，请重新识别并确认");
    }
  };

  const openTranscriptModal = () => {
    setTranscriptDraft(form.getFieldValue("transcript") ?? "");
    setTranscriptSegmentsDraft((form.getFieldValue("transcript_segments") ?? []).map((segment: WorkspaceTranscriptSegment) => ({ ...segment })));
    setTranscriptModalOpen(true);
  };

  const saveTranscriptDraft = () => {
    const segments = transcriptSegmentsDraft.filter((segment) => segment.text.trim() !== "");
    form.setFieldsValue({
      transcript: segments.length > 0 ? transcriptTextFromSegments(segments) : transcriptDraft,
      transcript_segments: segments
    });
    setTranscriptModalOpen(false);
  };

  const applyASRDraft = () => {
    if (!activeASRDraft?.segments.length) {
      return;
    }
    const segments = activeASRDraft.segments.map((segment) => ({ ...segment }));
    setTranscriptSegmentsDraft(segments);
    setTranscriptDraft(transcriptTextFromSegments(segments));
    setActiveSubtitleSegmentIndex(0);
  };

  const updateTranscriptSegmentText = (index: number, text: string) => {
    setTranscriptSegmentsDraft((current) => {
      const next = current.map((segment, segmentIndex) => (segmentIndex === index ? { ...segment, text } : segment));
      setTranscriptDraft(transcriptTextFromSegments(next));
      return next;
    });
  };

  const updateSubtitleSegmentRange = (index: number, startMs: number, endMs: number) => {
    const next = watchedTranscriptSegments.map((segment, segmentIndex) =>
      segmentIndex === index ? { ...segment, start_ms: startMs, end_ms: endMs } : segment
    );
    form.setFieldsValue({
      transcript: transcriptTextFromSegments(next),
      transcript_segments: next
    });
    setTranscriptSegmentsDraft((current) =>
      current.map((segment, segmentIndex) =>
        segmentIndex === index ? { ...segment, start_ms: startMs, end_ms: endMs } : segment
      )
    );
    setActiveSubtitleSegmentIndex(index);
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
      setItems((current) => current.map((item) => (item.id === response.item.id ? response.item : item)));
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

  return (
    <Space direction="vertical" size="middle" className="page-stack preprocess-page-stack">
      <div className="preprocess-workspace-toolbar">
        <Typography.Text className="preprocess-workspace-status">
          待处理 {workspaceStats.pending} / 已保存 {workspaceStats.saved} / 待提交 {workspaceStats.ready} / 已入库 {workspaceStats.submitted}
        </Typography.Text>
        <Space wrap>
          <Button icon={<RefreshIcon />} onClick={() => void loadItems()} loading={loading}>
            刷新
          </Button>
          <Button danger icon={<TrashIcon />} loading={clearing} onClick={() => void clearWorkspace()}>
            清空工作区
          </Button>
        </Space>
      </div>

      <section className="preprocess-workspace-board">
        {items.length === 0 ? (
          <div className="preprocess-workspace-empty">
            <Empty description={loading ? "正在加载本地工作区" : "还没有导入视频"} />
            <Typography.Text type="secondary">点击右下角按钮导入原始视频，处理完成前不会进入服务端素材库。</Typography.Text>
          </div>
        ) : (
          <div className="preprocess-asset-grid">
            {items.map((item) => {
              const previewUrl = getWorkspacePreviewUrl(item);
              return (
                <button
                  key={item.id}
                  type="button"
                  className="preprocess-asset-card"
                  onClick={() => setSelectedItemID(item.id)}
                  title={item.asset_name || item.original_file_name}
                >
                  <div className="preprocess-asset-preview">
                    {previewUrl ? (
                      <img src={previewUrl} alt={item.asset_name || item.original_file_name} />
                    ) : (
                      <video src={item.source_url} muted preload="metadata" />
                    )}
                    <Tag color={workspaceStatusColors[item.status]} className="preprocess-asset-status">
                      {workspaceStatusLabels[item.status]}
                    </Tag>
                    <Tag color={sourceTypeColors[item.source_type || defaultSourceType] ?? "default"} className="preprocess-asset-type">
                      {sourceTypeLabels[item.source_type || defaultSourceType] ?? "-"}
                    </Tag>
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
              );
            })}
          </div>
        )}

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
              onChange={(event) => selectImportFiles(Array.from(event.target.files ?? []))}
            />
            <UploadIcon />
            <span>选择视频文件</span>
            <Typography.Text type="secondary">支持一次选择多个原始视频，确认后导入本地预处理工作区。</Typography.Text>
          </label>

          {importPreviews.length > 0 ? (
            <div className="preprocess-import-preview-grid">
              {importPreviews.map((preview) => (
                <div key={preview.id} className="preprocess-import-preview-card">
                  <div className="preprocess-import-thumbnail">
                    {preview.thumbnailUrl ? (
                      <img src={preview.thumbnailUrl} alt={preview.file.name} />
                    ) : (
                      <video src={preview.objectUrl} muted preload="metadata" />
                    )}
                  </div>
                  <div className="preprocess-import-info">
                    <Typography.Text className="preprocess-import-name">{preview.file.name}</Typography.Text>
                    <Typography.Text type="secondary">
                      {formatFileSize(preview.file.size)} · {formatDuration(preview.durationMs)}
                    </Typography.Text>
                  </div>
                  <Button size="small" icon={<CloseIcon />} onClick={() => removeImportPreview(preview.id)}>
                    移除
                  </Button>
                </div>
              ))}
            </div>
          ) : (
            <div className="preprocess-import-empty">选择文件后将在这里显示缩略预览。</div>
          )}

          <div className="preprocess-import-actions">
            <Button onClick={closeImportModal} disabled={importing}>
              取消
            </Button>
            <Button type="primary" icon={<UploadIcon />} loading={importing} disabled={importPreviews.length === 0} onClick={() => void importFiles()}>
              确认导入工作区
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
                  onSubtitleSegmentSelect={setActiveSubtitleSegmentIndex}
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
                      <Button size="small" disabled={watchedSourceType !== "talking_head"} onClick={openTranscriptModal}>
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
        open={transcriptModalOpen}
        onCancel={() => setTranscriptModalOpen(false)}
        onOk={saveTranscriptDraft}
        width={720}
        title="口播转写"
        okText="保存"
        cancelText="取消"
        destroyOnClose={false}
        className="preprocess-text-modal preprocess-transcript-modal"
      >
        <div className="preprocess-transcript-modal-content">
          <div className="preprocess-transcript-actions">
            <Typography.Text type="secondary">识别范围：当前 I/O 选区</Typography.Text>
            <Button
              type="primary"
              loading={transcribingItemID === selectedItem?.id}
              disabled={!selectedItem || watchedSourceType !== "talking_head" || !selectedItem.probe.has_audio}
              onClick={() => void startTranscription()}
            >
              识别当前选区
            </Button>
          </div>
          {transcriptSegmentsDraft.length > 0 ? (
            <div className="preprocess-confirmed-transcript">
              <div className="preprocess-confirmed-transcript-header">
                <Typography.Text strong>已确认句段</Typography.Text>
                <Typography.Text type="secondary">逐句核对文字，时间范围来自当前选区</Typography.Text>
              </div>
              <div className="preprocess-confirmed-segment-list">
                {transcriptSegmentsDraft.map((segment, index) => (
                  <div
                    key={`${segment.start_ms}-${segment.end_ms}-${index}`}
                    className={`preprocess-confirmed-segment${activeSubtitleSegmentIndex === index ? " is-selected" : ""}`}
                  >
                    <Typography.Text className="preprocess-asr-segment-time">
                      +{formatTimestamp(segment.start_ms)} - +{formatTimestamp(segment.end_ms)}
                    </Typography.Text>
                    <Input.TextArea
                      value={segment.text}
                      autoSize={{ minRows: 1, maxRows: 4 }}
                      onChange={(event) => updateTranscriptSegmentText(index, event.target.value)}
                    />
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <Input.TextArea
              value={transcriptDraft}
              onChange={(event) => setTranscriptDraft(event.target.value)}
              rows={8}
              placeholder="请输入口播转写"
            />
          )}
          <div className="preprocess-asr-draft" aria-live="polite">
            <div className="preprocess-asr-draft-header">
              <div>
                <Typography.Text strong>识别草稿</Typography.Text>
                {activeASRDraft ? (
                  <Typography.Text type="secondary">
                    当前选区 · {formatTimestamp(activeASRDraft.source_in_ms)} - {formatTimestamp(activeASRDraft.source_out_ms)}
                  </Typography.Text>
                ) : null}
              </div>
              <Button size="small" disabled={!activeASRDraft?.segments.length} onClick={applyASRDraft}>
                应用草稿
              </Button>
            </div>
            {activeASRDraft?.segments.length ? (
              <div className="preprocess-asr-segment-list">
                {activeASRDraft.segments.map((segment, index) => (
                  <div key={`${segment.start_ms}-${segment.end_ms}-${index}`} className="preprocess-asr-segment">
                    <Typography.Text className="preprocess-asr-segment-time">
                      +{formatTimestamp(segment.start_ms)} - +{formatTimestamp(segment.end_ms)}
                    </Typography.Text>
                    <Typography.Text>{segment.text}</Typography.Text>
                  </div>
                ))}
              </div>
            ) : activeASRDraft?.text ? (
              <Typography.Paragraph className="preprocess-asr-draft-text">{activeASRDraft.text}</Typography.Paragraph>
            ) : (
              <Typography.Text type="secondary">尚未识别当前选区。</Typography.Text>
            )}
          </div>
        </div>
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
