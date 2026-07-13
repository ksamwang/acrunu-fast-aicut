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
import { AppShell } from "./app/AppShell";
import { normalizeViewForRole, readHashView, writeHashView, type ViewKey } from "./app/routes";
import { AssetsPage } from "./features/assets/AssetsPage";
import { PreprocessPage } from "./features/preprocess/PreprocessPage";
import { ProductManagementPage } from "./features/products/ProductsPage";
import { TasksPage } from "./features/tasks/TasksPage";
import { apiRequest } from "./shared/api/server-api";
import { formatDateTime, formatDuration, formatTimestamp } from "./shared/lib/format";
import { clearStoredSession, readStoredSession, storeSession } from "./shared/lib/session-storage";
import type { Asset, AssetEmbeddingListResponse, AssetEmbeddingObject, AssetEmbeddingRunResult, AssetEmbeddingTarget, AssetFrameResponse, AssetFrameSnapshot, AssetListResponse, AssetReviewPayload, AssetSemanticPreview, AssetSellingPointPayload, AssetSpeechSegment } from "./shared/types/asset";
import type { Session, User } from "./shared/types/auth";
import type { Product, ProductStats, SellingPoint } from "./shared/types/product";
import type { ModelCapabilitySettings, ModelDiscoveryResult, ModelProvider, ModelSelectOption, OpenAICompatibleSettings, RuntimeSettings, SystemConfig } from "./shared/types/settings";
import type { Task } from "./shared/types/task";
import "./styles.css";

function productReferenceImage(product?: Product | null) {
  const image = product?.metadata?.reference_image;
  return typeof image === "string" ? image : "";
}

function readImageFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    if (!file.type.startsWith("image/")) {
      reject(new Error("请选择图片文件"));
      return;
    }
    if (file.size > 2 * 1024 * 1024) {
      reject(new Error("产品参考图不能超过 2MB"));
      return;
    }
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(new Error("读取图片失败"));
    reader.readAsDataURL(file);
  });
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

const taskStatusLabels: Record<string, string> = {
  queued: "排队中",
  running: "执行中",
  completed: "已完成",
  failed: "失败"
};

const taskTypeLabels: Record<string, string> = {
  asset_analyze: "素材分析",
  asset_embedding: "素材向量化",
  asset_extract_frames: "素材抽帧",
  test: "测试任务"
};

function translateValue(value: string | undefined | null, labels: Record<string, string>) {
  if (!value) {
    return "-";
  }
  return labels[value] ?? value;
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
      embedding_model: settings.embedding.model,
      embedding_dimension: settings.embedding.dimension ?? 1024
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
            embedding: { provider_id: values.embedding_provider_id, model: values.embedding_model, dimension: values.embedding_dimension }
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
    { key: "llm", title: "LLM", providerField: "llm_provider_id", modelField: "llm_model", selectedProviderID: selectedLLMProviderID, description: "用于文案生成、编排等文本推理任务。", manualModelInput: false },
    { key: "vlm", title: "VLM", providerField: "vlm_provider_id", modelField: "vlm_model", selectedProviderID: selectedVLMProviderID, description: "用于素材抽帧分析、画面理解和标签提取。", manualModelInput: false },
    { key: "embedding", title: "向量模型", providerField: "embedding_provider_id", modelField: "embedding_model", selectedProviderID: selectedEmbeddingProviderID, description: "用于素材语义文本向量化和后续 pgvector 检索。", manualModelInput: true }
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
                    <Form.Item name={row.modelField} label="模型" rules={[{ required: true, message: row.manualModelInput ? "请输入模型 ID" : "请选择模型" }]}>
                      {row.manualModelInput ? (
                        <Input placeholder="请输入向量模型 ID，例如 text-embedding-v3" />
                      ) : (
                        <Select
                          showSearch
                          placeholder={row.selectedProviderID ? "选择模型" : "请先选择供应商"}
                          options={modelOptions}
                          disabled={!row.selectedProviderID || modelOptions.length === 0}
                          filterOption={(input, option) => String(option?.label ?? "").toLowerCase().includes(input.toLowerCase())}
                        />
                      )}
                    </Form.Item>
                    {row.key === "embedding" && (
                      <Form.Item name="embedding_dimension" label="维度" rules={[{ required: true, message: "请输入向量维度" }]}>
                        <InputNumber min={1} placeholder="1024" style={{ width: "100%" }} />
                      </Form.Item>
                    )}
                  </div>
                  {!row.manualModelInput && (
                    <Button size="small" disabled={!row.selectedProviderID} loading={loadingModels} onClick={() => void loadModels(row.selectedProviderID, true)}>
                      重新拉取该供应商模型
                    </Button>
                  )}
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
    <AppShell
      user={session.user}
      view={view}
      roleLabel={translateValue(session.user.role, roleLabels)}
      onNavigate={navigateView}
      onLogout={onLogout}
    >
      {view === "products" && <ProductManagementPage token={session.token} />}
      {view === "preprocess" && <PreprocessPage token={session.token} />}
      {view === "assets" && <AssetsPage token={session.token} />}
      {view === "tasks" && <TasksPage token={session.token} />}
      {view === "settings" && session.user.role === "admin" && <SettingsPage token={session.token} />}
    </AppShell>
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
