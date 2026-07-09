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

function ProductsPage({ token }: { token: string }) {
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
              { title: "素材", render: (_, asset) => asset.asset_name || asset.file_name },
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
    tag: "",
    keyword: "",
    minDurationMs: "",
    maxDurationMs: "",
    hasAudio: "",
    excludeDiscarded: "",
    sortBy: ""
  });
  const [assetPage, setAssetPage] = useState(1);
  const [assetPageSize, setAssetPageSize] = useState(20);
  const [frames, setFrames] = useState<AssetFrameSnapshot[]>([]);
  const [assetSellingPoints, setAssetSellingPoints] = useState<SellingPoint[]>([]);
  const [framesLoading, setFramesLoading] = useState(false);
  const [editingAnalysis, setEditingAnalysis] = useState(false);
  const [savingAnalysis, setSavingAnalysis] = useState(false);
  const [updatingArchive, setUpdatingArchive] = useState(false);
  const [savingSellingPoints, setSavingSellingPoints] = useState(false);
  const [reviewForm] = Form.useForm<AssetReviewPayload>();
  const [sellingPointForm] = Form.useForm<AssetSellingPointPayload>();

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

  return (
    <div data-testid="assets-page">
      <Space direction="vertical" size="middle" className="page-stack">
        <Typography.Title level={3}>素材库</Typography.Title>
        <Card title="本地代理入口">
          <Space direction="vertical" className="wide-space">
            <Input defaultValue="http://127.0.0.1:58721" />
            <Button type="primary">打开本地代理</Button>
          </Space>
        </Card>
        <Card title="筛选条件">
          <Space wrap>
            <Select
              data-testid="asset-filter-product"
              value={filters.productID || undefined}
              placeholder="产品"
              allowClear
              style={{ minWidth: 180 }}
              options={(products.data ?? []).map((product) => ({ value: product.id, label: product.name }))}
              onChange={(value) => {
                const nextValue = value ?? "";
                setAssetPage(1);
                setFilters((current) => ({ ...current, productID: nextValue, sellingPointID: "" }));
                setProductForSellingPoints(nextValue);
              }}
            />
            <Select
              data-testid="asset-filter-selling-point"
              value={filters.sellingPointID || undefined}
              placeholder="卖点"
              allowClear
              style={{ minWidth: 180 }}
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
              style={{ minWidth: 160 }}
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
              style={{ minWidth: 160 }}
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
            <Input
              data-testid="asset-filter-tag"
              value={filters.tag}
              placeholder="标签"
              style={{ width: 180 }}
              onChange={(event) => {
                setAssetPage(1);
                setFilters((current) => ({ ...current, tag: event.target.value }));
              }}
            />
            <Input
              data-testid="asset-filter-keyword"
              value={filters.keyword}
              placeholder="画面关键词"
              style={{ width: 180 }}
              onChange={(event) => {
                setAssetPage(1);
                setFilters((current) => ({ ...current, keyword: event.target.value }));
              }}
            />
            <Input
              data-testid="asset-filter-min-duration"
              value={filters.minDurationMs}
              placeholder="最小时长毫秒"
              style={{ width: 120 }}
              onChange={(event) => {
                setAssetPage(1);
                setFilters((current) => ({ ...current, minDurationMs: event.target.value }));
              }}
            />
            <Input
              data-testid="asset-filter-max-duration"
              value={filters.maxDurationMs}
              placeholder="最大时长毫秒"
              style={{ width: 120 }}
              onChange={(event) => {
                setAssetPage(1);
                setFilters((current) => ({ ...current, maxDurationMs: event.target.value }));
              }}
            />
            <Select
              data-testid="asset-filter-has-audio"
              value={filters.hasAudio || undefined}
              placeholder="是否含音频"
              allowClear
              style={{ minWidth: 140 }}
              options={[
                { value: "true", label: "是" },
                { value: "false", label: "否" }
              ]}
              onChange={(value) => {
                setAssetPage(1);
                setFilters((current) => ({ ...current, hasAudio: value ?? "" }));
              }}
            />
            <Select
              data-testid="asset-filter-exclude-discarded"
              value={filters.excludeDiscarded || undefined}
              placeholder="可用性"
              allowClear
              style={{ minWidth: 160 }}
              options={[
                { value: "true", label: "排除废弃" }
              ]}
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
              style={{ minWidth: 180 }}
              options={[
                { value: "updated_at_desc", label: "最近更新时间" },
                { value: "analyzed_at_desc", label: "最近分析时间" }
              ]}
              onChange={(value) => {
                setAssetPage(1);
                setFilters((current) => ({ ...current, sortBy: value ?? "" }));
              }}
            />
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
                  tag: "",
                  keyword: "",
                  minDurationMs: "",
                  maxDurationMs: "",
                  hasAudio: "",
                  excludeDiscarded: "",
                  sortBy: ""
                });
                setProductForSellingPoints("");
              }}
            >
              重置
            </Button>
            <Button data-testid="asset-filter-refresh" onClick={assets.reload}>刷新</Button>
          </Space>
        </Card>
        <Card title="素材列表">
          <Table<Asset>
            rowKey="id"
            loading={assets.loading}
            dataSource={assets.data?.items ?? []}
            pagination={{
              current: assets.data?.page ?? assetPage,
              pageSize: assets.data?.page_size ?? assetPageSize,
              total: assets.data?.total ?? 0,
              showSizeChanger: true,
              pageSizeOptions: ["10", "20", "50"],
              onChange: (page, pageSize) => {
                setAssetPage(page);
                setAssetPageSize(pageSize);
              }
            }}
            onRow={(record) => ({ onClick: () => setSelectedAsset(record) })}
            columns={[
              {
                title: "素材",
                render: (_, asset) => (
                  <Button type="link" className="table-link-button" onClick={() => setSelectedAsset(asset)}>
                    {asset.asset_name || asset.file_name}
                  </Button>
                )
              },
              { title: "文件", dataIndex: "file_name" },
              {
                title: "产品",
                render: (_, asset) => productNameByID.get(asset.product_id) ?? asset.product_id ?? "-"
              },
              { title: "类型", dataIndex: "source_type", render: (value) => translateValue(value, sourceTypeLabels) },
              { title: "状态", dataIndex: "status", render: (status) => <Tag>{translateValue(status, assetStatusLabels)}</Tag> },
              {
                title: "分析状态",
                dataIndex: "analysis_status",
                render: (status) => (status ? <Tag color="blue">{translateValue(status, analysisStatusLabels)}</Tag> : "-")
              },
              { title: "时长", render: (_, asset) => formatDuration(asset.duration_ms) },
              {
                title: "景别",
                dataIndex: "shot_size",
                render: (value) => translateValue(value, shotSizeLabels)
              },
              {
                title: "运镜",
                dataIndex: "camera_movement",
                render: (value) => translateValue(value, cameraMovementLabels)
              },
              {
                title: "标签",
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
                title: "分辨率",
                render: (_, asset) => (asset.width && asset.height ? `${asset.width}x${asset.height}` : "-")
              }
            ]}
          />
        </Card>
      </Space>

      <Modal
        title={selectedAsset ? `素材详情：${selectedAsset.asset_name || selectedAsset.file_name}` : "素材详情"}
        open={selectedAsset !== null}
        footer={null}
        width={960}
        onCancel={() => setSelectedAsset(null)}
      >
        {selectedAsset ? (
          <Space direction="vertical" size="large" className="wide-space" data-testid="asset-detail-modal">
            <Descriptions bordered column={2} size="small">
              <Descriptions.Item label="文件">{selectedAsset.file_name}</Descriptions.Item>
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

            <Card title="分析摘要">
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

                {editingAnalysis ? (
                  <Form form={reviewForm} layout="vertical" data-testid="asset-review-form">
                    <Form.Item name="scene_description" label="画面描述">
                      <Input.TextArea rows={3} />
                    </Form.Item>
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
                  <Descriptions bordered column={1} size="small" data-testid="asset-analysis-panel">
                    <Descriptions.Item label="画面描述">
                      {selectedAsset.scene_description || <Typography.Text type="secondary">暂无分析结果。</Typography.Text>}
                    </Descriptions.Item>
                    <Descriptions.Item label="景别">
                      {translateValue(selectedAsset.shot_size, shotSizeLabels)}
                    </Descriptions.Item>
                    <Descriptions.Item label="运镜">
                      {translateValue(selectedAsset.camera_movement, cameraMovementLabels)}
                    </Descriptions.Item>
                    <Descriptions.Item label="主体标签">
                      {renderTagList(selectedAsset.subjects, "暂无主体标签")}
                    </Descriptions.Item>
                    <Descriptions.Item label="场景标签">
                      {renderTagList(selectedAsset.scene_tags, "暂无场景标签")}
                    </Descriptions.Item>
                    <Descriptions.Item label="质量标签">
                      {renderTagList(selectedAsset.quality_tags, "暂无质量问题")}
                    </Descriptions.Item>
                    <Descriptions.Item label="复核备注">
                      {selectedAsset.reviewer_notes || <Typography.Text type="secondary">无</Typography.Text>}
                    </Descriptions.Item>
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
                )}
              </Space>
            </Card>

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
          </Space>
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

function SettingsPage({ token }: { token: string }) {
  const configs = useResource<SystemConfig[]>("/api/admin/system-configs", token);

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>系统设置</Typography.Title>
      <Card title="模型与并发配置" extra={<Button onClick={configs.reload}>刷新</Button>}>
        <Table<SystemConfig>
          rowKey="key"
          loading={configs.loading}
          dataSource={configs.data ?? []}
          columns={[
            { title: "键", dataIndex: "key" },
            { title: "值", dataIndex: "value", render: (value) => JSON.stringify(value) },
            { title: "类型", dataIndex: "type" },
            { title: "说明", dataIndex: "description" }
          ]}
        />
      </Card>
    </Space>
  );
}

function ConsoleApp({ session, onLogout }: { session: Session; onLogout: () => void }) {
  const [view, setView] = useState<ViewKey>("products");
  const menuItems = [
    { key: "products", label: "产品" },
    { key: "assets", label: "素材" },
    { key: "tasks", label: "任务" },
    ...(session.user.role === "admin" ? [{ key: "settings", label: "设置" }] : [])
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
            <Tag color={session.user.role === "admin" ? "blue" : "default"}>
              {translateValue(session.user.role, roleLabels)}
            </Tag>
            <Typography.Text>{session.user.display_name}</Typography.Text>
            <Button onClick={onLogout}>退出登录</Button>
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
