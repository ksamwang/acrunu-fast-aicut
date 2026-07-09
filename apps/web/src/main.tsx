import React, { useEffect, useMemo, useState } from "react";
import ReactDOM from "react-dom/client";
import {
  Button,
  Card,
  ConfigProvider,
  Descriptions,
  Empty,
  Form,
  Input,
  InputNumber,
  Layout,
  Menu,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message
} from "antd";
import zhCN from "antd/locale/zh_CN";
import "./styles.css";

type User = {
  id: string;
  username: string;
  display_name: string;
  role: "admin" | "user";
};

type Session = {
  token: string;
  user: User;
};

type Product = {
  id: string;
  name: string;
  description?: string;
  category?: string;
  status: string;
};

type SellingPoint = {
  id: string;
  product_id: string;
  title: string;
  description?: string;
  priority: number;
  status: string;
};

type Asset = {
  id: string;
  product_id: string;
  asset_name?: string;
  storage_key: string;
  file_name: string;
  source_type: string;
  status: string;
  analysis_status?: string;
  usability_status?: string;
  manual_clean_status?: string;
  duration_ms?: number;
  width?: number;
  height?: number;
  fps?: number;
  codec?: string;
  has_audio?: boolean;
  audio_codec?: string;
  bitrate_kbps?: number;
  reviewer_notes?: string;
  scene_description?: string;
  shot_size?: string;
  camera_movement?: string;
  subjects?: string[];
  scene_tags?: string[];
  quality_tags?: string[];
  analysis_error?: string;
};

type AssetFrameSnapshot = {
  id: string;
  asset_id: string;
  frame_index: number;
  timestamp_ms: number;
  storage_key: string;
  width?: number;
  height?: number;
  created_at: string;
};

type AssetFrameResponse = {
  asset_id: string;
  frames: AssetFrameSnapshot[];
};

type AssetReviewPayload = {
  scene_description: string;
  shot_size: string;
  camera_movement: string;
  subjects: string[];
  scene_tags: string[];
  quality_tags: string[];
  usability_status: string;
  reviewer_notes: string;
};

type Task = {
  id: string;
  product_id?: string;
  created_by_user_id?: string;
  task_type: string;
  status: string;
  payload_summary?: Record<string, unknown>;
  asset_id?: string;
  duration_ms?: number;
  error_message?: string;
  retry_count: number;
  created_at: string;
  updated_at?: string;
  started_at?: string;
  finished_at?: string;
};

type SystemConfig = {
  key: string;
  value: unknown;
  type: string;
  is_secret: boolean;
  description?: string;
};

type ViewKey = "products" | "assets" | "tasks" | "settings";

async function apiRequest<T>(path: string, options: RequestInit = {}, token?: string): Promise<T> {
  const response = await fetch(path, {
    ...options,
    headers: {
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers
    }
  });

  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload?.error?.message ?? "Request failed");
  }
  return payload.data as T;
}

function useResource<T>(path: string, token: string, deps: React.DependencyList = []) {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(false);

  const reload = async () => {
    setLoading(true);
    try {
      setData(await apiRequest<T>(path, {}, token));
    } catch (error) {
      message.error(error instanceof Error ? error.message : "Load failed");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
  }, deps);

  return { data, loading, reload };
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

function renderTagList(items?: string[], emptyText = "-") {
  if (!items || items.length === 0) {
    return <Typography.Text type="secondary">{emptyText}</Typography.Text>;
  }

  return (
    <Space wrap size={[6, 6]}>
      {items.map((item) => (
        <Tag key={item}>{item}</Tag>
      ))}
    </Space>
  );
}

function LoginPage({ onLogin }: { onLogin: (session: Session) => void }) {
  const [loading, setLoading] = useState(false);

  return (
    <div className="login-shell" data-testid="login-page">
      <Card className="login-card" title="AICut Console">
        <Form
          layout="vertical"
          initialValues={{ username: "admin", password: "admin" }}
          onFinish={async (values) => {
            setLoading(true);
            try {
              const session = await apiRequest<Session>("/api/auth/login", {
                method: "POST",
                body: JSON.stringify(values)
              });
              onLogin(session);
            } catch (error) {
              message.error(error instanceof Error ? error.message : "Login failed");
            } finally {
              setLoading(false);
            }
          }}
        >
          <Form.Item name="username" label="Username" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="password" label="Password" rules={[{ required: true }]}>
            <Input.Password />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={loading} block data-testid="login-submit">
            Sign In
          </Button>
        </Form>
      </Card>
    </div>
  );
}

function ProductsPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token);
  const [selectedProductID, setSelectedProductID] = useState<string | null>(null);
  const sellingPoints = useResource<SellingPoint[]>(
    selectedProductID ? `/api/products/${selectedProductID}/selling-points` : "/api/products/none/selling-points",
    token,
    [selectedProductID]
  );
  const [productOpen, setProductOpen] = useState(false);
  const [sellingPointOpen, setSellingPointOpen] = useState(false);
  const [productForm] = Form.useForm();
  const [sellingPointForm] = Form.useForm();

  const createProduct = async () => {
    const values = await productForm.validateFields();
    await apiRequest<Product>(
      "/api/products",
      {
        method: "POST",
        body: JSON.stringify({ ...values, metadata: {} })
      },
      token
    );
    setProductOpen(false);
    productForm.resetFields();
    await products.reload();
  };

  const createSellingPoint = async () => {
    if (!selectedProductID) {
      message.warning("Select a product first");
      return;
    }
    const values = await sellingPointForm.validateFields();
    await apiRequest<SellingPoint>(
      `/api/products/${selectedProductID}/selling-points`,
      {
        method: "POST",
        body: JSON.stringify(values)
      },
      token
    );
    setSellingPointOpen(false);
    sellingPointForm.resetFields();
    await sellingPoints.reload();
  };

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>Products</Typography.Title>
      <Card title="Product List" extra={<Button type="primary" onClick={() => setProductOpen(true)}>New Product</Button>}>
        <Table<Product>
          rowKey="id"
          loading={products.loading}
          dataSource={products.data ?? []}
          onRow={(record) => ({ onClick: () => setSelectedProductID(record.id) })}
          rowClassName={(record) => (record.id === selectedProductID ? "selected-row" : "")}
          columns={[
            { title: "Product", dataIndex: "name" },
            { title: "Category", dataIndex: "category" },
            { title: "Status", dataIndex: "status", render: (status) => <Tag>{status}</Tag> }
          ]}
        />
      </Card>
      <Card title="Selling Points" extra={<Button disabled={!selectedProductID} onClick={() => setSellingPointOpen(true)}>New Selling Point</Button>}>
        <Table<SellingPoint>
          rowKey="id"
          loading={sellingPoints.loading}
          dataSource={selectedProductID ? sellingPoints.data ?? [] : []}
          columns={[
            { title: "Title", dataIndex: "title" },
            { title: "Priority", dataIndex: "priority" },
            { title: "Status", dataIndex: "status", render: (status) => <Tag>{status}</Tag> }
          ]}
        />
      </Card>

      <Modal title="New Product" open={productOpen} onOk={createProduct} onCancel={() => setProductOpen(false)}>
        <Form form={productForm} layout="vertical">
          <Form.Item name="name" label="Product Name" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="category" label="Category">
            <Input />
          </Form.Item>
          <Form.Item name="description" label="Description">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title="New Selling Point" open={sellingPointOpen} onOk={createSellingPoint} onCancel={() => setSellingPointOpen(false)}>
        <Form form={sellingPointForm} layout="vertical">
          <Form.Item name="title" label="Title" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="priority" label="Priority" initialValue={0}>
            <InputNumber min={0} />
          </Form.Item>
          <Form.Item name="description" label="Description">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>
    </Space>
  );
}

function AssetsPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token);
  const [productForSellingPoints, setProductForSellingPoints] = useState<string>("");
  const sellingPoints = useResource<SellingPoint[]>(
    productForSellingPoints ? `/api/products/${productForSellingPoints}/selling-points` : "/api/products/none/selling-points",
    token,
    [productForSellingPoints]
  );
  const [filters, setFilters] = useState({
    productID: "",
    sellingPointID: "",
    sourceType: "",
    status: "",
    tag: "",
    minDurationMs: "",
    maxDurationMs: "",
    hasAudio: ""
  });
  const [selectedAsset, setSelectedAsset] = useState<Asset | null>(null);
  const [frames, setFrames] = useState<AssetFrameSnapshot[]>([]);
  const [framesLoading, setFramesLoading] = useState(false);
  const [editingAnalysis, setEditingAnalysis] = useState(false);
  const [savingAnalysis, setSavingAnalysis] = useState(false);
  const [reviewForm] = Form.useForm<AssetReviewPayload>();

  const assetPath = useMemo(() => {
    const params = new URLSearchParams();
    if (filters.productID) {
      params.set("product_id", filters.productID);
    }
    if (filters.sellingPointID) {
      params.set("selling_point_id", filters.sellingPointID);
    }
    if (filters.sourceType) {
      params.set("source_type", filters.sourceType);
    }
    if (filters.status) {
      params.set("status", filters.status);
    }
    if (filters.tag) {
      params.set("tag", filters.tag);
    }
    if (filters.minDurationMs) {
      params.set("min_duration_ms", filters.minDurationMs);
    }
    if (filters.maxDurationMs) {
      params.set("max_duration_ms", filters.maxDurationMs);
    }
    if (filters.hasAudio) {
      params.set("has_audio", filters.hasAudio);
    }
    const query = params.toString();
    return query ? `/api/assets?${query}` : "/api/assets";
  }, [filters]);

  const assets = useResource<Asset[]>(assetPath, token, [assetPath]);
  const productNameByID = useMemo(() => {
    const map = new Map<string, string>();
    for (const product of products.data ?? []) {
      map.set(product.id, product.name);
    }
    return map;
  }, [products.data]);

  useEffect(() => {
    if (!selectedAsset) {
      setFrames([]);
      return;
    }

    const loadFrames = async () => {
      setFramesLoading(true);
      try {
        const response = await apiRequest<AssetFrameResponse>(`/api/assets/${selectedAsset.id}/frames`, {}, token);
        setFrames(response.frames);
      } catch (error) {
        setFrames([]);
        message.error(error instanceof Error ? error.message : "Failed to load frame previews");
      } finally {
        setFramesLoading(false);
      }
    };

    void loadFrames();
  }, [selectedAsset, token]);

  useEffect(() => {
    if (!selectedAsset) {
      reviewForm.resetFields();
      setEditingAnalysis(false);
      return;
    }

    reviewForm.setFieldsValue({
      scene_description: selectedAsset.scene_description || "",
      shot_size: selectedAsset.shot_size || "",
      camera_movement: selectedAsset.camera_movement || "",
      subjects: selectedAsset.subjects || [],
      scene_tags: selectedAsset.scene_tags || [],
      quality_tags: selectedAsset.quality_tags || [],
      usability_status: selectedAsset.usability_status || "usable",
      reviewer_notes: selectedAsset.reviewer_notes || ""
    });
  }, [reviewForm, selectedAsset]);

  const saveAnalysisReview = async () => {
    if (!selectedAsset) {
      return;
    }

    const values = await reviewForm.validateFields();
    setSavingAnalysis(true);
    try {
      const updated = await apiRequest<Asset>(
        `/api/assets/${selectedAsset.id}/review`,
        {
          method: "PUT",
          body: JSON.stringify(values)
        },
        token
      );
      setSelectedAsset(updated);
      setEditingAnalysis(false);
      await assets.reload();
      message.success("Asset review updated");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "Failed to update asset review");
    } finally {
      setSavingAnalysis(false);
    }
  };

  return (
    <div data-testid="assets-page">
      <Space direction="vertical" size="middle" className="page-stack">
        <Typography.Title level={3}>Asset Library</Typography.Title>
        <Card title="Local Agent Entry">
          <Space direction="vertical" className="wide-space">
            <Input defaultValue="http://127.0.0.1:58721" />
            <Button type="primary">Open Local Agent</Button>
          </Space>
        </Card>
        <Card title="Filters">
          <Space wrap>
            <Select
              data-testid="asset-filter-product"
              value={filters.productID || undefined}
              placeholder="Product"
              allowClear
              style={{ minWidth: 180 }}
              options={(products.data ?? []).map((product) => ({ value: product.id, label: product.name }))}
              onChange={(value) => {
                const nextValue = value ?? "";
                setFilters((current) => ({ ...current, productID: nextValue, sellingPointID: "" }));
                setProductForSellingPoints(nextValue);
              }}
            />
            <Select
              data-testid="asset-filter-selling-point"
              value={filters.sellingPointID || undefined}
              placeholder="Selling Point"
              allowClear
              style={{ minWidth: 180 }}
              disabled={!filters.productID}
              options={(sellingPoints.data ?? []).map((item) => ({ value: item.id, label: item.title }))}
              onChange={(value) => setFilters((current) => ({ ...current, sellingPointID: value ?? "" }))}
            />
            <Select
              data-testid="asset-filter-source-type"
              value={filters.sourceType || undefined}
              placeholder="Source Type"
              allowClear
              style={{ minWidth: 160 }}
              options={[
                { value: "visual_only", label: "visual_only" },
                { value: "talking_head", label: "talking_head" }
              ]}
              onChange={(value) => setFilters((current) => ({ ...current, sourceType: value ?? "" }))}
            />
            <Select
              data-testid="asset-filter-status"
              value={filters.status || undefined}
              placeholder="Status"
              allowClear
              style={{ minWidth: 160 }}
              options={[
                { value: "ready", label: "ready" },
                { value: "failed", label: "failed" },
                { value: "uploaded", label: "uploaded" },
                { value: "archived", label: "archived" }
              ]}
              onChange={(value) => setFilters((current) => ({ ...current, status: value ?? "" }))}
            />
            <Input
              data-testid="asset-filter-tag"
              value={filters.tag}
              placeholder="Tag or keyword"
              style={{ width: 180 }}
              onChange={(event) => setFilters((current) => ({ ...current, tag: event.target.value }))}
            />
            <Input
              data-testid="asset-filter-min-duration"
              value={filters.minDurationMs}
              placeholder="Min ms"
              style={{ width: 120 }}
              onChange={(event) => setFilters((current) => ({ ...current, minDurationMs: event.target.value }))}
            />
            <Input
              data-testid="asset-filter-max-duration"
              value={filters.maxDurationMs}
              placeholder="Max ms"
              style={{ width: 120 }}
              onChange={(event) => setFilters((current) => ({ ...current, maxDurationMs: event.target.value }))}
            />
            <Select
              data-testid="asset-filter-has-audio"
              value={filters.hasAudio || undefined}
              placeholder="Has Audio"
              allowClear
              style={{ minWidth: 140 }}
              options={[
                { value: "true", label: "audio only" },
                { value: "false", label: "mute only" }
              ]}
              onChange={(value) => setFilters((current) => ({ ...current, hasAudio: value ?? "" }))}
            />
            <Button
              onClick={() => {
                setFilters({
                  productID: "",
                  sellingPointID: "",
                  sourceType: "",
                  status: "",
                  tag: "",
                  minDurationMs: "",
                  maxDurationMs: "",
                  hasAudio: ""
                });
                setProductForSellingPoints("");
              }}
            >
              Reset
            </Button>
            <Button onClick={assets.reload}>Refresh</Button>
          </Space>
        </Card>
        <Card title="Assets">
          <Table<Asset>
            rowKey="id"
            loading={assets.loading}
            dataSource={assets.data ?? []}
            pagination={false}
            onRow={(record) => ({ onClick: () => setSelectedAsset(record) })}
            columns={[
              {
                title: "Asset",
                render: (_, asset) => (
                  <Button type="link" className="table-link-button" onClick={() => setSelectedAsset(asset)}>
                    {asset.asset_name || asset.file_name}
                  </Button>
                )
              },
              { title: "File", dataIndex: "file_name" },
              {
                title: "Product",
                render: (_, asset) => productNameByID.get(asset.product_id) ?? asset.product_id ?? "-"
              },
              { title: "Type", dataIndex: "source_type" },
              { title: "Status", dataIndex: "status", render: (status) => <Tag>{status}</Tag> },
              {
                title: "Analysis",
                dataIndex: "analysis_status",
                render: (status) => (status ? <Tag color="blue">{status}</Tag> : "-")
              },
              { title: "Duration", render: (_, asset) => formatDuration(asset.duration_ms) },
              {
                title: "Shot Size",
                dataIndex: "shot_size",
                render: (value) => value || "-"
              },
              {
                title: "Movement",
                dataIndex: "camera_movement",
                render: (value) => value || "-"
              },
              {
                title: "Tags",
                render: (_, asset) => (
                  <Typography.Text className="summary-text">
                    {asset.scene_tags && asset.scene_tags.length > 0
                      ? asset.scene_tags.slice(0, 3).join(", ")
                      : asset.subjects && asset.subjects.length > 0
                        ? asset.subjects.slice(0, 3).join(", ")
                        : "-"}
                  </Typography.Text>
                )
              },
              {
                title: "Resolution",
                render: (_, asset) => (asset.width && asset.height ? `${asset.width}x${asset.height}` : "-")
              }
            ]}
          />
        </Card>
      </Space>

      <Modal
        title={selectedAsset ? `Asset Detail: ${selectedAsset.asset_name || selectedAsset.file_name}` : "Asset Detail"}
        open={selectedAsset !== null}
        footer={null}
        width={960}
        onCancel={() => setSelectedAsset(null)}
      >
        {selectedAsset ? (
          <Space direction="vertical" size="large" className="wide-space" data-testid="asset-detail-modal">
            <Descriptions bordered column={2} size="small">
              <Descriptions.Item label="File">{selectedAsset.file_name}</Descriptions.Item>
              <Descriptions.Item label="Source Type">{selectedAsset.source_type}</Descriptions.Item>
              <Descriptions.Item label="Status">{selectedAsset.status}</Descriptions.Item>
              <Descriptions.Item label="Analysis">{selectedAsset.analysis_status || "-"}</Descriptions.Item>
              <Descriptions.Item label="Duration">{formatDuration(selectedAsset.duration_ms)}</Descriptions.Item>
              <Descriptions.Item label="Resolution">
                {selectedAsset.width && selectedAsset.height ? `${selectedAsset.width}x${selectedAsset.height}` : "-"}
              </Descriptions.Item>
              <Descriptions.Item label="FPS">{selectedAsset.fps ?? "-"}</Descriptions.Item>
              <Descriptions.Item label="Codec">{selectedAsset.codec || "-"}</Descriptions.Item>
              <Descriptions.Item label="Has Audio">{selectedAsset.has_audio ? "yes" : "no"}</Descriptions.Item>
              <Descriptions.Item label="Audio Codec">{selectedAsset.audio_codec || "-"}</Descriptions.Item>
              <Descriptions.Item label="Bitrate">{selectedAsset.bitrate_kbps ? `${selectedAsset.bitrate_kbps} kbps` : "-"}</Descriptions.Item>
              <Descriptions.Item label="Manual Clean">{selectedAsset.manual_clean_status || "-"}</Descriptions.Item>
              <Descriptions.Item label="Usability">{selectedAsset.usability_status || "-"}</Descriptions.Item>
            </Descriptions>

            <Card title="Analysis Summary">
              <Space direction="vertical" className="wide-space">
                <Space>
                  <Button
                    size="small"
                    onClick={() => {
                      setEditingAnalysis((current) => !current);
                      reviewForm.setFieldsValue({
                        scene_description: selectedAsset.scene_description || "",
                        shot_size: selectedAsset.shot_size || "",
                        camera_movement: selectedAsset.camera_movement || "",
                        subjects: selectedAsset.subjects || [],
                        scene_tags: selectedAsset.scene_tags || [],
                        quality_tags: selectedAsset.quality_tags || [],
                        usability_status: selectedAsset.usability_status || "usable",
                        reviewer_notes: selectedAsset.reviewer_notes || ""
                      });
                    }}
                  >
                    {editingAnalysis ? "Cancel Edit" : "Edit Tags"}
                  </Button>
                  {editingAnalysis ? (
                    <Button type="primary" size="small" loading={savingAnalysis} onClick={saveAnalysisReview} data-testid="save-asset-review">
                      Save Review
                    </Button>
                  ) : null}
                </Space>

                {editingAnalysis ? (
                  <Form form={reviewForm} layout="vertical" data-testid="asset-review-form">
                    <Form.Item name="scene_description" label="Scene Description">
                      <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item name="shot_size" label="Shot Size">
                      <Select
                        allowClear
                        options={[
                          { value: "close_up", label: "close_up" },
                          { value: "medium_close_up", label: "medium_close_up" },
                          { value: "medium_shot", label: "medium_shot" },
                          { value: "wide_shot", label: "wide_shot" }
                        ]}
                      />
                    </Form.Item>
                    <Form.Item name="camera_movement" label="Camera Movement">
                      <Select
                        allowClear
                        options={[
                          { value: "static", label: "static" },
                          { value: "slow_push_in", label: "slow_push_in" },
                          { value: "pan", label: "pan" },
                          { value: "handheld", label: "handheld" }
                        ]}
                      />
                    </Form.Item>
                    <Form.Item name="subjects" label="Subjects">
                      <Select mode="tags" tokenSeparators={[","]} open={false} />
                    </Form.Item>
                    <Form.Item name="scene_tags" label="Scene Tags">
                      <Select mode="tags" tokenSeparators={[","]} open={false} />
                    </Form.Item>
                    <Form.Item name="quality_tags" label="Quality Tags">
                      <Select mode="tags" tokenSeparators={[","]} open={false} />
                    </Form.Item>
                    <Form.Item name="usability_status" label="Usability Status">
                      <Select
                        options={[
                          { value: "usable", label: "usable" },
                          { value: "needs_review", label: "needs_review" },
                          { value: "discarded", label: "discarded" }
                        ]}
                      />
                    </Form.Item>
                    <Form.Item name="reviewer_notes" label="Reviewer Notes">
                      <Input.TextArea rows={2} />
                    </Form.Item>
                  </Form>
                ) : (
                  <Descriptions bordered column={1} size="small" data-testid="asset-analysis-panel">
                    <Descriptions.Item label="Scene Description">
                      {selectedAsset.scene_description || <Typography.Text type="secondary">No analysis output yet.</Typography.Text>}
                    </Descriptions.Item>
                    <Descriptions.Item label="Shot Size">
                      {selectedAsset.shot_size || "-"}
                    </Descriptions.Item>
                    <Descriptions.Item label="Camera Movement">
                      {selectedAsset.camera_movement || "-"}
                    </Descriptions.Item>
                    <Descriptions.Item label="Subjects">
                      {renderTagList(selectedAsset.subjects, "No detected subjects")}
                    </Descriptions.Item>
                    <Descriptions.Item label="Scene Tags">
                      {renderTagList(selectedAsset.scene_tags, "No scene tags")}
                    </Descriptions.Item>
                    <Descriptions.Item label="Quality Tags">
                      {renderTagList(selectedAsset.quality_tags, "No quality issues")}
                    </Descriptions.Item>
                    <Descriptions.Item label="Reviewer Notes">
                      {selectedAsset.reviewer_notes || <Typography.Text type="secondary">None</Typography.Text>}
                    </Descriptions.Item>
                    <Descriptions.Item label="Analysis Error">
                      {selectedAsset.analysis_error || <Typography.Text type="secondary">None</Typography.Text>}
                    </Descriptions.Item>
                  </Descriptions>
                )}
              </Space>
            </Card>

            <Card title="Frame Previews" loading={framesLoading}>
              {frames.length === 0 ? (
                <Empty description="No extracted frames yet" />
              ) : (
                <div className="frame-grid">
                  {frames.map((frame) => (
                    <div key={frame.id} className="frame-card" data-testid="frame-card">
                      <img
                        className="frame-image"
                        src={`/storage/${encodeURI(frame.storage_key)}`}
                        alt={`frame-${frame.frame_index}`}
                      />
                      <Typography.Text strong>{formatTimestamp(frame.timestamp_ms)}</Typography.Text>
                      <Typography.Text type="secondary">Frame #{frame.frame_index}</Typography.Text>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          </Space>
        ) : null}
      </Modal>
    </div>
  );
}

function TasksPage({ token }: { token: string }) {
  const tasks = useResource<Task[]>("/api/tasks", token);
  const [creating, setCreating] = useState(false);
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const createTask = async () => {
    setCreating(true);
    try {
      await apiRequest<Task>("/api/tasks/test", { method: "POST" }, token);
      await tasks.reload();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "Create task failed");
    } finally {
      setCreating(false);
    }
  };

  const openTaskDetail = async (taskID: string) => {
    setDetailLoading(true);
    try {
      const task = await apiRequest<Task>(`/api/tasks/${taskID}`, {}, token);
      setSelectedTask(task);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "Load task failed");
    } finally {
      setDetailLoading(false);
    }
  };

  return (
    <div data-testid="tasks-page">
      <Space direction="vertical" size="middle" className="page-stack">
        <Typography.Title level={3}>Tasks</Typography.Title>
        <Card title="Batch Edit Tasks" extra={<Button type="primary" loading={creating} onClick={createTask}>Create Test Task</Button>}>
          <Table<Task>
            rowKey="id"
            loading={tasks.loading}
            dataSource={tasks.data ?? []}
            onRow={(record) => ({ onClick: () => void openTaskDetail(record.id) })}
            columns={[
              {
                title: "Task ID",
                dataIndex: "id",
                render: (value: string, task) => (
                  <Button type="link" className="table-link-button" onClick={() => void openTaskDetail(task.id)}>
                    {value}
                  </Button>
                )
              },
              { title: "Type", dataIndex: "task_type" },
              { title: "Status", dataIndex: "status", render: (status) => <Tag>{status}</Tag> },
              { title: "Asset", dataIndex: "asset_id", render: (value) => value || "-" },
              { title: "Retry", dataIndex: "retry_count" },
              { title: "Duration", dataIndex: "duration_ms", render: (value) => (value ? `${value} ms` : "-") },
              { title: "Created At", dataIndex: "created_at", render: (value) => formatDateTime(value) }
            ]}
          />
        </Card>
      </Space>

      <Modal
        title={selectedTask ? `Task Detail: ${selectedTask.id}` : "Task Detail"}
        open={selectedTask !== null}
        footer={null}
        width={840}
        confirmLoading={detailLoading}
        onCancel={() => setSelectedTask(null)}
      >
        {selectedTask ? (
          <Descriptions bordered column={1} size="small" data-testid="task-detail-modal">
            <Descriptions.Item label="Task Type">{selectedTask.task_type}</Descriptions.Item>
            <Descriptions.Item label="Status">
              <Tag>{selectedTask.status}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Asset ID">{selectedTask.asset_id || "-"}</Descriptions.Item>
            <Descriptions.Item label="Retry Count">{selectedTask.retry_count}</Descriptions.Item>
            <Descriptions.Item label="Duration">{selectedTask.duration_ms ? `${selectedTask.duration_ms} ms` : "-"}</Descriptions.Item>
            <Descriptions.Item label="Created At">{formatDateTime(selectedTask.created_at)}</Descriptions.Item>
            <Descriptions.Item label="Started At">{formatDateTime(selectedTask.started_at)}</Descriptions.Item>
            <Descriptions.Item label="Finished At">{formatDateTime(selectedTask.finished_at)}</Descriptions.Item>
            <Descriptions.Item label="Error Message">
              {selectedTask.error_message || <Typography.Text type="secondary">None</Typography.Text>}
            </Descriptions.Item>
            <Descriptions.Item label="Payload Summary">
              {selectedTask.payload_summary && Object.keys(selectedTask.payload_summary).length > 0 ? (
                <pre className="json-block">{JSON.stringify(selectedTask.payload_summary, null, 2)}</pre>
              ) : (
                <Typography.Text type="secondary">No payload summary</Typography.Text>
              )}
            </Descriptions.Item>
          </Descriptions>
        ) : null}
      </Modal>
    </div>
  );
}

function SettingsPage({ token }: { token: string }) {
  const configs = useResource<SystemConfig[]>("/api/admin/system-configs", token);

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>System Settings</Typography.Title>
      <Card title="Model and Concurrency" extra={<Button onClick={configs.reload}>Refresh</Button>}>
        <Table<SystemConfig>
          rowKey="key"
          loading={configs.loading}
          dataSource={configs.data ?? []}
          columns={[
            { title: "Key", dataIndex: "key" },
            { title: "Value", dataIndex: "value", render: (value) => JSON.stringify(value) },
            { title: "Type", dataIndex: "type" },
            { title: "Description", dataIndex: "description" }
          ]}
        />
      </Card>
    </Space>
  );
}

function ConsoleApp({ session, onLogout }: { session: Session; onLogout: () => void }) {
  const [view, setView] = useState<ViewKey>("products");
  const menuItems = [
    { key: "products", label: "Products" },
    { key: "assets", label: "Assets" },
    { key: "tasks", label: "Tasks" },
    ...(session.user.role === "admin" ? [{ key: "settings", label: "Settings" }] : [])
  ];

  return (
    <Layout className="app-shell" data-testid="console-app">
      <Layout.Sider width={220} theme="light">
        <div className="brand">AICut</div>
        <Menu selectedKeys={[view]} items={menuItems} onClick={(item) => setView(item.key as ViewKey)} />
      </Layout.Sider>
      <Layout>
        <Layout.Header className="topbar">
          <Space>
            <Tag color={session.user.role === "admin" ? "blue" : "default"}>{session.user.role}</Tag>
            <Typography.Text>{session.user.display_name}</Typography.Text>
            <Button onClick={onLogout}>Sign Out</Button>
          </Space>
        </Layout.Header>
        <Layout.Content className="content">
          {view === "products" && <ProductsPage token={session.token} />}
          {view === "assets" && <AssetsPage token={session.token} />}
          {view === "tasks" && <TasksPage token={session.token} />}
          {view === "settings" && session.user.role === "admin" && <SettingsPage token={session.token} />}
        </Layout.Content>
      </Layout>
    </Layout>
  );
}

function App() {
  const [session, setSession] = useState<Session | null>(null);

  return session ? (
    <ConsoleApp session={session} onLogout={() => setSession(null)} />
  ) : (
    <LoginPage onLogin={setSession} />
  );
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN}>
      <App />
    </ConfigProvider>
  </React.StrictMode>
);
