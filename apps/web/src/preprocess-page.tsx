import React, { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message
} from "antd";

const LOCAL_AGENT_BASE_URL = "http://127.0.0.1:58721";

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

function formatDuration(durationMs?: number) {
  if (!durationMs) {
    return "-";
  }
  const totalSeconds = Math.floor(durationMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
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

export function PreprocessPage() {
  const [items, setItems] = useState<WorkspaceItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [importing, setImporting] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [selectedItemID, setSelectedItemID] = useState<string | null>(null);
  const [form] = Form.useForm();

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

  const selectedIndex = useMemo(
    () => items.findIndex((item) => item.id === selectedItemID),
    [items, selectedItemID]
  );
  const selectedItem = selectedIndex >= 0 ? items[selectedIndex] : null;

  useEffect(() => {
    if (!selectedItem) {
      return;
    }
    form.setFieldsValue({
      asset_name: selectedItem.asset_name ?? "",
      source_type: selectedItem.source_type ?? "visual_only",
      source_in_ms: selectedItem.source_in_ms ?? 0,
      source_out_ms: selectedItem.source_out_ms ?? selectedItem.probe.duration_ms ?? 0,
      transcript: selectedItem.transcript ?? "",
      reviewer_notes: selectedItem.reviewer_notes ?? ""
    });
  }, [form, selectedItem]);

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
      const saved = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(values)
      });
      const prepared = await localAgentRequest<WorkspaceItemResponse>(`/workspace/items/${selectedItem.id}/prepare`, {
        method: "POST"
      });
      setItems((current) =>
        current.map((item) => {
          if (item.id === saved.item.id) {
            return prepared.item;
          }
          return item;
        })
      );
      setSelectedItemID(prepared.item.id);
      message.success("本地预处理已完成，当前状态为待提交");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "执行预处理失败");
    } finally {
      setPreparing(false);
    }
  };

  const clearWorkspace = async () => {
    setClearing(true);
    try {
      await localAgentRequest("/workspace/clear", { method: "POST" });
      setItems([]);
      setSelectedItemID(null);
      message.success("本地预处理工作区已清空");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "清空工作区失败");
    } finally {
      setClearing(false);
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

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>预处理工作区</Typography.Title>

      <Alert
        type="info"
        showIcon
        message="这里是本地预处理工作区，不是服务端素材库"
        description="原始视频、处理中间态、仅保存未提交的结果都只保留在本地 Agent 工作区，服务端不会读取或分析这些对象。"
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
            <Typography.Text type="secondary">
              已选择 {selectedFiles.length} 个文件
            </Typography.Text>
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
              title: "更新时间",
              dataIndex: "updated_at",
              render: (value?: string) => formatDateTime(value)
            },
            {
              title: "错误",
              render: (_, item) => item.last_error ? <Typography.Text type="danger">{item.last_error}</Typography.Text> : "-"
            }
          ]}
        />
      </Card>

      <Drawer
        width={920}
        open={!!selectedItem}
        title={selectedItem ? `预处理：${selectedItem.asset_name || selectedItem.original_file_name}` : "预处理"}
        onClose={() => setSelectedItemID(null)}
        destroyOnClose={false}
      >
        {selectedItem ? (
          <Space direction="vertical" size="large" className="wide-space">
            <Space wrap className="wide-space preprocess-toolbar">
              <Button onClick={() => openNeighbor(-1)} disabled={selectedIndex <= 0}>
                上一条
              </Button>
              <Button onClick={() => openNeighbor(1)} disabled={selectedIndex < 0 || selectedIndex >= items.length - 1}>
                下一条
              </Button>
              <Button loading={saving} onClick={() => void saveDraft()}>
                仅保存
              </Button>
              <Button type="primary" loading={preparing} onClick={() => void prepareItem()}>
                完成处理
              </Button>
            </Space>

            {selectedItem.last_error ? <Alert type="error" showIcon message={selectedItem.last_error} /> : null}

            <div className="preprocess-preview-grid">
              <Card size="small" title="原始视频">
                <video className="preprocess-video" controls src={selectedItem.source_url} />
              </Card>
              <Card size="small" title="Clean Shot 预览">
                {selectedItem.clean_shot_url ? (
                  <video className="preprocess-video" controls src={selectedItem.clean_shot_url} />
                ) : (
                  <Empty description="尚未生成 clean shot" />
                )}
              </Card>
            </div>

            <Form form={form} layout="vertical">
              <div className="preprocess-form-grid">
                <Form.Item name="asset_name" label="素材名称">
                  <Input placeholder="可选，提交入库时可作为素材名称" />
                </Form.Item>
                <Form.Item name="source_type" label="素材类型" rules={[{ required: true, message: "请选择素材类型" }]}>
                  <Select
                    options={[
                      { value: "visual_only", label: "纯画面" },
                      { value: "talking_head", label: "口播" }
                    ]}
                  />
                </Form.Item>
                <Form.Item name="source_in_ms" label="裁切起点（毫秒）" rules={[{ required: true, message: "请输入裁切起点" }]}>
                  <InputNumber min={0} style={{ width: "100%" }} />
                </Form.Item>
                <Form.Item name="source_out_ms" label="裁切终点（毫秒）" rules={[{ required: true, message: "请输入裁切终点" }]}>
                  <InputNumber min={1} style={{ width: "100%" }} />
                </Form.Item>
              </div>
              <Form.Item noStyle shouldUpdate={(prev, next) => prev.source_type !== next.source_type}>
                {({ getFieldValue }) =>
                  getFieldValue("source_type") === "talking_head" ? (
                    <Form.Item
                      name="transcript"
                      label="口播转写"
                      rules={[{ required: true, message: "口播素材必须填写转写内容" }]}
                    >
                      <Input.TextArea rows={5} placeholder="例如：[00:00:03:00]-[00:00:05:00] 大家好。" />
                    </Form.Item>
                  ) : null
                }
              </Form.Item>
              <Form.Item name="reviewer_notes" label="备注">
                <Input.TextArea rows={3} placeholder="记录本地预处理备注，不会自动进入服务端素材库" />
              </Form.Item>
            </Form>

            <Card size="small" title="本地分析结果">
              <Descriptions bordered size="small" column={2}>
                <Descriptions.Item label="状态">{workspaceStatusLabels[selectedItem.status]}</Descriptions.Item>
                <Descriptions.Item label="时长">{formatDuration(selectedItem.probe.duration_ms)}</Descriptions.Item>
                <Descriptions.Item label="分辨率">
                  {selectedItem.probe.width && selectedItem.probe.height ? `${selectedItem.probe.width} x ${selectedItem.probe.height}` : "-"}
                </Descriptions.Item>
                <Descriptions.Item label="帧率">{selectedItem.probe.fps || "-"}</Descriptions.Item>
                <Descriptions.Item label="视频编码">{selectedItem.probe.codec || "-"}</Descriptions.Item>
                <Descriptions.Item label="音频编码">{selectedItem.probe.audio_codec || "-"}</Descriptions.Item>
                <Descriptions.Item label="画面描述">
                  {selectedItem.analysis?.scene_description || "-"}
                </Descriptions.Item>
                <Descriptions.Item label="景别">
                  {selectedItem.analysis?.shot_size ? shotSizeLabels[selectedItem.analysis.shot_size] ?? selectedItem.analysis.shot_size : "-"}
                </Descriptions.Item>
                <Descriptions.Item label="运镜">
                  {selectedItem.analysis?.camera_movement
                    ? cameraMovementLabels[selectedItem.analysis.camera_movement] ?? selectedItem.analysis.camera_movement
                    : "-"}
                </Descriptions.Item>
                <Descriptions.Item label="可用性">{selectedItem.analysis?.usability_status || "-"}</Descriptions.Item>
              </Descriptions>
            </Card>

            <Card size="small" title="三帧抽样">
              {selectedItem.frame_snapshots.length > 0 ? (
                <div className="frame-grid">
                  {selectedItem.frame_snapshots.map((frame) => (
                    <div key={frame.frame_index} className="frame-card">
                      <img className="frame-image" src={frame.image_url} alt={`frame-${frame.frame_index}`} />
                      <Typography.Text type="secondary">
                        第 {frame.frame_index + 1} 帧 · {formatTimestamp(frame.timestamp_ms)}
                      </Typography.Text>
                    </div>
                  ))}
                </div>
              ) : (
                <Empty description="完成处理后会生成 10% / 50% / 90% 三帧" />
              )}
            </Card>
          </Space>
        ) : null}
      </Drawer>
    </Space>
  );
}
