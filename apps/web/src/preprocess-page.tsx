import React, { useEffect, useMemo, useState } from "react";
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
  Table,
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

export function PreprocessPage({ token }: { token: string }) {
  const [items, setItems] = useState<WorkspaceItem[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [sellingPoints, setSellingPoints] = useState<SellingPoint[]>([]);
  const [loading, setLoading] = useState(false);
  const [importing, setImporting] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [duplicating, setDuplicating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [selectedItemID, setSelectedItemID] = useState<string | null>(null);
  const [submitProductID, setSubmitProductID] = useState<string>("");
  const [submitSellingPointIDs, setSubmitSellingPointIDs] = useState<string[]>([]);
  const [framesPreviewOpen, setFramesPreviewOpen] = useState(false);
  const [cleanShotPreviewOpen, setCleanShotPreviewOpen] = useState(false);
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

  const importFiles = async () => {
    if (selectedFiles.length === 0) {
      message.warning("请先选择原始视频文件");
      return;
    }

    const body = new FormData();
    selectedFiles.forEach((file) => body.append("files", file));

    setImporting(true);
    try {
      await localAgentRequest("/workspace/import", {
        method: "POST",
        body
      });
      setSelectedFiles([]);
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

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>预处理工作区</Typography.Title>

      <Alert
        type="info"
        showIcon
        message="这里是本地预处理工作区，不是服务端素材库"
        description="原始视频、处理中间态、仅保存未提交的结果都只保留在本地 Agent 工作区。只有正式提交后，服务端才会创建素材资产。"
      />

      <Card
        title="导入原始视频"
        extra={
          <Space>
            <Button onClick={() => void loadItems()}>刷新列表</Button>
            <Button danger loading={clearing} onClick={() => void clearWorkspace()}>
              清空工作区
            </Button>
          </Space>
        }
      >
        <Space direction="vertical" className="wide-space" size="middle">
          <input
            type="file"
            accept="video/*"
            multiple
            onChange={(event) => setSelectedFiles(Array.from(event.target.files ?? []))}
          />
          <Space wrap>
            <Button type="primary" loading={importing} onClick={() => void importFiles()}>
              导入到工作区
            </Button>
            <Typography.Text type="secondary">已选择 {selectedFiles.length} 个文件</Typography.Text>
          </Space>
        </Space>
      </Card>

      <Card title="待处理视频列表">
        <Table<WorkspaceItem>
          rowKey="id"
          loading={loading}
          dataSource={items}
          locale={{ emptyText: <Empty description="本地工作区还没有视频" /> }}
          pagination={false}
          columns={[
            {
              title: "视频",
              render: (_, item) => (
                <Button type="link" className="table-link-button" onClick={() => setSelectedItemID(item.id)}>
                  {item.asset_name || item.original_file_name}
                </Button>
              )
            },
            {
              title: "状态",
              dataIndex: "status",
              render: (value: WorkspaceItem["status"]) => <Tag>{workspaceStatusLabels[value]}</Tag>
            },
            {
              title: "类型",
              dataIndex: "source_type",
              render: (value?: string) => (value ? sourceTypeLabels[value] ?? value : "-")
            },
            {
              title: "时长",
              render: (_, item) => formatDuration(item.probe.duration_ms)
            },
            {
              title: "提交结果",
              render: (_, item) => (item.submitted_asset_id ? item.submitted_asset_id : "-")
            },
            {
              title: "更新时间",
              dataIndex: "updated_at",
              render: (value?: string) => formatDateTime(value)
            },
            {
              title: "错误",
              render: (_, item) =>
                item.last_error ? <Typography.Text type="danger">{item.last_error}</Typography.Text> : "-"
            }
          ]}
        />
      </Card>

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
                  onTrimChange={(range) => updateTrimRange(range.inMs, range.outMs)}
                  analysisOverlay={
                    <div className="preprocess-analysis-overlay">
                      <Typography.Text className="preprocess-overlay-title">本地分析结果</Typography.Text>
                      <Descriptions size="small" column={1}>
                        <Descriptions.Item label="状态">{workspaceStatusLabels[selectedItem.status]}</Descriptions.Item>
                        <Descriptions.Item label="时长">{formatProbeDuration(selectedItem.probe.duration_ms)}</Descriptions.Item>
                        <Descriptions.Item label="分辨率">{formatResolution(selectedItem.probe.width, selectedItem.probe.height)}</Descriptions.Item>
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

            <div className="preprocess-footer-strip">
              {watchedSourceType === "talking_head" ? (
                <Form.Item
                  name="transcript"
                  label="口播转写"
                  className="preprocess-footer-field preprocess-transcript-field"
                  rules={[{ required: true, message: "口播素材必须填写转写内容" }]}
                >
                  <Input.TextArea rows={2} placeholder="[00:00:03:00]-[00:00:05:00] 大家好。" />
                </Form.Item>
              ) : (
                <div className="preprocess-footer-note">
                  <Typography.Text type="secondary">当前为纯画面素材，无需填写口播转写。</Typography.Text>
                </div>
              )}

              <Form.Item name="reviewer_notes" label="备注" className="preprocess-footer-field">
                <Input.TextArea className="preprocess-reviewer-notes-input" rows={2} placeholder="记录本地预处理备注" />
              </Form.Item>

              <div className="preprocess-footer-actions">
                <Button onClick={() => setFramesPreviewOpen(true)} disabled={selectedItem.frame_snapshots.length === 0}>
                  查看三帧抽样
                </Button>
                {selectedItem.clean_shot_url ? (
                  <Button onClick={() => setCleanShotPreviewOpen(true)}>查看 clean shot</Button>
                ) : (
                  <Typography.Text type="secondary">完成处理后会生成 clean shot 与三帧抽样。</Typography.Text>
                )}
              </div>
            </div>
          </Form>
        ) : null}
      </Modal>

      <Modal
        open={framesPreviewOpen}
        onCancel={() => setFramesPreviewOpen(false)}
        footer={null}
        width={920}
        title="三帧抽样"
        destroyOnClose={false}
      >
        {selectedItem?.frame_snapshots.length ? (
          <Image.PreviewGroup>
            <div className="frame-grid">
              {selectedItem.frame_snapshots.map((frame) => (
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
        open={cleanShotPreviewOpen}
        onCancel={() => setCleanShotPreviewOpen(false)}
        footer={null}
        width={920}
        title="clean shot 预览"
        destroyOnClose={false}
      >
        {selectedItem?.clean_shot_url ? (
          <video className="preprocess-video preprocess-clean-shot-modal-video" controls src={selectedItem.clean_shot_url} />
        ) : (
          <Empty description="当前还没有 clean shot 预览" />
        )}
      </Modal>
    </Space>
  );
}
