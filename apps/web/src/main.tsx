import React, { useEffect, useMemo, useState } from "react";
import ReactDOM from "react-dom/client";
import {
  Alert,
  Button,
  Card,
  ConfigProvider,
  Descriptions,
  Divider,
  Drawer,
  Empty,
  Form,
  Input,
  InputNumber,
  Layout,
  Menu,
  Modal,
  Pagination,
  Popconfirm,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
  message
} from "antd";
import zhCN from "antd/locale/zh_CN";
import { PreprocessPage } from "./preprocess-page";
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
  asset_count?: number;
};

type ProductStats = {
  product_id: string;
  asset_count: number;
  usable_asset_count: number;
  pending_analysis_count: number;
};

type Asset = {
  id: string;
  product_id: string;
  asset_name?: string;
  storage_key: string;
  file_name: string;
  source_original_name?: string;
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
  model_labels?: Record<string, unknown>;
  model_result?: Record<string, unknown>;
  review_overrides?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  analysis_error?: string;
  updated_at?: string;
  analyzed_at?: string;
};

type AssetSellingPointPayload = {
  selling_point_ids: string[];
};

type AssetListResponse = {
  items: Asset[];
  total: number;
  page: number;
  page_size: number;
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

type AssetSpeechSegment = {
  id: string;
  asset_id: string;
  start_ms: number;
  end_ms: number;
  transcript: string;
  confidence?: number;
  source: string;
  status: string;
  created_at: string;
  updated_at: string;
};

type AssetEmbeddingTarget = {
  object_type: string;
  object_id: string;
  asset_id: string;
  text: string;
  metadata?: Record<string, unknown>;
};

type AssetSemanticPreview = {
  asset_id: string;
  open_semantic_description: string;
  embedding_targets: AssetEmbeddingTarget[];
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

type AssetBusinessTagPayload = {
  is_curated: boolean;
  business_tags: string[];
  narrative_roles: string[];
  usage_notes: string;
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

type OpenAICompatibleSettings = {
  provider: string;
  base_url: string;
  api_key_configured: boolean;
  llm_model: string;
  vlm_model: string;
  embedding_model: string;
};

type RuntimeSettings = {
  llm_max_concurrency: number;
  vlm_max_concurrency: number;
  asr_max_concurrency: number;
  tts_max_concurrency: number;
  render_max_concurrency: number;
  task_max_queued_per_user: number;
  task_max_running_per_user: number;
  vlm_timeout_seconds: number;
  vlm_max_retries: number;
};

type ModelDiscoveryResult = {
  models: Array<{
    id: string;
  }>;
};

type ModelSelectOption = {
  value: string;
  label: string;
};

type ModelProvider = {
  id: string;
  name: string;
  provider_type: string;
  base_url: string;
  api_key_configured: boolean;
  enabled: boolean;
};

type ModelCapabilitySetting = {
  capability: string;
  provider_id: string;
  model: string;
};

type ModelCapabilitySettings = {
  llm: ModelCapabilitySetting;
  vlm: ModelCapabilitySetting;
  embedding: ModelCapabilitySetting;
};

type ViewKey = "products" | "preprocess" | "assets" | "tasks" | "settings";

const defaultView: ViewKey = "products";
const viewKeys: ViewKey[] = ["products", "preprocess", "assets", "tasks", "settings"];
const storedSessionKey = "aicut.session";

function isViewKey(value: string): value is ViewKey {
  return viewKeys.includes(value as ViewKey);
}

function normalizeViewForRole(view: ViewKey, role?: User["role"]): ViewKey {
  if (view === "settings" && role !== "admin") {
    return defaultView;
  }
  return view;
}

function readHashView(role?: User["role"]): ViewKey {
  const raw = window.location.hash.replace(/^#\/?/, "").split(/[/?&]/)[0];
  if (!isViewKey(raw)) {
    return defaultView;
  }
  return normalizeViewForRole(raw, role);
}

function writeHashView(view: ViewKey) {
  const nextHash = `#/${view}`;
  if (window.location.hash !== nextHash) {
    window.location.hash = nextHash;
  }
}

function readStoredSession(): Session | null {
  try {
    const raw = window.localStorage.getItem(storedSessionKey);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as Session;
    if (!parsed?.token || !parsed?.user?.role) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

function storeSession(session: Session) {
  window.localStorage.setItem(storedSessionKey, JSON.stringify(session));
}

function clearStoredSession() {
  window.localStorage.removeItem(storedSessionKey);
}

const roleLabels: Record<string, string> = {
  admin: "管理员",
  user: "用户"
};

const productStatusLabels: Record<string, string> = {
  active: "启用",
  archived: "已归档"
};

const assetStatusLabels: Record<string, string> = {
  active: "启用",
  archived: "已归档",
  uploaded: "已上传",
  ready: "可用"
};

const analysisStatusLabels: Record<string, string> = {
  pending_analysis: "待分析",
  analyzing: "分析中",
  ready: "已完成",
  failed: "失败"
};

const sourceTypeLabels: Record<string, string> = {
  visual_only: "纯画面",
  talking_head: "口播",
  "local-agent": "本地代理",
  "server-upload": "服务端上传",
  "manual-import": "手动导入"
};

const usabilityStatusLabels: Record<string, string> = {
  usable: "可用",
  needs_review: "待复核",
  discarded: "废弃"
};

const manualCleanStatusLabels: Record<string, string> = {
  cleaned: "已清洗"
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

const taskStatusLabels: Record<string, string> = {
  queued: "排队中",
  running: "执行中",
  completed: "已完成",
  failed: "失败"
};

const taskTypeLabels: Record<string, string> = {
  asset_analyze: "素材分析",
  asset_extract_frames: "素材抽帧",
  test: "测试任务"
};

function translateValue(value: string | undefined | null, labels: Record<string, string>) {
  if (!value) {
    return "-";
  }
  return labels[value] ?? value;
}

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
    throw new Error(payload?.error?.message ?? "请求失败");
  }
  return payload.data as T;
}

function useResource<T>(path: string | null, token: string, deps: React.DependencyList = []) {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(false);

  const reload = async () => {
    if (!path) {
      setData(null);
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      setData(await apiRequest<T>(path, {}, token));
    } catch (error) {
      message.error(error instanceof Error ? error.message : "加载失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
  }, [path, ...deps]);

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

function assetVideoURL(asset: Asset) {
  return `/storage/${encodeURI(asset.storage_key)}`;
}

function assetFileDisplayName(asset: Asset) {
  if (asset.source_original_name && /^clean-shot\.[^.]+$/i.test(asset.file_name)) {
    return asset.source_original_name;
  }
  return asset.file_name || asset.source_original_name || "-";
}

function assetDisplayTitle(asset: Asset) {
  return asset.asset_name || assetFileDisplayName(asset);
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
      <Card className="login-card" title="AICut 控制台">
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
              message.error(error instanceof Error ? error.message : "登录失败");
            } finally {
              setLoading(false);
            }
          }}
        >
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: "请输入用户名" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, message: "请输入密码" }]}>
            <Input.Password />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={loading} block data-testid="login-submit">
            登录
          </Button>
        </Form>
      </Card>
    </div>
  );
}

function LegacyProductsPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token);
  const [selectedProductID, setSelectedProductID] = useState<string | null>(null);
  const [selectedSellingPointID, setSelectedSellingPointID] = useState<string | null>(null);
  const sellingPoints = useResource<SellingPoint[]>(
    selectedProductID ? `/api/products/${selectedProductID}/selling-points` : null,
    token,
    [selectedProductID]
  );
  const productStats = useResource<ProductStats>(
    selectedProductID ? `/api/products/${selectedProductID}/stats` : null,
    token,
    [selectedProductID]
  );
  const sellingPointAssets = useResource<Asset[]>(
    selectedSellingPointID ? `/api/selling-points/${selectedSellingPointID}/assets` : null,
    token,
    [selectedSellingPointID]
  );
  const [productOpen, setProductOpen] = useState(false);
  const [sellingPointOpen, setSellingPointOpen] = useState(false);
  const [editingProduct, setEditingProduct] = useState<Product | null>(null);
  const [editingSellingPoint, setEditingSellingPoint] = useState<SellingPoint | null>(null);
  const [productForm] = Form.useForm();
  const [sellingPointForm] = Form.useForm();

  const saveProduct = async () => {
    const values = await productForm.validateFields();
    await apiRequest<Product>(
      editingProduct ? `/api/products/${editingProduct.id}` : "/api/products",
      {
        method: editingProduct ? "PUT" : "POST",
        body: JSON.stringify({ ...values, metadata: {} })
      },
      token
    );
    setProductOpen(false);
    setEditingProduct(null);
    productForm.resetFields();
    await products.reload();
  };

  const saveSellingPoint = async () => {
    if (!selectedProductID) {
      message.warning("请先选择产品");
      return;
    }
    const values = await sellingPointForm.validateFields();
    await apiRequest<SellingPoint>(
      editingSellingPoint
        ? `/api/selling-points/${editingSellingPoint.id}`
        : `/api/products/${selectedProductID}/selling-points`,
      {
        method: editingSellingPoint ? "PUT" : "POST",
        body: JSON.stringify(values)
      },
      token
    );
    setSellingPointOpen(false);
    setEditingSellingPoint(null);
    sellingPointForm.resetFields();
    await sellingPoints.reload();
  };

  const selectedProduct = (products.data ?? []).find((product) => product.id === selectedProductID) ?? null;
  const selectedSellingPoint = (sellingPoints.data ?? []).find((item) => item.id === selectedSellingPointID) ?? null;
  const createProduct = saveProduct;
  const createSellingPoint = saveSellingPoint;

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>产品</Typography.Title>
      <Card title="产品列表" extra={<Button type="primary" onClick={() => setProductOpen(true)}>新建产品</Button>}>
        <Table<Product>
          rowKey="id"
          loading={products.loading}
          dataSource={products.data ?? []}
          onRow={(record) => ({ onClick: () => {
            setSelectedProductID(record.id);
            setSelectedSellingPointID(null);
          } })}
          rowClassName={(record) => (record.id === selectedProductID ? "selected-row" : "")}
          columns={[
            { title: "产品", dataIndex: "name" },
            { title: "分类", dataIndex: "category" },
            { title: "状态", dataIndex: "status", render: (status) => <Tag>{translateValue(status, productStatusLabels)}</Tag> }
          ]}
        />
      </Card>
      <Card title="产品统计">
        {selectedProduct ? (
          <Descriptions bordered column={3} size="small">
            <Descriptions.Item label="产品">{selectedProduct.name}</Descriptions.Item>
            <Descriptions.Item label="素材数">
              <span data-testid="product-asset-count">{productStats.data?.asset_count ?? 0}</span>
            </Descriptions.Item>
            <Descriptions.Item label="可用素材">
              <span data-testid="product-usable-asset-count">{productStats.data?.usable_asset_count ?? 0}</span>
            </Descriptions.Item>
            <Descriptions.Item label="待分析">
              <span data-testid="product-pending-analysis-count">{productStats.data?.pending_analysis_count ?? 0}</span>
            </Descriptions.Item>
          </Descriptions>
        ) : (
          <Typography.Text type="secondary">请选择一个产品查看统计信息。</Typography.Text>
        )}
      </Card>
      <Card title="卖点" extra={<Button disabled={!selectedProductID} onClick={() => setSellingPointOpen(true)}>新建卖点</Button>}>
        <Table<SellingPoint>
          rowKey="id"
          loading={sellingPoints.loading}
          dataSource={selectedProductID ? sellingPoints.data ?? [] : []}
          onRow={(record) => ({ onClick: () => setSelectedSellingPointID(record.id) })}
          rowClassName={(record) => (record.id === selectedSellingPointID ? "selected-row" : "")}
          columns={[
            { title: "标题", dataIndex: "title" },
            { title: "优先级", dataIndex: "priority" },
            { title: "关联素材", dataIndex: "asset_count", render: (value) => value ?? 0 },
            { title: "状态", dataIndex: "status", render: (status) => <Tag>{translateValue(status, productStatusLabels)}</Tag> }
          ]}
        />
      </Card>
      <Card title="卖点关联素材">
        {selectedSellingPoint ? (
          <Table<Asset>
            rowKey="id"
            loading={sellingPointAssets.loading}
            dataSource={sellingPointAssets.data ?? []}
            pagination={false}
            columns={[
              { title: "卖点", render: () => selectedSellingPoint.title },
              { title: "素材", render: (_, asset) => assetDisplayTitle(asset) },
              { title: "类型", dataIndex: "source_type", render: (value) => translateValue(value, sourceTypeLabels) },
              { title: "状态", dataIndex: "status", render: (status) => <Tag>{translateValue(status, assetStatusLabels)}</Tag> },
              { title: "分析状态", dataIndex: "analysis_status", render: (status) => translateValue(status, analysisStatusLabels) }
            ]}
          />
        ) : (
          <Typography.Text type="secondary">请选择一个卖点查看关联素材。</Typography.Text>
        )}
      </Card>

      <Modal title="新建产品" open={productOpen} onOk={createProduct} onCancel={() => setProductOpen(false)} okText="确认" cancelText="取消">
        <Form form={productForm} layout="vertical">
          <Form.Item name="name" label="产品名称" rules={[{ required: true, message: "请输入产品名称" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="category" label="分类">
            <Input />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title="新建卖点" open={sellingPointOpen} onOk={createSellingPoint} onCancel={() => setSellingPointOpen(false)} okText="确认" cancelText="取消">
        <Form form={sellingPointForm} layout="vertical">
          <Form.Item name="title" label="标题" rules={[{ required: true, message: "请输入卖点标题" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="priority" label="优先级" initialValue={0}>
            <InputNumber min={0} />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>
    </Space>
  );
}

function ProductsPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token);
  const [selectedProductID, setSelectedProductID] = useState<string | null>(null);
  const [selectedSellingPointID, setSelectedSellingPointID] = useState<string | null>(null);
  const [productStatsMap, setProductStatsMap] = useState<Record<string, ProductStats>>({});
  const [productStatsLoading, setProductStatsLoading] = useState(false);
  const sellingPoints = useResource<SellingPoint[]>(
    selectedProductID ? `/api/products/${selectedProductID}/selling-points` : null,
    token,
    [selectedProductID]
  );
  const sellingPointAssets = useResource<Asset[]>(
    selectedSellingPointID ? `/api/selling-points/${selectedSellingPointID}/assets` : null,
    token,
    [selectedSellingPointID]
  );
  const [productOpen, setProductOpen] = useState(false);
  const [sellingPointOpen, setSellingPointOpen] = useState(false);
  const [sellingPointManagerOpen, setSellingPointManagerOpen] = useState(false);
  const [sellingPointAssetsOpen, setSellingPointAssetsOpen] = useState(false);
  const [editingProduct, setEditingProduct] = useState<Product | null>(null);
  const [editingSellingPoint, setEditingSellingPoint] = useState<SellingPoint | null>(null);
  const [productForm] = Form.useForm();
  const [sellingPointForm] = Form.useForm();

  useEffect(() => {
    const productList = products.data ?? [];
    if (productList.length === 0) {
      setProductStatsMap({});
      setProductStatsLoading(false);
      return;
    }

    let cancelled = false;
    setProductStatsLoading(true);

    void Promise.all(
      productList.map(async (product) => {
        const stats = await apiRequest<ProductStats>(`/api/products/${product.id}/stats`, {}, token);
        return [product.id, stats] as const;
      })
    )
      .then((entries) => {
        if (!cancelled) {
          setProductStatsMap(Object.fromEntries(entries));
        }
      })
      .catch((error) => {
        if (!cancelled) {
          message.error(error instanceof Error ? error.message : "加载产品统计失败");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setProductStatsLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [products.data, token]);

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
      message.warning("请先选择产品");
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

  const selectedProduct = (products.data ?? []).find((product) => product.id === selectedProductID) ?? null;
  const selectedSellingPoint = (sellingPoints.data ?? []).find((item) => item.id === selectedSellingPointID) ?? null;
  const productRows = (products.data ?? []).map((product) => ({
    ...product,
    stats: productStatsMap[product.id]
  }));

  const openSellingPointManager = (productID: string) => {
    setSelectedProductID(productID);
    setSelectedSellingPointID(null);
    setSellingPointManagerOpen(true);
  };

  const openSellingPointAssets = (sellingPointID: string) => {
    setSelectedSellingPointID(sellingPointID);
    setSellingPointAssetsOpen(true);
  };

  const openCreateProduct = () => {
    setEditingProduct(null);
    productForm.resetFields();
    setProductOpen(true);
  };

  const openEditProduct = (product: Product) => {
    setEditingProduct(product);
    productForm.setFieldsValue({
      name: product.name,
      category: product.category,
      description: product.description
    });
    setProductOpen(true);
  };

  const deleteProduct = async (product: Product) => {
    const assetCount = productStatsMap[product.id]?.asset_count ?? 0;
    if (assetCount > 0) {
      message.warning("该产品已关联素材，不允许删除");
      return;
    }
    await apiRequest(`/api/products/${product.id}`, { method: "DELETE" }, token);
    if (selectedProductID === product.id) {
      setSelectedProductID(null);
      setSelectedSellingPointID(null);
    }
    await products.reload();
    message.success("产品已删除");
  };

  const openCreateSellingPoint = () => {
    setEditingSellingPoint(null);
    sellingPointForm.resetFields();
    setSellingPointOpen(true);
  };

  const openEditSellingPoint = (sellingPoint: SellingPoint) => {
    setEditingSellingPoint(sellingPoint);
    sellingPointForm.setFieldsValue({
      title: sellingPoint.title,
      priority: sellingPoint.priority,
      description: sellingPoint.description
    });
    setSellingPointOpen(true);
  };

  const deleteSellingPoint = async (sellingPoint: SellingPoint) => {
    if ((sellingPoint.asset_count ?? 0) > 0) {
      message.warning("该卖点已关联素材，不允许删除");
      return;
    }
    await apiRequest(`/api/selling-points/${sellingPoint.id}`, { method: "DELETE" }, token);
    if (selectedSellingPointID === sellingPoint.id) {
      setSelectedSellingPointID(null);
    }
    await sellingPoints.reload();
    message.success("卖点已删除");
  };

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>产品</Typography.Title>
      <Card title="产品列表" extra={<Button type="primary" onClick={() => setProductOpen(true)}>新建产品</Button>}>
        <Table<(Product & { stats?: ProductStats })>
          rowKey="id"
          loading={products.loading || productStatsLoading}
          dataSource={productRows}
          columns={[
            { title: "产品", dataIndex: "name", width: 220 },
            { title: "分类", dataIndex: "category", render: (value) => value || "-" },
            { title: "状态", dataIndex: "status", width: 120, render: (status) => <Tag>{translateValue(status, productStatusLabels)}</Tag> },
            {
              title: "素材数",
              width: 100,
              render: (_, record) => <span data-testid={record.id === selectedProductID ? "product-asset-count" : undefined}>{record.stats?.asset_count ?? "-"}</span>
            },
            {
              title: "可用素材",
              width: 120,
              render: (_, record) => <span data-testid={record.id === selectedProductID ? "product-usable-asset-count" : undefined}>{record.stats?.usable_asset_count ?? "-"}</span>
            },
            {
              title: "待分析",
              width: 100,
              render: (_, record) => <span data-testid={record.id === selectedProductID ? "product-pending-analysis-count" : undefined}>{record.stats?.pending_analysis_count ?? "-"}</span>
            },
            {
              title: "操作",
              key: "actions",
              width: 220,
              render: (_, record) => (
                <Space size="small" wrap>
                  <Button type="link" className="table-link-button" onClick={() => openSellingPointManager(record.id)}>
                    卖点管理
                  </Button>
                </Space>
              )
            }
          ]}
        />
      </Card>

      <Modal title="新建产品" open={productOpen} onOk={createProduct} onCancel={() => setProductOpen(false)} okText="确认" cancelText="取消">
        <Form form={productForm} layout="vertical">
          <Form.Item name="name" label="产品名称" rules={[{ required: true, message: "请输入产品名称" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="category" label="分类">
            <Input />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={selectedProduct ? `卖点管理：${selectedProduct.name}` : "卖点管理"}
        open={sellingPointManagerOpen}
        onCancel={() => {
          setSellingPointManagerOpen(false);
          setSelectedSellingPointID(null);
        }}
        footer={null}
        width={960}
      >
        <Space direction="vertical" size="middle" className="wide-space">
          <Descriptions
            bordered
            size="small"
            column={3}
            items={[
              { key: "name", label: "产品", children: selectedProduct?.name ?? "-" },
              { key: "category", label: "分类", children: selectedProduct?.category || "-" },
              { key: "status", label: "状态", children: selectedProduct ? translateValue(selectedProduct.status, productStatusLabels) : "-" }
            ]}
          />

          <div className="products-modal-toolbar">
            <Typography.Text type="secondary">在这里管理当前产品的卖点，并按卖点查看关联素材。</Typography.Text>
            <Button
              type="primary"
              disabled={!selectedProductID}
              onClick={() => {
                sellingPointForm.resetFields();
                setSellingPointOpen(true);
              }}
            >
              新建卖点
            </Button>
          </div>

          <Table<SellingPoint>
            rowKey="id"
            loading={sellingPoints.loading}
            dataSource={selectedProductID ? sellingPoints.data ?? [] : []}
            pagination={false}
            locale={{ emptyText: "当前产品还没有卖点" }}
            columns={[
              { title: "标题", dataIndex: "title" },
              { title: "优先级", dataIndex: "priority", width: 100 },
              { title: "关联素材数", dataIndex: "asset_count", width: 120, render: (value) => value ?? 0 },
              { title: "状态", dataIndex: "status", width: 120, render: (status) => <Tag>{translateValue(status, productStatusLabels)}</Tag> },
              {
                title: "操作",
                key: "actions",
                width: 160,
                render: (_, record) => (
                  <Button type="link" className="table-link-button" onClick={() => openSellingPointAssets(record.id)}>
                    查看关联素材
                  </Button>
                )
              }
            ]}
          />
        </Space>
      </Modal>

      <Modal
        title={selectedProduct ? `新建卖点：${selectedProduct.name}` : "新建卖点"}
        open={sellingPointOpen}
        onOk={createSellingPoint}
        onCancel={() => setSellingPointOpen(false)}
        okText="确认"
        cancelText="取消"
      >
        <Form form={sellingPointForm} layout="vertical">
          <Form.Item name="title" label="标题" rules={[{ required: true, message: "请输入卖点标题" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="priority" label="优先级" initialValue={0}>
            <InputNumber min={0} />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title={selectedSellingPoint ? `关联素材：${selectedSellingPoint.title}` : "关联素材"}
        open={sellingPointAssetsOpen}
        onClose={() => setSellingPointAssetsOpen(false)}
        width={820}
      >
        <Table<Asset>
          rowKey="id"
          loading={sellingPointAssets.loading}
          dataSource={sellingPointAssets.data ?? []}
          pagination={false}
          locale={{ emptyText: "当前卖点还没有关联素材" }}
          columns={[
            { title: "素材", render: (_, asset) => assetDisplayTitle(asset) },
            { title: "类型", dataIndex: "source_type", width: 120, render: (value) => translateValue(value, sourceTypeLabels) },
            { title: "状态", dataIndex: "status", width: 120, render: (status) => <Tag>{translateValue(status, assetStatusLabels)}</Tag> },
            { title: "分析状态", dataIndex: "analysis_status", width: 120, render: (status) => translateValue(status, analysisStatusLabels) }
          ]}
        />
      </Drawer>
    </Space>
  );
}

function ProductManagementPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token);
  const [selectedProductID, setSelectedProductID] = useState<string | null>(null);
  const [selectedSellingPointID, setSelectedSellingPointID] = useState<string | null>(null);
  const [productStatsMap, setProductStatsMap] = useState<Record<string, ProductStats>>({});
  const [productStatsLoading, setProductStatsLoading] = useState(false);
  const [productOpen, setProductOpen] = useState(false);
  const [sellingPointOpen, setSellingPointOpen] = useState(false);
  const [sellingPointManagerOpen, setSellingPointManagerOpen] = useState(false);
  const [sellingPointAssetsOpen, setSellingPointAssetsOpen] = useState(false);
  const [editingProduct, setEditingProduct] = useState<Product | null>(null);
  const [editingSellingPoint, setEditingSellingPoint] = useState<SellingPoint | null>(null);
  const [productForm] = Form.useForm();
  const [sellingPointForm] = Form.useForm();

  const sellingPoints = useResource<SellingPoint[]>(
    selectedProductID ? `/api/products/${selectedProductID}/selling-points` : null,
    token,
    [selectedProductID]
  );
  const sellingPointAssets = useResource<Asset[]>(
    selectedSellingPointID ? `/api/selling-points/${selectedSellingPointID}/assets` : null,
    token,
    [selectedSellingPointID]
  );

  useEffect(() => {
    const productList = products.data ?? [];
    if (productList.length === 0) {
      setProductStatsMap({});
      setProductStatsLoading(false);
      return;
    }

    let cancelled = false;
    setProductStatsLoading(true);
    void Promise.all(
      productList.map(async (product) => {
        const stats = await apiRequest<ProductStats>(`/api/products/${product.id}/stats`, {}, token);
        return [product.id, stats] as const;
      })
    )
      .then((entries) => {
        if (!cancelled) {
          setProductStatsMap(Object.fromEntries(entries));
        }
      })
      .catch((error) => {
        if (!cancelled) {
          message.error(error instanceof Error ? error.message : "加载产品统计失败");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setProductStatsLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [products.data, token]);

  const selectedProduct = (products.data ?? []).find((product) => product.id === selectedProductID) ?? null;
  const selectedSellingPoint = (sellingPoints.data ?? []).find((item) => item.id === selectedSellingPointID) ?? null;
  const productRows = (products.data ?? []).map((product) => ({ ...product, stats: productStatsMap[product.id] }));

  const openCreateProduct = () => {
    setEditingProduct(null);
    productForm.resetFields();
    setProductOpen(true);
  };

  const openEditProduct = (product: Product) => {
    setEditingProduct(product);
    productForm.setFieldsValue({
      name: product.name,
      category: product.category,
      description: product.description
    });
    setProductOpen(true);
  };

  const saveProduct = async () => {
    const values = await productForm.validateFields();
    await apiRequest<Product>(
      editingProduct ? `/api/products/${editingProduct.id}` : "/api/products",
      {
        method: editingProduct ? "PUT" : "POST",
        body: JSON.stringify({ ...values, metadata: {} })
      },
      token
    );
    setProductOpen(false);
    setEditingProduct(null);
    productForm.resetFields();
    await products.reload();
    message.success(editingProduct ? "产品已更新" : "产品已创建");
  };

  const deleteProduct = async (product: Product) => {
    if ((productStatsMap[product.id]?.asset_count ?? 0) > 0) {
      message.warning("该产品已关联素材，不允许删除");
      return;
    }
    await apiRequest(`/api/products/${product.id}`, { method: "DELETE" }, token);
    if (selectedProductID === product.id) {
      setSelectedProductID(null);
      setSelectedSellingPointID(null);
    }
    await products.reload();
    message.success("产品已删除");
  };

  const openSellingPointManager = (productID: string) => {
    setSelectedProductID(productID);
    setSelectedSellingPointID(null);
    setSellingPointManagerOpen(true);
  };

  const openCreateSellingPoint = () => {
    setEditingSellingPoint(null);
    sellingPointForm.resetFields();
    setSellingPointOpen(true);
  };

  const openEditSellingPoint = (sellingPoint: SellingPoint) => {
    setEditingSellingPoint(sellingPoint);
    sellingPointForm.setFieldsValue({
      title: sellingPoint.title,
      priority: sellingPoint.priority,
      description: sellingPoint.description
    });
    setSellingPointOpen(true);
  };

  const saveSellingPoint = async () => {
    if (!selectedProductID) {
      message.warning("请先选择产品");
      return;
    }
    const values = await sellingPointForm.validateFields();
    await apiRequest<SellingPoint>(
      editingSellingPoint ? `/api/selling-points/${editingSellingPoint.id}` : `/api/products/${selectedProductID}/selling-points`,
      {
        method: editingSellingPoint ? "PUT" : "POST",
        body: JSON.stringify(values)
      },
      token
    );
    setSellingPointOpen(false);
    setEditingSellingPoint(null);
    sellingPointForm.resetFields();
    await sellingPoints.reload();
    message.success(editingSellingPoint ? "卖点已更新" : "卖点已创建");
  };

  const deleteSellingPoint = async (sellingPoint: SellingPoint) => {
    if ((sellingPoint.asset_count ?? 0) > 0) {
      message.warning("该卖点已关联素材，不允许删除");
      return;
    }
    await apiRequest(`/api/selling-points/${sellingPoint.id}`, { method: "DELETE" }, token);
    if (selectedSellingPointID === sellingPoint.id) {
      setSelectedSellingPointID(null);
    }
    await sellingPoints.reload();
    message.success("卖点已删除");
  };

  const openSellingPointAssets = (sellingPointID: string) => {
    setSelectedSellingPointID(sellingPointID);
    setSellingPointAssetsOpen(true);
  };

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>产品</Typography.Title>
      <Card title="产品列表" extra={<Button type="primary" onClick={openCreateProduct}>新建产品</Button>}>
        <Table<(Product & { stats?: ProductStats })>
          rowKey="id"
          loading={products.loading || productStatsLoading}
          dataSource={productRows}
          columns={[
            { title: "产品", dataIndex: "name", width: 220 },
            { title: "分类", dataIndex: "category", render: (value) => value || "-" },
            { title: "状态", dataIndex: "status", width: 100, render: (status) => <Tag>{translateValue(status, productStatusLabels)}</Tag> },
            { title: "素材", width: 90, render: (_, record) => record.stats?.asset_count ?? "-" },
            { title: "可用素材", width: 100, render: (_, record) => record.stats?.usable_asset_count ?? "-" },
            { title: "待分析", width: 90, render: (_, record) => record.stats?.pending_analysis_count ?? "-" },
            {
              title: "操作",
              key: "actions",
              width: 300,
              render: (_, record) => (
                <Space size="small" wrap>
                  <Button type="link" className="table-link-button" onClick={() => openSellingPointManager(record.id)}>
                    卖点
                  </Button>
                  <Button type="link" className="table-link-button" onClick={() => openEditProduct(record)}>
                    编辑
                  </Button>
                  <Popconfirm
                    title="删除产品"
                    description="没有素材关联的产品会被直接删除。确认删除？"
                    okText="删除"
                    cancelText="取消"
                    onConfirm={() => void deleteProduct(record)}
                  >
                    <Button type="link" danger className="table-link-button">
                      删除
                    </Button>
                  </Popconfirm>
                </Space>
              )
            }
          ]}
        />
      </Card>

      <Modal
        title={editingProduct ? "编辑产品" : "新建产品"}
        open={productOpen}
        onOk={saveProduct}
        onCancel={() => {
          setProductOpen(false);
          setEditingProduct(null);
          productForm.resetFields();
        }}
        okText="确认"
        cancelText="取消"
      >
        <Form form={productForm} layout="vertical">
          <Form.Item name="name" label="产品名称" rules={[{ required: true, message: "请输入产品名称" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="category" label="分类">
            <Input />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={selectedProduct ? `卖点管理：${selectedProduct.name}` : "卖点管理"}
        open={sellingPointManagerOpen}
        onCancel={() => {
          setSellingPointManagerOpen(false);
          setSelectedSellingPointID(null);
        }}
        footer={null}
        width={980}
      >
        <Space direction="vertical" size="middle" className="wide-space">
          <div className="products-modal-toolbar">
            <Typography.Text type="secondary">管理当前产品卖点，查看每个卖点关联的素材画面。</Typography.Text>
            <Button type="primary" disabled={!selectedProductID} onClick={openCreateSellingPoint}>
              新建卖点
            </Button>
          </div>
          <Table<SellingPoint>
            rowKey="id"
            loading={sellingPoints.loading}
            dataSource={selectedProductID ? sellingPoints.data ?? [] : []}
            pagination={false}
            locale={{ emptyText: "当前产品还没有卖点" }}
            columns={[
              { title: "标题", dataIndex: "title" },
              { title: "优先级", dataIndex: "priority", width: 100 },
              { title: "关联素材", dataIndex: "asset_count", width: 110, render: (value) => value ?? 0 },
              { title: "状态", dataIndex: "status", width: 100, render: (status) => <Tag>{translateValue(status, productStatusLabels)}</Tag> },
              {
                title: "操作",
                key: "actions",
                width: 260,
                render: (_, record) => (
                  <Space size="small" wrap>
                    <Button type="link" className="table-link-button" onClick={() => openSellingPointAssets(record.id)}>
                      关联素材
                    </Button>
                    <Button type="link" className="table-link-button" onClick={() => openEditSellingPoint(record)}>
                      编辑
                    </Button>
                    <Popconfirm
                      title="删除卖点"
                      description="没有素材关联的卖点会被直接删除。确认删除？"
                      okText="删除"
                      cancelText="取消"
                      onConfirm={() => void deleteSellingPoint(record)}
                    >
                      <Button type="link" danger className="table-link-button">
                        删除
                      </Button>
                    </Popconfirm>
                  </Space>
                )
              }
            ]}
          />
        </Space>
      </Modal>

      <Modal
        title={editingSellingPoint ? "编辑卖点" : selectedProduct ? `新建卖点：${selectedProduct.name}` : "新建卖点"}
        open={sellingPointOpen}
        onOk={saveSellingPoint}
        onCancel={() => {
          setSellingPointOpen(false);
          setEditingSellingPoint(null);
          sellingPointForm.resetFields();
        }}
        okText="确认"
        cancelText="取消"
      >
        <Form form={sellingPointForm} layout="vertical">
          <Form.Item name="title" label="标题" rules={[{ required: true, message: "请输入卖点标题" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="priority" label="优先级" initialValue={0}>
            <InputNumber min={0} />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title={selectedSellingPoint ? `关联素材：${selectedSellingPoint.title}` : "关联素材"}
        open={sellingPointAssetsOpen}
        onClose={() => setSellingPointAssetsOpen(false)}
        width={980}
      >
        {sellingPointAssets.loading ? (
          <Card loading />
        ) : (sellingPointAssets.data ?? []).length === 0 ? (
          <Empty description="当前卖点还没有关联素材" />
        ) : (
          <div className="product-linked-asset-grid">
            {(sellingPointAssets.data ?? []).map((asset) => (
              <Card key={asset.id} className="product-linked-asset-card" bodyStyle={{ padding: 12 }}>
                <video className="product-linked-asset-video" src={assetVideoURL(asset)} muted preload="metadata" controls />
                <Typography.Text className="product-linked-asset-title" title={assetDisplayTitle(asset)}>
                  {assetDisplayTitle(asset)}
                </Typography.Text>
                <Space size={[4, 4]} wrap>
                  <Tag>{translateValue(asset.source_type, sourceTypeLabels)}</Tag>
                  <Tag>{formatDuration(asset.duration_ms)}</Tag>
                  {asset.width && asset.height ? <Tag>{asset.width}x{asset.height}</Tag> : null}
                  {asset.shot_size ? <Tag>{translateValue(asset.shot_size, shotSizeLabels)}</Tag> : null}
                  {asset.camera_movement ? <Tag>{translateValue(asset.camera_movement, cameraMovementLabels)}</Tag> : null}
                </Space>
                <Typography.Paragraph className="product-linked-asset-desc" ellipsis={{ rows: 2 }}>
                  {asset.scene_description || "暂无画面描述"}
                </Typography.Paragraph>
              </Card>
            ))}
          </div>
        )}
      </Drawer>
    </Space>
  );
}

function AssetsPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token);
  const [productForSellingPoints, setProductForSellingPoints] = useState<string>("");
  const [selectedAsset, setSelectedAsset] = useState<Asset | null>(null);
  const sellingPoints = useResource<SellingPoint[]>(
    productForSellingPoints ? `/api/products/${productForSellingPoints}/selling-points` : null,
    token,
    [productForSellingPoints]
  );
  const assetDetailSellingPoints = useResource<SellingPoint[]>(
    selectedAsset ? `/api/products/${selectedAsset.product_id}/selling-points` : null,
    token,
    [selectedAsset?.product_id]
  );
  const [filters, setFilters] = useState({
    productID: "",
    sellingPointID: "",
    sourceType: "",
    status: "",
    usabilityStatus: "",
    shotSize: "",
    tag: "",
    keyword: "",
    minDurationMs: "",
    maxDurationMs: "",
    hasAudio: "",
    likelyHasSpeech: "",
    excludeDiscarded: "",
    sortBy: ""
  });
  const [assetFiltersExpanded, setAssetFiltersExpanded] = useState(false);
  const [assetPage, setAssetPage] = useState(1);
  const [assetPageSize, setAssetPageSize] = useState(20);
  const [frames, setFrames] = useState<AssetFrameSnapshot[]>([]);
  const [speechSegments, setSpeechSegments] = useState<AssetSpeechSegment[]>([]);
  const [semanticPreview, setSemanticPreview] = useState<AssetSemanticPreview | null>(null);
  const [assetSellingPoints, setAssetSellingPoints] = useState<SellingPoint[]>([]);
  const [framesLoading, setFramesLoading] = useState(false);
  const [speechSegmentsLoading, setSpeechSegmentsLoading] = useState(false);
  const [semanticPreviewLoading, setSemanticPreviewLoading] = useState(false);
  const [editingAnalysis, setEditingAnalysis] = useState(false);
  const [savingAnalysis, setSavingAnalysis] = useState(false);
  const [updatingArchive, setUpdatingArchive] = useState(false);
  const [savingSellingPoints, setSavingSellingPoints] = useState(false);
  const [savingBusinessTags, setSavingBusinessTags] = useState(false);
  const [reviewForm] = Form.useForm<AssetReviewPayload>();
  const [sellingPointForm] = Form.useForm<AssetSellingPointPayload>();
  const [businessTagForm] = Form.useForm<AssetBusinessTagPayload>();

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
    if (filters.usabilityStatus) {
      params.set("usability_status", filters.usabilityStatus);
    }
    if (filters.shotSize) {
      params.set("shot_size", filters.shotSize);
    }
    if (filters.tag) {
      params.set("tag", filters.tag);
    }
    if (filters.keyword) {
      params.set("keyword", filters.keyword);
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
    if (filters.likelyHasSpeech) {
      params.set("likely_has_speech", filters.likelyHasSpeech);
    }
    if (filters.excludeDiscarded) {
      params.set("exclude_discarded", filters.excludeDiscarded);
    }
    if (filters.sortBy) {
      params.set("sort_by", filters.sortBy);
    }
    params.set("page", String(assetPage));
    params.set("page_size", String(assetPageSize));
    const query = params.toString();
    return query ? `/api/assets?${query}` : "/api/assets";
  }, [assetPage, assetPageSize, filters]);

  const assets = useResource<AssetListResponse>(assetPath, token, [assetPath]);
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
      setSpeechSegments([]);
      setSemanticPreview(null);
      setAssetSellingPoints([]);
      return;
    }

    const loadFrames = async () => {
      setFramesLoading(true);
      try {
        const response = await apiRequest<AssetFrameResponse>(`/api/assets/${selectedAsset.id}/frames`, {}, token);
        setFrames(response.frames);
      } catch (error) {
        setFrames([]);
        message.error(error instanceof Error ? error.message : "加载抽帧预览失败");
      } finally {
        setFramesLoading(false);
      }
    };

    void loadFrames();

    const loadSemanticPreview = async () => {
      setSemanticPreviewLoading(true);
      try {
        const response = await apiRequest<AssetSemanticPreview>(`/api/assets/${selectedAsset.id}/semantic-preview`, {}, token);
        setSemanticPreview(response);
      } catch (error) {
        setSemanticPreview(null);
        message.error(error instanceof Error ? error.message : "加载开放语义描述失败");
      } finally {
        setSemanticPreviewLoading(false);
      }
    };

    void loadSemanticPreview();

    const loadSpeechSegments = async () => {
      if (selectedAsset.source_type !== "talking_head") {
        setSpeechSegments([]);
        setSpeechSegmentsLoading(false);
        return;
      }

      setSpeechSegmentsLoading(true);
      try {
        const response = await apiRequest<AssetSpeechSegment[]>(`/api/assets/${selectedAsset.id}/speech-segments`, {}, token);
        setSpeechSegments(response);
      } catch (error) {
        setSpeechSegments([]);
        message.error(error instanceof Error ? error.message : "加载口播句段失败");
      } finally {
        setSpeechSegmentsLoading(false);
      }
    };

    void loadSpeechSegments();

    const loadAssetSellingPoints = async () => {
      try {
        const response = await apiRequest<SellingPoint[]>(`/api/assets/${selectedAsset.id}/selling-points`, {}, token);
        setAssetSellingPoints(response);
        sellingPointForm.setFieldsValue({
          selling_point_ids: response.map((item) => item.id)
        });
      } catch (error) {
        setAssetSellingPoints([]);
        message.error(error instanceof Error ? error.message : "加载素材卖点失败");
      }
    };

    void loadAssetSellingPoints();
  }, [selectedAsset, token]);

  useEffect(() => {
    if (!selectedAsset) {
      reviewForm.resetFields();
      sellingPointForm.resetFields();
      businessTagForm.resetFields();
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
    businessTagForm.setFieldsValue({
      is_curated: Boolean(selectedAsset.metadata?.is_curated),
      business_tags: Array.isArray(selectedAsset.metadata?.business_tags)
        ? (selectedAsset.metadata?.business_tags as string[])
        : [],
      narrative_roles: Array.isArray(selectedAsset.metadata?.narrative_roles)
        ? (selectedAsset.metadata?.narrative_roles as string[])
        : [],
      usage_notes: typeof selectedAsset.metadata?.usage_notes === "string" ? (selectedAsset.metadata?.usage_notes as string) : ""
    });
  }, [businessTagForm, reviewForm, selectedAsset, sellingPointForm]);

  useEffect(() => {
    sellingPointForm.setFieldsValue({
      selling_point_ids: assetSellingPoints.map((item) => item.id)
    });
  }, [assetSellingPoints, sellingPointForm]);

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
      message.success("素材复核已更新");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "更新素材复核失败");
    } finally {
      setSavingAnalysis(false);
    }
  };

  const updateAssetArchiveState = async (asset: Asset, action: "archive" | "restore") => {
    setUpdatingArchive(true);
    try {
      const updated = await apiRequest<Asset>(
        `/api/assets/${asset.id}/${action}`,
        { method: "POST" },
        token
      );
      setSelectedAsset(updated);
      await assets.reload();
      message.success(action === "archive" ? "素材已归档" : "素材已恢复");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "更新素材状态失败");
    } finally {
      setUpdatingArchive(false);
    }
  };

  const saveAssetSellingPoints = async () => {
    if (!selectedAsset) {
      return;
    }

    const values = await sellingPointForm.validateFields();
    setSavingSellingPoints(true);
    try {
      const updated = await apiRequest<SellingPoint[]>(
        `/api/assets/${selectedAsset.id}/selling-points`,
        {
          method: "PUT",
          body: JSON.stringify(values)
        },
        token
      );
      setAssetSellingPoints(updated);
      message.success("素材卖点关联已更新");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "更新素材卖点失败");
    } finally {
      setSavingSellingPoints(false);
    }
  };

  const saveAssetBusinessTags = async () => {
    if (!selectedAsset) {
      return;
    }

    const values = await businessTagForm.validateFields();
    setSavingBusinessTags(true);
    try {
      const updated = await apiRequest<Asset>(
        `/api/assets/${selectedAsset.id}/business-tags`,
        {
          method: "PUT",
          body: JSON.stringify(values)
        },
        token
      );
      setSelectedAsset(updated);
      await assets.reload();
      message.success("精选业务标签已更新");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "更新精选业务标签失败");
    } finally {
      setSavingBusinessTags(false);
    }
  };

  const assetItems = assets.data?.items ?? [];
  const assetTotal = assets.data?.total ?? 0;
  const activeFilterCount = Object.values(filters).filter(Boolean).length;

  return (
    <div data-testid="assets-page" className="asset-library-page">
      <Space direction="vertical" size="middle" className="page-stack asset-library-stack">
        <Card className="asset-filter-card" bodyStyle={{ padding: 12 }}>
          <div className="asset-filter-toolbar">
            <Input
              data-testid="asset-filter-keyword"
              value={filters.keyword}
              allowClear
              placeholder="搜索画面描述、文件名或标签"
              className="asset-filter-search"
              onChange={(event) => {
                setAssetPage(1);
                setFilters((current) => ({ ...current, keyword: event.target.value }));
              }}
            />
            <Select
              data-testid="asset-filter-product"
              value={filters.productID || undefined}
              placeholder="产品"
              allowClear
              className="asset-filter-select"
              options={(products.data ?? []).map((product) => ({ value: product.id, label: product.name }))}
              onChange={(value) => {
                const nextValue = value ?? "";
                setAssetPage(1);
                setFilters((current) => ({ ...current, productID: nextValue, sellingPointID: "" }));
                setProductForSellingPoints(nextValue);
              }}
            />
            <Button onClick={() => setAssetFiltersExpanded((current) => !current)}>
              {assetFiltersExpanded ? "收起筛选" : `更多筛选${activeFilterCount > 0 ? `(${activeFilterCount})` : ""}`}
            </Button>
            <Button
              data-testid="asset-filter-reset"
              onClick={() => {
                setAssetPage(1);
                setAssetPageSize(20);
                setFilters({
                  productID: "",
                  sellingPointID: "",
                  sourceType: "",
                  status: "",
                  usabilityStatus: "",
                  shotSize: "",
                  tag: "",
                  keyword: "",
                  minDurationMs: "",
                  maxDurationMs: "",
                  hasAudio: "",
                  likelyHasSpeech: "",
                  excludeDiscarded: "",
                  sortBy: ""
                });
                setProductForSellingPoints("");
              }}
            >
              重置
            </Button>
            <Button data-testid="asset-filter-refresh" onClick={assets.reload} loading={assets.loading}>刷新</Button>
          </div>
          {assetFiltersExpanded ? (
            <div className="asset-filter-advanced">
              <Select
                data-testid="asset-filter-selling-point"
                value={filters.sellingPointID || undefined}
                placeholder="卖点"
                allowClear
                className="asset-filter-select"
                disabled={!filters.productID}
                options={(sellingPoints.data ?? []).map((item) => ({ value: item.id, label: item.title }))}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, sellingPointID: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-source-type"
                value={filters.sourceType || undefined}
                placeholder="素材类型"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "visual_only", label: "纯画面" },
                  { value: "talking_head", label: "口播" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, sourceType: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-status"
                value={filters.status || undefined}
                placeholder="状态"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "ready", label: "已完成" },
                  { value: "failed", label: "失败" },
                  { value: "uploaded", label: "已上传" },
                  { value: "archived", label: "已归档" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, status: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-usability-status"
                value={filters.usabilityStatus || undefined}
                placeholder="可用状态"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "usable", label: "可用" },
                  { value: "needs_review", label: "待复核" },
                  { value: "discarded", label: "废弃" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, usabilityStatus: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-shot-size"
                value={filters.shotSize || undefined}
                placeholder="景别"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "close_up", label: "特写" },
                  { value: "medium_close_up", label: "近景" },
                  { value: "medium_shot", label: "中景" },
                  { value: "wide_shot", label: "远景" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, shotSize: value ?? "" }));
                }}
              />
              <Input
                data-testid="asset-filter-tag"
                value={filters.tag}
                placeholder="标签"
                className="asset-filter-compact"
                onChange={(event) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, tag: event.target.value }));
                }}
              />
              <Input
                data-testid="asset-filter-min-duration"
                value={filters.minDurationMs}
                placeholder="最小时长 ms"
                className="asset-filter-duration"
                onChange={(event) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, minDurationMs: event.target.value }));
                }}
              />
              <Input
                data-testid="asset-filter-max-duration"
                value={filters.maxDurationMs}
                placeholder="最大时长 ms"
                className="asset-filter-duration"
                onChange={(event) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, maxDurationMs: event.target.value }));
                }}
              />
              <Select
                data-testid="asset-filter-has-audio"
                value={filters.hasAudio || undefined}
                placeholder="音频"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "true", label: "包含音频" },
                  { value: "false", label: "无音频" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, hasAudio: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-likely-has-speech"
                value={filters.likelyHasSpeech || undefined}
                placeholder="人声"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "true", label: "有人声" },
                  { value: "false", label: "无人声" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, likelyHasSpeech: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-exclude-discarded"
                value={filters.excludeDiscarded || undefined}
                placeholder="可用性过滤"
                allowClear
                className="asset-filter-select"
                options={[{ value: "true", label: "排除废弃" }]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, excludeDiscarded: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-sort"
                value={filters.sortBy || undefined}
                placeholder="排序"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "updated_at_desc", label: "最近更新时间" },
                  { value: "analyzed_at_desc", label: "最近分析时间" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, sortBy: value ?? "" }));
                }}
              />
            </div>
          ) : null}
        </Card>

        <Card
          className="asset-grid-card"
          title="素材列表"
          extra={
            <Typography.Text type="secondary">
              第 {assets.data?.page ?? assetPage} 页 / 共 {assetTotal} 条
            </Typography.Text>
          }
          loading={assets.loading}
        >
          {assetItems.length === 0 ? (
            <Empty description="暂无匹配素材" />
          ) : (
            <div className="asset-card-grid">
              {assetItems.map((asset) => {
                const title = assetDisplayTitle(asset);
                const fileName = assetFileDisplayName(asset);
                const tags = [...(asset.scene_tags ?? []), ...(asset.subjects ?? [])].slice(0, 4);
                return (
                  <button
                    key={asset.id}
                    type="button"
                    className="asset-library-card"
                    onClick={() => setSelectedAsset(asset)}
                    aria-label={title}
                  >
                    <div className="asset-card-media">
                      <video src={assetVideoURL(asset)} muted preload="metadata" />
                      <div className="asset-card-scrim" />
                      <Tag className="asset-card-status">{translateValue(asset.status, assetStatusLabels)}</Tag>
                      <Tag className="asset-card-type">{translateValue(asset.source_type, sourceTypeLabels)}</Tag>
                      <span className="asset-card-duration">{formatDuration(asset.duration_ms)}</span>
                    </div>
                    <div className="asset-card-body">
                      <Typography.Text className="asset-card-title">{title}</Typography.Text>
                      <Typography.Text className="asset-card-file">{fileName}</Typography.Text>
                      <div className="asset-card-meta">
                        <span>{productNameByID.get(asset.product_id) ?? asset.product_id ?? "-"}</span>
                        <span>{asset.width && asset.height ? asset.width + "x" + asset.height : "未知分辨率"}</span>
                        <span>{asset.fps ? asset.fps.toFixed(0) + "fps" : "未知帧率"}</span>
                      </div>
                      <div className="asset-card-labels">
                        {asset.shot_size ? <Tag>{translateValue(asset.shot_size, shotSizeLabels)}</Tag> : null}
                        {asset.camera_movement ? <Tag>{translateValue(asset.camera_movement, cameraMovementLabels)}</Tag> : null}
                        {asset.analysis_status ? <Tag color="blue">{translateValue(asset.analysis_status, analysisStatusLabels)}</Tag> : null}
                      </div>
                      <Typography.Text className="asset-card-description">
                        {asset.scene_description || "暂无画面描述"}
                      </Typography.Text>
                      <div className="asset-card-tags">
                        {tags.length > 0 ? tags.map((tag) => <span key={tag}>{tag}</span>) : <span>暂无标签</span>}
                      </div>
                    </div>
                  </button>
                );
              })}
            </div>
          )}
          <div className="asset-pagination">
            <Pagination
              current={assets.data?.page ?? assetPage}
              pageSize={assets.data?.page_size ?? assetPageSize}
              total={assetTotal}
              showSizeChanger
              pageSizeOptions={["10", "20", "50"]}
              onChange={(page, pageSize) => {
                setAssetPage(page);
                setAssetPageSize(pageSize);
              }}
            />
          </div>
        </Card>
      </Space>

      <Modal
        title={selectedAsset ? `素材详情：${assetDisplayTitle(selectedAsset)}` : "素材详情"}
        open={selectedAsset !== null}
        footer={null}
        width="86vw"
        className="asset-detail-modal"
        onCancel={() => setSelectedAsset(null)}
      >
        {selectedAsset ? (
          <div className="asset-detail-shell" data-testid="asset-detail-modal">
            <div className="asset-detail-workspace">
              <section className="asset-detail-preview">
                <video src={assetVideoURL(selectedAsset)} controls preload="metadata" />
                <div className="asset-detail-title-block">
                  <div>
                    <Typography.Title level={4}>{assetDisplayTitle(selectedAsset)}</Typography.Title>
                    <Typography.Text type="secondary">{assetFileDisplayName(selectedAsset)}</Typography.Text>
                  </div>
                  <Space wrap>
                    <Tag>{translateValue(selectedAsset.source_type, sourceTypeLabels)}</Tag>
                    <Tag>{translateValue(selectedAsset.status, assetStatusLabels)}</Tag>
                    {selectedAsset.analysis_status ? <Tag color="blue">{translateValue(selectedAsset.analysis_status, analysisStatusLabels)}</Tag> : null}
                  </Space>
                </div>
                <div className="asset-detail-quick-meta">
                  <span>时长 {formatDuration(selectedAsset.duration_ms)}</span>
                  <span>{selectedAsset.width && selectedAsset.height ? `${selectedAsset.width}x${selectedAsset.height}` : "未知分辨率"}</span>
                  <span>{selectedAsset.fps ? `${selectedAsset.fps}fps` : "未知帧率"}</span>
                  <span>{selectedAsset.has_audio ? "含音频" : "无音频"}</span>
                  <span>{translateValue(selectedAsset.usability_status, usabilityStatusLabels)}</span>
                </div>
              </section>

              <section className="asset-detail-analysis-card">
                <div className="asset-detail-section-head">
                  <div>
                    <Typography.Title level={5}>分析与维护</Typography.Title>
                    <Typography.Text type="secondary">画面标签、可用性和人工复核</Typography.Text>
                  </div>
                  <Space wrap>
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
                      {editingAnalysis ? "取消编辑" : "编辑标签"}
                    </Button>
                    {editingAnalysis ? (
                      <Button type="primary" size="small" loading={savingAnalysis} onClick={saveAnalysisReview} data-testid="save-asset-review">
                        保存复核
                      </Button>
                    ) : null}
                    {selectedAsset.status === "archived" ? (
                      <Button
                        size="small"
                        loading={updatingArchive}
                        onClick={() => void updateAssetArchiveState(selectedAsset, "restore")}
                        data-testid="restore-asset"
                      >
                        恢复素材
                      </Button>
                    ) : (
                      <Button
                        size="small"
                        danger
                        loading={updatingArchive}
                        onClick={() => void updateAssetArchiveState(selectedAsset, "archive")}
                        data-testid="archive-asset"
                      >
                        归档素材
                      </Button>
                    )}
                  </Space>
                </div>

                {editingAnalysis ? (
                  <Form form={reviewForm} layout="vertical" data-testid="asset-review-form" className="asset-detail-review-form">
                    <Form.Item name="scene_description" label="画面描述">
                      <Input.TextArea rows={3} />
                    </Form.Item>
                    <div className="asset-detail-form-grid">
                      <Form.Item name="shot_size" label="景别">
                        <Select
                          allowClear
                          options={[
                            { value: "close_up", label: "特写" },
                            { value: "medium_close_up", label: "近景" },
                            { value: "medium_shot", label: "中景" },
                            { value: "wide_shot", label: "远景" }
                          ]}
                        />
                      </Form.Item>
                      <Form.Item name="camera_movement" label="运镜">
                        <Select
                          allowClear
                          options={[
                            { value: "static", label: "固定机位" },
                            { value: "slow_push_in", label: "缓慢推进" },
                            { value: "pan", label: "平移" },
                            { value: "handheld", label: "手持" }
                          ]}
                        />
                      </Form.Item>
                    </div>
                    <Form.Item name="subjects" label="主体标签">
                      <Select mode="tags" tokenSeparators={[","]} open={false} />
                    </Form.Item>
                    <Form.Item name="scene_tags" label="场景标签">
                      <Select mode="tags" tokenSeparators={[","]} open={false} />
                    </Form.Item>
                    <Form.Item name="quality_tags" label="质量标签">
                      <Select mode="tags" tokenSeparators={[","]} open={false} />
                    </Form.Item>
                    <Form.Item name="usability_status" label="可用性">
                      <Select
                        options={[
                          { value: "usable", label: "可用" },
                          { value: "needs_review", label: "待复核" },
                          { value: "discarded", label: "废弃" }
                        ]}
                      />
                    </Form.Item>
                    <Form.Item name="reviewer_notes" label="复核备注">
                      <Input.TextArea rows={2} />
                    </Form.Item>
                  </Form>
                ) : (
                  <Descriptions bordered column={1} size="small" data-testid="asset-analysis-panel" className="asset-detail-analysis-summary">
                    <Descriptions.Item label="画面描述">
                      {selectedAsset.scene_description || <Typography.Text type="secondary">暂无分析结果。</Typography.Text>}
                    </Descriptions.Item>
                    <Descriptions.Item label="景别">{translateValue(selectedAsset.shot_size, shotSizeLabels)}</Descriptions.Item>
                    <Descriptions.Item label="运镜">{translateValue(selectedAsset.camera_movement, cameraMovementLabels)}</Descriptions.Item>
                    <Descriptions.Item label="主体标签">{renderTagList(selectedAsset.subjects, "暂无主体标签")}</Descriptions.Item>
                    <Descriptions.Item label="场景标签">{renderTagList(selectedAsset.scene_tags, "暂无场景标签")}</Descriptions.Item>
                    <Descriptions.Item label="质量标签">{renderTagList(selectedAsset.quality_tags, "暂无质量问题")}</Descriptions.Item>
                    <Descriptions.Item label="复核备注">
                      {selectedAsset.reviewer_notes || <Typography.Text type="secondary">无</Typography.Text>}
                    </Descriptions.Item>
                  </Descriptions>
                )}
              </section>
            </div>

            <Tabs
              className="asset-detail-tabs"
              defaultActiveKey="basic"
              items={[
                {
                  key: "basic",
                  label: "基础信息",
                  forceRender: true,
                  children: (
                    <div className="asset-detail-tab-panel">
                      <Descriptions bordered column={2} size="small">
                        <Descriptions.Item label="文件">{assetFileDisplayName(selectedAsset)}</Descriptions.Item>
                        <Descriptions.Item label="素材类型">{translateValue(selectedAsset.source_type, sourceTypeLabels)}</Descriptions.Item>
                        <Descriptions.Item label="状态">{translateValue(selectedAsset.status, assetStatusLabels)}</Descriptions.Item>
                        <Descriptions.Item label="分析状态">{translateValue(selectedAsset.analysis_status, analysisStatusLabels)}</Descriptions.Item>
                        <Descriptions.Item label="时长">{formatDuration(selectedAsset.duration_ms)}</Descriptions.Item>
                        <Descriptions.Item label="分辨率">
                          {selectedAsset.width && selectedAsset.height ? `${selectedAsset.width}x${selectedAsset.height}` : "-"}
                        </Descriptions.Item>
                        <Descriptions.Item label="帧率">{selectedAsset.fps ?? "-"}</Descriptions.Item>
                        <Descriptions.Item label="视频编码">{selectedAsset.codec || "-"}</Descriptions.Item>
                        <Descriptions.Item label="是否含音频">{selectedAsset.has_audio ? "是" : "否"}</Descriptions.Item>
                        <Descriptions.Item label="音频编码">{selectedAsset.audio_codec || "-"}</Descriptions.Item>
                        <Descriptions.Item label="码率">{selectedAsset.bitrate_kbps ? `${selectedAsset.bitrate_kbps} kbps` : "-"}</Descriptions.Item>
                        <Descriptions.Item label="人工清洗状态">
                          {translateValue(selectedAsset.manual_clean_status, manualCleanStatusLabels)}
                        </Descriptions.Item>
                        <Descriptions.Item label="可用性">{translateValue(selectedAsset.usability_status, usabilityStatusLabels)}</Descriptions.Item>
                      </Descriptions>
                    </div>
                  )
                },
                {
                  key: "ai",
                  label: "AI 分析",
                  forceRender: true,
                  children: (
                    <div className="asset-detail-tab-panel">
                      <Descriptions bordered column={1} size="small">
                        <Descriptions.Item label="分析错误">
                          {selectedAsset.analysis_error || <Typography.Text type="secondary">无</Typography.Text>}
                        </Descriptions.Item>
                        <Descriptions.Item label="模型原始结果">
                          {selectedAsset.model_result && Object.keys(selectedAsset.model_result).length > 0 ? (
                            <pre className="json-block">{JSON.stringify(selectedAsset.model_result, null, 2)}</pre>
                          ) : (
                            <Typography.Text type="secondary">暂无模型原始输出</Typography.Text>
                          )}
                        </Descriptions.Item>
                        <Descriptions.Item label="模型标准化标签">
                          {selectedAsset.model_labels && Object.keys(selectedAsset.model_labels).length > 0 ? (
                            <pre className="json-block">{JSON.stringify(selectedAsset.model_labels, null, 2)}</pre>
                          ) : (
                            <Typography.Text type="secondary">暂无标准化模型标签</Typography.Text>
                          )}
                        </Descriptions.Item>
                        <Descriptions.Item label="人工覆盖结果">
                          {selectedAsset.review_overrides && Object.keys(selectedAsset.review_overrides).length > 0 ? (
                            <pre className="json-block">{JSON.stringify(selectedAsset.review_overrides, null, 2)}</pre>
                          ) : (
                            <Typography.Text type="secondary">暂无人工覆盖</Typography.Text>
                          )}
                        </Descriptions.Item>
                      </Descriptions>
                    </div>
                  )
                },
                {
                  key: "business",
                  label: "业务标签",
                  forceRender: true,
                  children: (
                    <div className="asset-detail-tab-panel">
                      <Card
                        title="精选业务标签"
                        extra={
                          <Button type="primary" size="small" loading={savingBusinessTags} onClick={saveAssetBusinessTags}>
                            保存精选标签
                          </Button>
                        }
                      >
                        <Space direction="vertical" className="wide-space">
                          <Form form={businessTagForm} layout="vertical">
                            <Form.Item name="is_curated" label="是否精选">
                              <Select
                                options={[
                                  { value: true, label: "精选素材" },
                                  { value: false, label: "普通素材" }
                                ]}
                              />
                            </Form.Item>
                            <Form.Item name="business_tags" label="业务标签">
                              <Select mode="tags" tokenSeparators={[","]} open={false} />
                            </Form.Item>
                            <Form.Item name="narrative_roles" label="叙事角色">
                              <Select mode="tags" tokenSeparators={[","]} open={false} />
                            </Form.Item>
                            <Form.Item name="usage_notes" label="使用建议">
                              <Input.TextArea rows={3} />
                            </Form.Item>
                          </Form>
                          <Descriptions bordered column={1} size="small">
                            <Descriptions.Item label="当前精选状态">
                              {selectedAsset.metadata?.is_curated ? "精选素材" : "普通素材"}
                            </Descriptions.Item>
                            <Descriptions.Item label="当前业务标签">
                              {renderTagList(
                                Array.isArray(selectedAsset.metadata?.business_tags) ? (selectedAsset.metadata?.business_tags as string[]) : [],
                                "暂无业务标签"
                              )}
                            </Descriptions.Item>
                            <Descriptions.Item label="当前叙事角色">
                              {renderTagList(
                                Array.isArray(selectedAsset.metadata?.narrative_roles) ? (selectedAsset.metadata?.narrative_roles as string[]) : [],
                                "暂无叙事角色"
                              )}
                            </Descriptions.Item>
                            <Descriptions.Item label="当前使用建议">
                              {typeof selectedAsset.metadata?.usage_notes === "string" && selectedAsset.metadata?.usage_notes ? (
                                selectedAsset.metadata?.usage_notes as string
                              ) : (
                                <Typography.Text type="secondary">暂无使用建议</Typography.Text>
                              )}
                            </Descriptions.Item>
                          </Descriptions>
                        </Space>
                      </Card>
                    </div>
                  )
                },
                {
                  key: "selling-points",
                  label: "卖点关联",
                  forceRender: true,
                  children: (
                    <div className="asset-detail-tab-panel">
                      <Card
                        title="卖点关联"
                        extra={
                          <Button
                            type="primary"
                            size="small"
                            loading={savingSellingPoints}
                            onClick={saveAssetSellingPoints}
                            data-testid="save-asset-selling-points"
                          >
                            保存卖点关联
                          </Button>
                        }
                      >
                        <Space direction="vertical" className="wide-space">
                          <Form form={sellingPointForm} layout="vertical">
                            <Form.Item name="selling_point_ids" label="关联卖点">
                              <Select
                                mode="multiple"
                                allowClear
                                placeholder="请选择卖点"
                                options={(assetDetailSellingPoints.data ?? []).map((item) => ({
                                  value: item.id,
                                  label: item.title
                                }))}
                                data-testid="asset-selling-points-select"
                              />
                            </Form.Item>
                          </Form>
                          <Descriptions bordered column={1} size="small">
                            <Descriptions.Item label="当前关联">
                              {assetSellingPoints.length > 0 ? (
                                <Space wrap size={[6, 6]}>
                                  {assetSellingPoints.map((item) => (
                                    <Tag key={item.id}>{item.title}</Tag>
                                  ))}
                                </Space>
                              ) : (
                                <Typography.Text type="secondary">暂无关联卖点。</Typography.Text>
                              )}
                            </Descriptions.Item>
                          </Descriptions>
                        </Space>
                      </Card>
                    </div>
                  )
                },
                {
                  key: "frames",
                  label: "抽帧预览",
                  forceRender: true,
                  children: (
                    <div className="asset-detail-tab-panel">
                      <Card title="抽帧预览" loading={framesLoading}>
                        {frames.length === 0 ? (
                          <Empty description="暂无抽帧结果" />
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
                                <Typography.Text type="secondary">第 {frame.frame_index} 帧</Typography.Text>
                              </div>
                            ))}
                          </div>
                        )}
                      </Card>
                    </div>
                  )
                },
                ...(selectedAsset.source_type === "talking_head"
                  ? [
                      {
                        key: "speech",
                        label: "口播句段",
                        forceRender: true,
                        children: (
                          <div className="asset-detail-tab-panel">
                            <Card title="口播句段" loading={speechSegmentsLoading}>
                              {speechSegments.length === 0 ? (
                                <Empty description="暂无已入库的口播句段" />
                              ) : (
                                <Table<AssetSpeechSegment>
                                  rowKey="id"
                                  dataSource={speechSegments}
                                  pagination={false}
                                  size="small"
                                  columns={[
                                    {
                                      title: "时间",
                                      render: (_, segment) => `${formatTimestamp(segment.start_ms)} - ${formatTimestamp(segment.end_ms)}`
                                    },
                                    {
                                      title: "文本",
                                      dataIndex: "transcript"
                                    },
                                    {
                                      title: "来源",
                                      dataIndex: "source"
                                    },
                                    {
                                      title: "状态",
                                      dataIndex: "status"
                                    }
                                  ]}
                                />
                              )}
                            </Card>
                          </div>
                        )
                      }
                    ]
                  : []),
                {
                  key: "semantic",
                  label: "向量预览",
                  forceRender: true,
                  children: (
                    <div className="asset-detail-tab-panel">
                      <Card title="开放语义与向量化对象" loading={semanticPreviewLoading}>
                        <Descriptions bordered column={1} size="small">
                          <Descriptions.Item label="开放语义描述">
                            {semanticPreview?.open_semantic_description ? (
                              semanticPreview.open_semantic_description
                            ) : (
                              <Typography.Text type="secondary">暂无开放语义描述</Typography.Text>
                            )}
                          </Descriptions.Item>
                          <Descriptions.Item label="向量化对象预览">
                            {semanticPreview && semanticPreview.embedding_targets.length > 0 ? (
                              <Table<AssetEmbeddingTarget>
                                rowKey="object_id"
                                dataSource={semanticPreview.embedding_targets}
                                pagination={false}
                                size="small"
                                columns={[
                                  {
                                    title: "对象类型",
                                    render: (_, item) => item.object_type
                                  },
                                  {
                                    title: "对象文本",
                                    dataIndex: "text"
                                  },
                                  {
                                    title: "元数据",
                                    render: (_, item) =>
                                      item.metadata && Object.keys(item.metadata).length > 0 ? (
                                        <pre className="json-block">{JSON.stringify(item.metadata, null, 2)}</pre>
                                      ) : (
                                        <Typography.Text type="secondary">无</Typography.Text>
                                      )
                                  }
                                ]}
                              />
                            ) : (
                              <Typography.Text type="secondary">暂无向量化对象预览</Typography.Text>
                            )}
                          </Descriptions.Item>
                        </Descriptions>
                      </Card>
                    </div>
                  )
                }
              ]}
            />
          </div>
        ) : null}
      </Modal>
    </div>
  );
}

function TasksPage({ token }: { token: string }) {
  const [taskFilters, setTaskFilters] = useState({
    taskType: "",
    status: ""
  });
  const taskPath = useMemo(() => {
    const params = new URLSearchParams();
    if (taskFilters.taskType) {
      params.set("task_type", taskFilters.taskType);
    }
    if (taskFilters.status) {
      params.set("status", taskFilters.status);
    }
    const query = params.toString();
    return query ? `/api/tasks?${query}` : "/api/tasks";
  }, [taskFilters]);
  const tasks = useResource<Task[]>(taskPath, token, [taskPath]);
  const [creating, setCreating] = useState(false);
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const taskTypeLabel = (taskType: string) => {
    return translateValue(taskType, taskTypeLabels);
  };

  const createTask = async () => {
    setCreating(true);
    try {
      await apiRequest<Task>("/api/tasks/test", { method: "POST" }, token);
      await tasks.reload();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "创建任务失败");
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
      message.error(error instanceof Error ? error.message : "加载任务失败");
    } finally {
      setDetailLoading(false);
    }
  };

  return (
    <div data-testid="tasks-page">
      <Space direction="vertical" size="middle" className="page-stack">
        <Typography.Title level={3}>任务</Typography.Title>
        <Card title="任务筛选">
          <Space wrap>
            <Select
              data-testid="task-filter-type"
              value={taskFilters.taskType || undefined}
              placeholder="任务类型"
              allowClear
              style={{ minWidth: 220 }}
              options={[
                { value: "asset_extract_frames", label: "素材抽帧" },
                { value: "asset_analyze", label: "素材分析" },
                { value: "test", label: "测试任务" }
              ]}
              onChange={(value) => setTaskFilters((current) => ({ ...current, taskType: value ?? "" }))}
            />
            <Select
              data-testid="task-filter-status"
              value={taskFilters.status || undefined}
              placeholder="状态"
              allowClear
              style={{ minWidth: 160 }}
              options={[
                { value: "queued", label: "排队中" },
                { value: "running", label: "执行中" },
                { value: "completed", label: "已完成" },
                { value: "failed", label: "失败" }
              ]}
              onChange={(value) => setTaskFilters((current) => ({ ...current, status: value ?? "" }))}
            />
            <Button data-testid="task-filter-reset" onClick={() => setTaskFilters({ taskType: "", status: "" })}>重置</Button>
            <Button data-testid="task-filter-refresh" onClick={tasks.reload}>刷新</Button>
          </Space>
        </Card>
        <Card title="批量剪辑任务" extra={<Button type="primary" loading={creating} onClick={createTask}>创建测试任务</Button>}>
          <Table<Task>
            rowKey="id"
            loading={tasks.loading}
            dataSource={tasks.data ?? []}
            onRow={(record) => ({ onClick: () => void openTaskDetail(record.id) })}
            columns={[
              {
                title: "任务 ID",
                dataIndex: "id",
                render: (value: string, task) => (
                  <Button type="link" className="table-link-button" onClick={() => void openTaskDetail(task.id)}>
                    {value}
                  </Button>
                )
              },
              { title: "类型", dataIndex: "task_type", render: (value) => taskTypeLabel(value) },
              { title: "状态", dataIndex: "status", render: (status) => <Tag>{translateValue(status, taskStatusLabels)}</Tag> },
              { title: "素材 ID", dataIndex: "asset_id", render: (value) => value || "-" },
              { title: "重试次数", dataIndex: "retry_count" },
              { title: "耗时", dataIndex: "duration_ms", render: (value) => (value ? `${value} 毫秒` : "-") },
              { title: "创建时间", dataIndex: "created_at", render: (value) => formatDateTime(value) }
            ]}
          />
        </Card>
      </Space>

      <Modal
        title={selectedTask ? `任务详情：${selectedTask.id}` : "任务详情"}
        open={selectedTask !== null}
        footer={null}
        width={840}
        confirmLoading={detailLoading}
        onCancel={() => setSelectedTask(null)}
      >
        {selectedTask ? (
          <Descriptions bordered column={1} size="small" data-testid="task-detail-modal">
            <Descriptions.Item label="任务类型">{taskTypeLabel(selectedTask.task_type)}</Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag>{translateValue(selectedTask.status, taskStatusLabels)}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="素材 ID">{selectedTask.asset_id || "-"}</Descriptions.Item>
            <Descriptions.Item label="重试次数">{selectedTask.retry_count}</Descriptions.Item>
            <Descriptions.Item label="耗时">{selectedTask.duration_ms ? `${selectedTask.duration_ms} 毫秒` : "-"}</Descriptions.Item>
            <Descriptions.Item label="创建时间">{formatDateTime(selectedTask.created_at)}</Descriptions.Item>
            <Descriptions.Item label="开始时间">{formatDateTime(selectedTask.started_at)}</Descriptions.Item>
            <Descriptions.Item label="结束时间">{formatDateTime(selectedTask.finished_at)}</Descriptions.Item>
            <Descriptions.Item label="错误信息">
              {selectedTask.error_message || <Typography.Text type="secondary">无</Typography.Text>}
            </Descriptions.Item>
            <Descriptions.Item label="负载摘要">
              {selectedTask.payload_summary && Object.keys(selectedTask.payload_summary).length > 0 ? (
                <pre className="json-block">{JSON.stringify(selectedTask.payload_summary, null, 2)}</pre>
              ) : (
                <Typography.Text type="secondary">暂无负载摘要</Typography.Text>
              )}
            </Descriptions.Item>
          </Descriptions>
        ) : null}
      </Modal>
    </div>
  );
}

function LegacySettingsPage({ token }: { token: string }) {
  const providerSettings = useResource<OpenAICompatibleSettings>("/api/admin/model-access/openai-compatible", token);
  const runtimeSettings = useResource<RuntimeSettings>("/api/admin/runtime-settings", token);
  const [providerForm] = Form.useForm();
  const [runtimeForm] = Form.useForm();
  const [modelOptions, setModelOptions] = useState<ModelSelectOption[]>([]);
  const [testingConnection, setTestingConnection] = useState(false);
  const [loadingModels, setLoadingModels] = useState(false);
  const [savingProvider, setSavingProvider] = useState(false);
  const [savingRuntime, setSavingRuntime] = useState(false);
  const [lastModelCount, setLastModelCount] = useState<number | null>(null);
  const [connectionStatus, setConnectionStatus] = useState<"idle" | "success" | "error">("idle");

  const mergeModelOptions = (items: string[]) => {
    const unique = Array.from(new Set(items.map((item) => item.trim()).filter(Boolean))).sort((a, b) => a.localeCompare(b));
    return unique.map((item) => ({ value: item, label: item }));
  };

  const syncModelOptions = (items: string[]) => {
    const currentValues = providerForm.getFieldsValue(["llm_model", "vlm_model", "embedding_model"]);
    setModelOptions(
      mergeModelOptions([
        ...(items ?? []),
        currentValues.llm_model ?? "",
        currentValues.vlm_model ?? "",
        currentValues.embedding_model ?? ""
      ])
    );
  };

  useEffect(() => {
    if (!providerSettings.data) {
      return;
    }
    providerForm.setFieldsValue({
      base_url: providerSettings.data.base_url,
      api_key: "",
      llm_model: providerSettings.data.llm_model,
      vlm_model: providerSettings.data.vlm_model,
      embedding_model: providerSettings.data.embedding_model
    });
    syncModelOptions([providerSettings.data.llm_model, providerSettings.data.vlm_model, providerSettings.data.embedding_model]);
  }, [providerForm, providerSettings.data]);

  useEffect(() => {
    if (!runtimeSettings.data) {
      return;
    }
    runtimeForm.setFieldsValue(runtimeSettings.data);
  }, [runtimeForm, runtimeSettings.data]);

  const loadModels = async (showSuccessMessage: boolean) => {
    await providerForm.validateFields(["base_url"]);
    const values = providerForm.getFieldsValue();
    setLoadingModels(true);
    try {
      const result = await apiRequest<ModelDiscoveryResult>(
        "/api/admin/model-access/openai-compatible/models",
        {
          method: "POST",
          body: JSON.stringify({
            base_url: values.base_url ?? "",
            api_key: values.api_key ?? ""
          })
        },
        token
      );
      const discovered = result.models.map((item) => item.id);
      syncModelOptions(discovered);
      setLastModelCount(discovered.length);
      setConnectionStatus("success");
      if (showSuccessMessage) {
        message.success(discovered.length > 0 ? `已获取 ${discovered.length} 个模型。` : "连接成功，但当前端点未返回可用模型。");
      }
    } catch (error) {
      setConnectionStatus("error");
      message.error(error instanceof Error ? error.message : "获取模型列表失败");
    } finally {
      setLoadingModels(false);
    }
  };

  useEffect(() => {
    if (!providerSettings.data?.base_url) {
      return;
    }
    void loadModels(false);
  }, [providerSettings.data?.base_url]);

  const testConnection = async () => {
    await providerForm.validateFields(["base_url"]);
    const values = providerForm.getFieldsValue();
    setTestingConnection(true);
    try {
      const result = await apiRequest<{ reachable: boolean; model_count: number }>(
        "/api/admin/model-access/openai-compatible/test",
        {
          method: "POST",
          body: JSON.stringify({
            base_url: values.base_url ?? "",
            api_key: values.api_key ?? ""
          })
        },
        token
      );
      setLastModelCount(result.model_count);
      setConnectionStatus("success");
      message.success(`连接成功，当前可见模型数：${result.model_count}`);
    } catch (error) {
      setConnectionStatus("error");
      message.error(error instanceof Error ? error.message : "连接测试失败");
    } finally {
      setTestingConnection(false);
    }
  };

  const saveProviderSettings = async () => {
    const values = await providerForm.validateFields();
    setSavingProvider(true);
    try {
      await apiRequest<OpenAICompatibleSettings>(
        "/api/admin/model-access/openai-compatible",
        {
          method: "PUT",
          body: JSON.stringify(values)
        },
        token
      );
      await providerSettings.reload();
      syncModelOptions([values.llm_model ?? "", values.vlm_model ?? "", values.embedding_model ?? ""]);
      message.success("模型接入配置已保存。");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存模型接入配置失败");
    } finally {
      setSavingProvider(false);
    }
  };

  const saveRuntimeSettings = async () => {
    const values = await runtimeForm.validateFields();
    setSavingRuntime(true);
    try {
      await apiRequest<RuntimeSettings>(
        "/api/admin/runtime-settings",
        {
          method: "PUT",
          body: JSON.stringify(values)
        },
        token
      );
      await runtimeSettings.reload();
      message.success("运行控制配置已保存。");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存运行控制配置失败");
    } finally {
      setSavingRuntime(false);
    }
  };

  const providerSummaryItems = [
    { label: "接入协议", value: "OpenAI Compatible" },
    { label: "Base URL", value: providerSettings.data?.base_url || "未配置" },
    { label: "密钥状态", value: providerSettings.data?.api_key_configured ? "已保存" : "未保存" },
    { label: "模型发现", value: lastModelCount === null ? "未检测" : `${lastModelCount} 个模型` }
  ];

  const connectionAlert =
    connectionStatus === "success"
      ? {
          type: "success" as const,
          message: "端点连通正常",
          description: lastModelCount === null ? "可以继续拉取模型列表并保存默认模型。" : `最近一次检测发现 ${lastModelCount} 个模型。`
        }
      : connectionStatus === "error"
        ? {
            type: "error" as const,
            message: "最近一次连接失败",
            description: "请检查 Base URL、密钥和服务端网络连通性。"
          }
        : {
            type: "info" as const,
            message: "建议先测试连接，再拉取模型列表",
            description: "模型不再手动输入，而是从兼容 OpenAI 协议的端点自动发现。"
          };

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>系统设置</Typography.Title>

      <Card
        title="模型接入"
        extra={<Button onClick={providerSettings.reload}>刷新</Button>}
        loading={providerSettings.loading}
      >
        <Space direction="vertical" className="wide-space" size="middle">
          <Typography.Text type="secondary">
            当前仅支持 OpenAI 兼容协议端点。填写端点地址和密钥后，模型列表从端点拉取，不需要手动输入模型名称。
          </Typography.Text>

          <Form form={providerForm} layout="vertical">
            <Form.Item label="协议类型">
              <Input value="OpenAI Compatible" disabled />
            </Form.Item>
            <Form.Item
              name="base_url"
              label="Base URL"
              rules={[{ required: true, message: "请输入 Base URL" }]}
            >
              <Input placeholder="例如：https://api.openai.com/v1" />
            </Form.Item>
            <Form.Item
              name="api_key"
              label="API Key"
              extra={providerSettings.data?.api_key_configured ? "当前已保存密钥；留空表示保持现有密钥不变。" : undefined}
            >
              <Input.Password placeholder="留空则不修改已保存密钥" />
            </Form.Item>
            <Space wrap>
              <Button loading={testingConnection} onClick={() => void testConnection()}>
                测试连接
              </Button>
              <Button loading={loadingModels} onClick={() => void loadModels(true)}>
                获取模型列表
              </Button>
              <Button type="primary" loading={savingProvider} onClick={() => void saveProviderSettings()}>
                保存模型配置
              </Button>
            </Space>
            <Form.Item
              name="llm_model"
              label="默认 LLM 模型"
              rules={[{ required: true, message: "请选择默认 LLM 模型" }]}
            >
              <Select
                showSearch
                placeholder="请先获取模型列表"
                options={modelOptions}
                filterOption={(input, option) => String(option?.label ?? "").toLowerCase().includes(input.toLowerCase())}
              />
            </Form.Item>
            <Form.Item
              name="vlm_model"
              label="默认 VLM 模型"
              rules={[{ required: true, message: "请选择默认 VLM 模型" }]}
            >
              <Select
                showSearch
                placeholder="请先获取模型列表"
                options={modelOptions}
                filterOption={(input, option) => String(option?.label ?? "").toLowerCase().includes(input.toLowerCase())}
              />
            </Form.Item>
            <Form.Item
              name="embedding_model"
              label="默认向量模型"
              extra="用于素材语义文本向量化和后续 pgvector 相似度召回。"
              rules={[{ required: true, message: "请选择默认向量模型" }]}
            >
              <Select
                showSearch
                placeholder="请先获取模型列表"
                options={modelOptions}
                filterOption={(input, option) => String(option?.label ?? "").toLowerCase().includes(input.toLowerCase())}
              />
            </Form.Item>
          </Form>
        </Space>
      </Card>

      <Card
        title="运行控制"
        extra={<Button onClick={runtimeSettings.reload}>刷新</Button>}
        loading={runtimeSettings.loading}
      >
        <Form form={runtimeForm} layout="vertical">
          <Space align="start" size="large" wrap className="wide-space">
            <Form.Item
              name="llm_max_concurrency"
              label="LLM 并发数"
              rules={[{ required: true, message: "请输入 LLM 并发数" }]}
            >
              <InputNumber min={1} style={{ width: 180 }} />
            </Form.Item>
            <Form.Item
              name="vlm_max_concurrency"
              label="VLM 并发数"
              rules={[{ required: true, message: "请输入 VLM 并发数" }]}
            >
              <InputNumber min={1} style={{ width: 180 }} />
            </Form.Item>
            <Form.Item
              name="asr_max_concurrency"
              label="ASR 并发数"
              rules={[{ required: true, message: "请输入 ASR 并发数" }]}
            >
              <InputNumber min={1} style={{ width: 180 }} />
            </Form.Item>
            <Form.Item
              name="tts_max_concurrency"
              label="TTS 并发数"
              rules={[{ required: true, message: "请输入 TTS 并发数" }]}
            >
              <InputNumber min={1} style={{ width: 180 }} />
            </Form.Item>
            <Form.Item
              name="render_max_concurrency"
              label="渲染并发数"
              rules={[{ required: true, message: "请输入渲染并发数" }]}
            >
              <InputNumber min={1} style={{ width: 180 }} />
            </Form.Item>
            <Form.Item
              name="task_max_queued_per_user"
              label="单用户最大排队任务数"
              rules={[{ required: true, message: "请输入单用户最大排队任务数" }]}
            >
              <InputNumber min={1} style={{ width: 220 }} />
            </Form.Item>
            <Form.Item
              name="task_max_running_per_user"
              label="单用户最大运行任务数"
              rules={[{ required: true, message: "请输入单用户最大运行任务数" }]}
            >
              <InputNumber min={1} style={{ width: 220 }} />
            </Form.Item>
            <Form.Item
              name="vlm_timeout_seconds"
              label="VLM 超时秒数"
              rules={[{ required: true, message: "请输入 VLM 超时秒数" }]}
            >
              <InputNumber min={1} style={{ width: 180 }} />
            </Form.Item>
            <Form.Item
              name="vlm_max_retries"
              label="VLM 最大重试次数"
              rules={[{ required: true, message: "请输入 VLM 最大重试次数" }]}
            >
              <InputNumber min={0} style={{ width: 180 }} />
            </Form.Item>
          </Space>
          <Button type="primary" loading={savingRuntime} onClick={() => void saveRuntimeSettings()}>
            保存运行控制
          </Button>
        </Form>
      </Card>
    </Space>
  );
}

function SettingsPage({ token }: { token: string }) {
  const providersResource = useResource<ModelProvider[]>("/api/admin/model-providers", token);
  const capabilitySettings = useResource<ModelCapabilitySettings>("/api/admin/model-settings", token);
  const runtimeSettings = useResource<RuntimeSettings>("/api/admin/runtime-settings", token);
  const [providerForm] = Form.useForm();
  const [capabilityForm] = Form.useForm();
  const [runtimeForm] = Form.useForm();
  const [providerModalOpen, setProviderModalOpen] = useState(false);
  const [editingProvider, setEditingProvider] = useState<ModelProvider | null>(null);
  const [modelsByProvider, setModelsByProvider] = useState<Record<string, ModelSelectOption[]>>({});
  const [testingConnection, setTestingConnection] = useState(false);
  const [loadingModels, setLoadingModels] = useState(false);
  const [savingProvider, setSavingProvider] = useState(false);
  const [savingCapabilities, setSavingCapabilities] = useState(false);
  const [savingRuntime, setSavingRuntime] = useState(false);
  const [lastModelCount, setLastModelCount] = useState<number | null>(null);
  const [settingsTab, setSettingsTab] = useState<"providers" | "models" | "runtime">("providers");

  const providers = providersResource.data ?? [];
  const providerOptions = providers.map((provider) => ({ value: provider.id, label: provider.name }));

  useEffect(() => {
    const settings = capabilitySettings.data;
    if (!settings) {
      return;
    }
    capabilityForm.setFieldsValue({
      llm_provider_id: settings.llm.provider_id,
      llm_model: settings.llm.model,
      vlm_provider_id: settings.vlm.provider_id,
      vlm_model: settings.vlm.model,
      embedding_provider_id: settings.embedding.provider_id,
      embedding_model: settings.embedding.model
    });
    setModelsByProvider((current) => {
      const next = { ...current };
      for (const setting of [settings.llm, settings.vlm, settings.embedding]) {
        if (!setting.provider_id || !setting.model) {
          continue;
        }
        const existing = next[setting.provider_id] ?? [];
        if (!existing.some((item) => item.value === setting.model)) {
          next[setting.provider_id] = [...existing, { value: setting.model, label: setting.model }];
        }
      }
      return next;
    });
  }, [capabilityForm, capabilitySettings.data]);

  useEffect(() => {
    if (!runtimeSettings.data) {
      return;
    }
    runtimeForm.setFieldsValue(runtimeSettings.data);
  }, [runtimeForm, runtimeSettings.data]);

  const openCreateProvider = () => {
    setEditingProvider(null);
    providerForm.setFieldsValue({
      name: "",
      provider_type: "openai_compatible",
      base_url: "",
      api_key: "",
      enabled: true
    });
    setProviderModalOpen(true);
  };

  const openEditProvider = (provider: ModelProvider) => {
    setEditingProvider(provider);
    providerForm.setFieldsValue({
      name: provider.name,
      provider_type: provider.provider_type,
      base_url: provider.base_url,
      api_key: "",
      enabled: provider.enabled
    });
    setProviderModalOpen(true);
  };

  const saveProvider = async () => {
    const values = await providerForm.validateFields();
    setSavingProvider(true);
    try {
      const path = editingProvider ? `/api/admin/model-providers/${editingProvider.id}` : "/api/admin/model-providers";
      await apiRequest<ModelProvider>(
        path,
        {
          method: editingProvider ? "PUT" : "POST",
          body: JSON.stringify(values)
        },
        token
      );
      setProviderModalOpen(false);
      await providersResource.reload();
      message.success("供应商已保存");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存供应商失败");
    } finally {
      setSavingProvider(false);
    }
  };

  const deleteProvider = async (providerID: string) => {
    try {
      await apiRequest<{ deleted: boolean }>(`/api/admin/model-providers/${providerID}`, { method: "DELETE" }, token);
      await providersResource.reload();
      message.success("供应商已删除");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "删除供应商失败");
    }
  };

  const loadModels = async (providerID: string, showSuccessMessage: boolean) => {
    if (!providerID) {
      message.warning("请先选择供应商");
      return;
    }
    setLoadingModels(true);
    try {
      const result = await apiRequest<ModelDiscoveryResult>(`/api/admin/model-providers/${providerID}/models`, { method: "POST" }, token);
      const discovered = result.models.map((item) => ({ value: item.id, label: item.id }));
      setModelsByProvider((current) => ({ ...current, [providerID]: discovered }));
      setLastModelCount(discovered.length);
      if (showSuccessMessage) {
        message.success(discovered.length > 0 ? `已获取 ${discovered.length} 个模型` : "连接成功，但当前端点未返回模型");
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : "获取模型列表失败");
    } finally {
      setLoadingModels(false);
    }
  };

  const testConnection = async (providerID: string) => {
    if (!providerID) {
      message.warning("请先选择供应商");
      return;
    }
    setTestingConnection(true);
    try {
      const result = await apiRequest<{ reachable: boolean; model_count: number }>(`/api/admin/model-providers/${providerID}/test`, { method: "POST" }, token);
      setLastModelCount(result.model_count);
      message.success(`连接成功，当前可见模型数：${result.model_count}`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "连接测试失败");
    } finally {
      setTestingConnection(false);
    }
  };

  const saveCapabilitySettings = async () => {
    const values = await capabilityForm.validateFields();
    setSavingCapabilities(true);
    try {
      await apiRequest<ModelCapabilitySettings>(
        "/api/admin/model-settings",
        {
          method: "PUT",
          body: JSON.stringify({
            llm: { provider_id: values.llm_provider_id, model: values.llm_model },
            vlm: { provider_id: values.vlm_provider_id, model: values.vlm_model },
            embedding: { provider_id: values.embedding_provider_id, model: values.embedding_model }
          })
        },
        token
      );
      await capabilitySettings.reload();
      message.success("默认模型已保存");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存默认模型失败");
    } finally {
      setSavingCapabilities(false);
    }
  };

  const saveRuntimeSettings = async () => {
    const values = await runtimeForm.validateFields();
    setSavingRuntime(true);
    try {
      await apiRequest<RuntimeSettings>(
        "/api/admin/runtime-settings",
        {
          method: "PUT",
          body: JSON.stringify(values)
        },
        token
      );
      await runtimeSettings.reload();
      message.success("运行控制配置已保存");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存运行控制配置失败");
    } finally {
      setSavingRuntime(false);
    }
  };

  const selectedLLMProviderID = Form.useWatch("llm_provider_id", capabilityForm);
  const selectedVLMProviderID = Form.useWatch("vlm_provider_id", capabilityForm);
  const selectedEmbeddingProviderID = Form.useWatch("embedding_provider_id", capabilityForm);

  const capabilityRows = [
    { key: "llm", title: "LLM", providerField: "llm_provider_id", modelField: "llm_model", selectedProviderID: selectedLLMProviderID, description: "用于文案生成、编排等文本推理任务。" },
    { key: "vlm", title: "VLM", providerField: "vlm_provider_id", modelField: "vlm_model", selectedProviderID: selectedVLMProviderID, description: "用于素材抽帧分析、画面理解和标签提取。" },
    { key: "embedding", title: "向量模型", providerField: "embedding_provider_id", modelField: "embedding_model", selectedProviderID: selectedEmbeddingProviderID, description: "用于素材语义文本向量化和后续 pgvector 检索。" }
  ];

  return (
    <Space direction="vertical" size="middle" className="page-stack settings-page-stack">
      <Tabs
        className="settings-top-tabs"
        activeKey={settingsTab}
        onChange={(key) => setSettingsTab(key as "providers" | "models" | "runtime")}
        items={[
          { key: "providers", label: "模型供应商" },
          { key: "models", label: "默认模型" },
          { key: "runtime", label: "运行控制" }
        ]}
      />

      <Card
        className="settings-tab-panel"
        style={{ display: settingsTab === "providers" ? undefined : "none" }}
        extra={
          <Space>
            <Button onClick={providersResource.reload}>刷新</Button>
            <Button type="primary" onClick={openCreateProvider}>新增供应商</Button>
          </Space>
        }
        loading={providersResource.loading}
      >
        <Space direction="vertical" size="large" className="wide-space">
          <Table
            rowKey="id"
            pagination={false}
            dataSource={providers}
            columns={[
              { title: "名称", dataIndex: "name" },
              { title: "协议", dataIndex: "provider_type", render: () => "OpenAI Compatible" },
              { title: "Base URL", dataIndex: "base_url", ellipsis: true },
              { title: "密钥", dataIndex: "api_key_configured", render: (configured: boolean) => configured ? <Tag color="green">已保存</Tag> : <Tag>未保存</Tag> },
              { title: "状态", dataIndex: "enabled", render: (enabled: boolean) => enabled ? <Tag color="blue">启用</Tag> : <Tag>停用</Tag> },
              {
                title: "操作",
                render: (_: unknown, provider: ModelProvider) => (
                  <Space>
                    <Button size="small" loading={testingConnection} onClick={() => void testConnection(provider.id)}>测试</Button>
                    <Button size="small" loading={loadingModels} onClick={() => void loadModels(provider.id, true)}>拉取模型</Button>
                    <Button size="small" onClick={() => openEditProvider(provider)}>编辑</Button>
                    <Popconfirm title="确认删除该供应商？" onConfirm={() => void deleteProvider(provider.id)}>
                      <Button size="small" danger>删除</Button>
                    </Popconfirm>
                  </Space>
                )
              }
            ]}
          />
        </Space>
      </Card>

      <Card
        className="settings-tab-panel"
        style={{ display: settingsTab === "models" ? undefined : "none" }}
        extra={<Button type="primary" loading={savingCapabilities} onClick={() => void saveCapabilitySettings()}>保存默认模型</Button>}
        loading={capabilitySettings.loading}
      >
        <Form form={capabilityForm} layout="vertical">
          <Space direction="vertical" size="middle" className="wide-space">
            {capabilityRows.map((row) => {
              const modelOptions = modelsByProvider[row.selectedProviderID] ?? [];
              return (
                <Card size="small" key={row.key} title={row.title} className="settings-inner-card">
                  <Typography.Paragraph type="secondary">{row.description}</Typography.Paragraph>
                  <div className="settings-form-grid">
                    <Form.Item name={row.providerField} label="供应商" rules={[{ required: true, message: "请选择供应商" }]}>
                      <Select
                        placeholder="选择供应商"
                        options={providerOptions}
                        onChange={(providerID) => {
                          capabilityForm.setFieldValue(row.modelField, "");
                          if (!modelsByProvider[providerID]) {
                            void loadModels(providerID, false);
                          }
                        }}
                      />
                    </Form.Item>
                    <Form.Item name={row.modelField} label="模型" rules={[{ required: true, message: "请选择模型" }]}>
                      <Select
                        showSearch
                        placeholder={row.selectedProviderID ? "选择模型" : "请先选择供应商"}
                        options={modelOptions}
                        disabled={!row.selectedProviderID || modelOptions.length === 0}
                        filterOption={(input, option) => String(option?.label ?? "").toLowerCase().includes(input.toLowerCase())}
                      />
                    </Form.Item>
                  </div>
                  <Button size="small" disabled={!row.selectedProviderID} loading={loadingModels} onClick={() => void loadModels(row.selectedProviderID, true)}>
                    重新拉取该供应商模型
                  </Button>
                </Card>
              );
            })}
            {lastModelCount !== null && <Typography.Text type="secondary">最近一次拉取到 {lastModelCount} 个模型。</Typography.Text>}
          </Space>
        </Form>
      </Card>

      <Modal
        title={editingProvider ? "编辑供应商" : "新增供应商"}
        open={providerModalOpen}
        onCancel={() => setProviderModalOpen(false)}
        onOk={() => void saveProvider()}
        confirmLoading={savingProvider}
        destroyOnHidden
      >
        <Form form={providerForm} layout="vertical">
          <Form.Item name="name" label="供应商名称" rules={[{ required: true, message: "请输入供应商名称" }]}>
            <Input placeholder="例如：DeepSeek、硅基流动、OpenAI" />
          </Form.Item>
          <Form.Item name="provider_type" label="协议类型" initialValue="openai_compatible">
            <Input value="openai_compatible" disabled />
          </Form.Item>
          <Form.Item name="base_url" label="Base URL" rules={[{ required: true, message: "请输入 Base URL" }]}>
            <Input placeholder="https://api.example.com/v1" />
          </Form.Item>
          <Form.Item
            name="api_key"
            label="API Key"
            extra={editingProvider?.api_key_configured ? "留空表示保持当前已保存密钥不变。" : "如果供应商需要鉴权，请填写访问密钥。"}
          >
            <Input.Password placeholder="sk-..." />
          </Form.Item>
          <Form.Item name="enabled" label="状态">
            <Select options={[{ value: true, label: "启用" }, { value: false, label: "停用" }]} />
          </Form.Item>
        </Form>
      </Modal>

      <Card
        className="settings-tab-panel"
        style={{ display: settingsTab === "runtime" ? undefined : "none" }}
        extra={<Button onClick={runtimeSettings.reload}>刷新</Button>}
        loading={runtimeSettings.loading}
      >
        <Space direction="vertical" size="large" className="wide-space">
          <Form form={runtimeForm} layout="vertical">
            <div className="settings-section-grid">
              <Card size="small" title="模型与渲染并发" className="settings-inner-card">
                <div className="settings-form-grid">
                  <Form.Item
                    name="llm_max_concurrency"
                    label="LLM 并发数"
                    extra="当前已不再限制 LLM 并发，保留字段用于兼容旧配置。"
                    rules={[{ required: true, message: "请输入 LLM 并发数" }]}
                  >
                    <InputNumber min={1} style={{ width: "100%" }} />
                  </Form.Item>
                  <Form.Item
                    name="vlm_max_concurrency"
                    label="VLM 并发数"
                    extra="当前已不再限制 VLM 并发，保留字段用于兼容旧配置。"
                    rules={[{ required: true, message: "请输入 VLM 并发数" }]}
                  >
                    <InputNumber min={1} style={{ width: "100%" }} />
                  </Form.Item>
                  <Form.Item
                    name="render_max_concurrency"
                    label="渲染并发数"
                    extra="控制服务端 ffmpeg 成片渲染的并发上限。"
                    rules={[{ required: true, message: "请输入渲染并发数" }]}
                  >
                    <InputNumber min={1} style={{ width: "100%" }} />
                  </Form.Item>
                </div>
              </Card>

              <Card size="small" title="音频任务并发" className="settings-inner-card">
                <div className="settings-form-grid">
                  <Form.Item
                    name="asr_max_concurrency"
                    label="ASR 并发数"
                    extra="控制语音识别任务并发上限。"
                    rules={[{ required: true, message: "请输入 ASR 并发数" }]}
                  >
                    <InputNumber min={1} style={{ width: "100%" }} />
                  </Form.Item>
                  <Form.Item
                    name="tts_max_concurrency"
                    label="TTS 并发数"
                    extra="控制配音生成任务并发上限。"
                    rules={[{ required: true, message: "请输入 TTS 并发数" }]}
                  >
                    <InputNumber min={1} style={{ width: "100%" }} />
                  </Form.Item>
                </div>
              </Card>

              <Card size="small" title="任务队列保护" className="settings-inner-card">
                <div className="settings-form-grid">
                  <Form.Item
                    name="task_max_queued_per_user"
                    label="单用户最大排队任务"
                    extra="防止单个用户一次性提交过多批量任务。"
                    rules={[{ required: true, message: "请输入单用户最大排队任务" }]}
                  >
                    <InputNumber min={1} style={{ width: "100%" }} />
                  </Form.Item>
                  <Form.Item
                    name="task_max_running_per_user"
                    label="单用户最大运行任务"
                    extra="控制同一用户同时执行的任务数量。"
                    rules={[{ required: true, message: "请输入单用户最大运行任务" }]}
                  >
                    <InputNumber min={1} style={{ width: "100%" }} />
                  </Form.Item>
                </div>
              </Card>

              <Card size="small" title="VLM 请求参数" className="settings-inner-card">
                <div className="settings-form-grid">
                  <Form.Item
                    name="vlm_timeout_seconds"
                    label="VLM 超时秒数"
                    extra="单次 VLM 请求的超时时间。"
                    rules={[{ required: true, message: "请输入 VLM 超时秒数" }]}
                  >
                    <InputNumber min={1} style={{ width: "100%" }} />
                  </Form.Item>
                  <Form.Item
                    name="vlm_max_retries"
                    label="VLM 最大重试次数"
                    extra="请求失败后的自动重试次数。"
                    rules={[{ required: true, message: "请输入 VLM 最大重试次数" }]}
                  >
                    <InputNumber min={0} style={{ width: "100%" }} />
                  </Form.Item>
                </div>
              </Card>
            </div>

            <Button type="primary" loading={savingRuntime} onClick={() => void saveRuntimeSettings()}>
              保存运行控制
            </Button>
          </Form>
        </Space>
      </Card>
    </Space>
  );
}

function ConsoleApp({ session, onLogout }: { session: Session; onLogout: () => void }) {
  const [view, setView] = useState<ViewKey>(() => readHashView(session.user.role));
  const menuItems = [
    { key: "products", label: "产品" },
    { key: "preprocess", label: "预处理" },
    { key: "assets", label: "素材" },
    { key: "tasks", label: "任务" },
    ...(session.user.role === "admin" ? [{ key: "settings", label: "设置" }] : [])
  ];

  useEffect(() => {
    const syncViewFromHash = () => {
      const nextView = readHashView(session.user.role);
      setView(nextView);
      if (window.location.hash !== `#/${nextView}`) {
        writeHashView(nextView);
      }
    };

    syncViewFromHash();
    window.addEventListener("hashchange", syncViewFromHash);
    return () => window.removeEventListener("hashchange", syncViewFromHash);
  }, [session.user.role]);

  const navigateView = (next: ViewKey) => {
    const normalized = normalizeViewForRole(next, session.user.role);
    setView(normalized);
    writeHashView(normalized);
  };

  return (
    <Layout className="app-shell" data-testid="console-app">
      <Layout.Sider width={220} theme="light">
        <div className="brand">AICut</div>
        <Menu selectedKeys={[view]} items={menuItems} onClick={(item) => navigateView(item.key as ViewKey)} />
      </Layout.Sider>
      <Layout>
        <Layout.Header className="topbar">
          <Space>
            <Tag color={session.user.role === "admin" ? "blue" : "default"}>
              {translateValue(session.user.role, roleLabels)}
            </Tag>
            <Typography.Text>{session.user.display_name}</Typography.Text>
            <Button onClick={onLogout}>退出登录</Button>
          </Space>
        </Layout.Header>
        <Layout.Content className="content">
          {view === "products" && <ProductManagementPage token={session.token} />}
          {view === "preprocess" && <PreprocessPage token={session.token} />}
          {view === "assets" && <AssetsPage token={session.token} />}
          {view === "tasks" && <TasksPage token={session.token} />}
          {view === "settings" && session.user.role === "admin" && <SettingsPage token={session.token} />}
        </Layout.Content>
      </Layout>
    </Layout>
  );
}

function App() {
  const [session, setSession] = useState<Session | null>(() => readStoredSession());

  const handleLogin = (nextSession: Session) => {
    storeSession(nextSession);
    setSession(nextSession);
    const nextView = readHashView(nextSession.user.role);
    writeHashView(nextView);
  };

  const handleLogout = () => {
    clearStoredSession();
    setSession(null);
  };

  return session ? (
    <ConsoleApp session={session} onLogout={handleLogout} />
  ) : (
    <LoginPage onLogin={handleLogin} />
  );
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN}>
      <App />
    </ConfigProvider>
  </React.StrictMode>
);
