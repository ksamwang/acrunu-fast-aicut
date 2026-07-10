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
  Modal,
  Select,
  Space,
  Tag,
  Typography,
  message
} from "antd";
import { VideoTrimEditor } from "./video-trim-editor";

const LOCAL_AGENT_BASE_URL = "http://127.0.0.1:58721";

type Product = {
  id: string;
  name: string;
};

type SellingPoint = {
  id: string;
  title: string;
};

type UploadToken = {
  token: string;
  product_id: string;
};

type WorkspaceProbe = {
  duration_ms?: number;
  width?: number;
  height?: number;
  fps?: number;
  codec?: string;
  has_audio?: boolean;
  audio_codec?: string;
  bitrate_kbps?: number;
};

type WorkspaceFrameSnapshot = {
  frame_index: number;
  timestamp_ms: number;
  image_url: string;
};

type WorkspaceAnalysis = {
  scene_description?: string;
  shot_size?: string;
  camera_movement?: string;
  subjects?: string[];
  scene_tags?: string[];
  quality_tags?: string[];
  usability_status?: string;
};

type WorkspaceItem = {
  id: string;
  status: "pending" | "saved" | "ready_to_submit" | "submitted";
  product_id?: string;
  submitted_asset_id?: string;
  asset_name?: string;
  source_type?: "visual_only" | "talking_head";
  original_file_name: string;
  source_in_ms: number;
  source_out_ms: number;
  transcript?: string;
  reviewer_notes?: string;
  probe: WorkspaceProbe;
  preview_in_ms?: number;
  preview_out_ms?: number;
  preview_frame_snapshots: WorkspaceFrameSnapshot[];
  analysis?: WorkspaceAnalysis;
  frame_snapshots: WorkspaceFrameSnapshot[];
  source_url: string;
  clean_shot_url?: string;
  checksum?: string;
  last_error?: string;
  updated_at: string;
};

type WorkspaceListResponse = {
  items: WorkspaceItem[];
};

type WorkspaceItemResponse = {
  item: WorkspaceItem;
};

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

const sourceTypeLabels: Record<string, string> = {
  visual_only: "纯画面",
  talking_head: "口播"
};

const shotSizeLabels: Record<string, string> = {
  close_up: "特写",
  medium_close_up: "近景",
  medium_shot: "中景",
  wide_shot: "远景"
};

const cameraMovementLabels: Record<string, string> = {
  static: "固定机位",
  slow_push_in: "缓慢推进",
  pan: "平移",
  handheld: "手持"
};

async function localAgentRequest<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`${LOCAL_AGENT_BASE_URL}${path}`, options);
  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload?.error ?? "本地 Agent 请求失败");
  }
  return payload as T;
}

async function apiRequest<T>(path: string, token: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...options,
    headers: {
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      Authorization: `Bearer ${token}`,
      ...options.headers
    }
  });
  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload?.error?.message ?? "服务端请求失败");
  }
  return payload.data as T;
}

function formatDuration(durationMs?: number) {
  if (durationMs === undefined || durationMs === null) {
    return "-";
  }
  const totalSeconds = Math.floor(durationMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

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

function formatTimestamp(durationMs: number) {
  const totalSeconds = Math.floor(durationMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  const milliseconds = durationMs % 1000;
  return `${minutes}:${seconds.toString().padStart(2, "0")}.${milliseconds.toString().padStart(3, "0")}`;
}

function formatDateTime(value?: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatFileSize(size: number) {
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}

function getWorkspacePreviewUrl(item: WorkspaceItem) {
  return item.frame_snapshots[0]?.image_url;
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

export function PreprocessPage({ token }: { token: string }) {
  const [items, setItems] = useState<WorkspaceItem[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [sellingPoints, setSellingPoints] = useState<SellingPoint[]>([]);
  const [loading, setLoading] = useState(false);
  const [importing, setImporting] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [previewingFrames, setPreviewingFrames] = useState(false);
  const [duplicating, setDuplicating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [importPreviews, setImportPreviews] = useState<ImportPreview[]>([]);
  const [selectedItemID, setSelectedItemID] = useState<string | null>(null);
  const [submitProductID, setSubmitProductID] = useState<string>("");
  const [submitSellingPointIDs, setSubmitSellingPointIDs] = useState<string[]>([]);
  const [framesPreviewOpen, setFramesPreviewOpen] = useState(false);
  const [transcriptModalOpen, setTranscriptModalOpen] = useState(false);
  const [notesModalOpen, setNotesModalOpen] = useState(false);
  const [transcriptDraft, setTranscriptDraft] = useState("");
  const [notesDraft, setNotesDraft] = useState("");
  const importPreviewsRef = useRef<ImportPreview[]>([]);
  const [form] = Form.useForm();
  const watchedSourceType = Form.useWatch("source_type", form);
  const watchedSourceInMs = Form.useWatch("source_in_ms", form) ?? 0;
  const watchedSourceOutMs = Form.useWatch("source_out_ms", form) ?? 0;

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
        const result = await apiRequest<Product[]>("/api/products", token);
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
  const workspaceStats = useMemo(
    () => ({
      pending: items.filter((item) => item.status === "pending").length,
      saved: items.filter((item) => item.status === "saved").length,
      ready: items.filter((item) => item.status === "ready_to_submit").length,
      submitted: items.filter((item) => item.status === "submitted").length
    }),
    [items]
  );

  useEffect(() => {
    if (!selectedItem) {
      return;
    }
    const sourceOutMs =
      selectedItem.source_out_ms > 0
        ? selectedItem.source_out_ms
        : selectedItem.probe.duration_ms ?? 0;
    form.setFieldsValue({
      asset_name: selectedItem.asset_name ?? "",
      source_type: selectedItem.source_type ?? "visual_only",
      source_in_ms: selectedItem.source_in_ms ?? 0,
      source_out_ms: sourceOutMs,
      transcript: selectedItem.transcript ?? "",
      reviewer_notes: selectedItem.reviewer_notes ?? ""
    });
    setSubmitProductID(selectedItem.product_id ?? "");
    setSubmitSellingPointIDs([]);
  }, [form, selectedItem]);

  useEffect(() => {
    if (!submitProductID) {
      setSellingPoints([]);
      setSubmitSellingPointIDs([]);
      return;
    }

    void (async () => {
      try {
        const result = await apiRequest<SellingPoint[]>(`/api/products/${submitProductID}/selling-points`, token);
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
    const sourceInMs = Number(values.source_in_ms ?? selectedItem.source_in_ms ?? 0);
    const sourceOutMs = Number(values.source_out_ms ?? selectedItem.source_out_ms ?? selectedItem.probe.duration_ms ?? 0);
    if (!Number.isFinite(sourceInMs) || !Number.isFinite(sourceOutMs) || sourceOutMs <= sourceInMs) {
      message.warning("请先设置有效的裁切入点和出点");
      return;
    }

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
      setSubmitProductID("");
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
      const uploadToken = await apiRequest<UploadToken>("/api/uploads/tokens", token, {
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
    form.setFieldsValue({
      source_in_ms: startMs,
      source_out_ms: endMs
    });
  };

  const openTranscriptModal = () => {
    setTranscriptDraft(form.getFieldValue("transcript") ?? "");
    setTranscriptModalOpen(true);
  };

  const saveTranscriptDraft = () => {
    form.setFieldValue("transcript", transcriptDraft);
    setTranscriptModalOpen(false);
  };

  const openNotesModal = () => {
    setNotesDraft(form.getFieldValue("reviewer_notes") ?? "");
    setNotesModalOpen(true);
  };

  const saveNotesDraft = () => {
    form.setFieldValue("reviewer_notes", notesDraft);
    setNotesModalOpen(false);
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
                    <Tag className="preprocess-asset-status">{workspaceStatusLabels[item.status]}</Tag>
                    <Tag className="preprocess-asset-type">{sourceTypeLabels[item.source_type || "visual_only"] ?? "-"}</Tag>
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
          <Form form={form} layout="vertical" className="preprocess-workbench">
            <Form.Item name="source_in_ms" hidden>
              <Input type="hidden" />
            </Form.Item>
            <Form.Item name="source_out_ms" hidden>
              <Input type="hidden" />
            </Form.Item>
            <Form.Item
              name="transcript"
              hidden
              rules={watchedSourceType === "talking_head" ? [{ required: true, message: "口播素材必须填写转写内容" }] : undefined}
            >
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
                <Tag>{sourceTypeLabels[selectedItem.source_type || "visual_only"] ?? "-"}</Tag>
                {selectedItem.submitted_asset_id ? <Tag color="success">已入库</Tag> : null}
              </div>

              <div className="preprocess-header-fields">
                <Form.Item name="asset_name" className="preprocess-header-field">
                  <Input className="preprocess-asset-name-input" placeholder="素材名称" />
                </Form.Item>

                <Form.Item
                  name="source_type"
                  className="preprocess-header-field"
                  rules={[{ required: true, message: "请选择素材类型" }]}
                >
                  <Select
                    popupClassName="preprocess-select-dropdown"
                    placeholder="素材类型"
                    options={[
                      { value: "visual_only", label: "纯画面" },
                      { value: "talking_head", label: "口播" }
                    ]}
                  />
                </Form.Item>

                <div className="preprocess-header-submit">
                  <Typography.Text type="secondary">正式提交</Typography.Text>
                  <Select
                    popupClassName="preprocess-select-dropdown"
                    placeholder="产品"
                    value={submitProductID || undefined}
                    onChange={(value) => setSubmitProductID(value)}
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
                    disabled={selectedItem.status !== "ready_to_submit"}
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
                  durationMs={selectedItem.probe.duration_ms ?? Math.max(watchedSourceOutMs, selectedItem.source_out_ms, 0)}
                  fps={selectedItem.probe.fps}
                  trimInMs={watchedSourceInMs}
                  trimOutMs={watchedSourceOutMs}
                  hotkeysEnabled={!!selectedItem && !framesPreviewOpen}
                  onTrimChange={(range) => updateTrimRange(range.inMs, range.outMs)}
                  extraControls={
                    <>
                      <Button size="small" loading={previewingFrames} onClick={() => void previewFrames()}>
                        三帧
                      </Button>
                      <Button size="small" disabled={watchedSourceType !== "talking_head"} onClick={openTranscriptModal}>
                        转写
                      </Button>
                      <Button size="small" onClick={openNotesModal}>
                        备注
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
                        <Descriptions.Item label="可用状态">{selectedItem.analysis?.usability_status || "-"}</Descriptions.Item>
                      </Descriptions>
                    </div>
                  }
                />
              </Card>
            </div>
          </Form>
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
        open={transcriptModalOpen}
        onCancel={() => setTranscriptModalOpen(false)}
        onOk={saveTranscriptDraft}
        width={720}
        title="口播转写"
        okText="保存"
        cancelText="取消"
        destroyOnClose={false}
        className="preprocess-text-modal"
      >
        <Input.TextArea
          value={transcriptDraft}
          onChange={(event) => setTranscriptDraft(event.target.value)}
          rows={8}
          placeholder="[00:00:03:00]-[00:00:05:00] 大家好。"
        />
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
