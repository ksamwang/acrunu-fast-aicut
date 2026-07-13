import { useEffect, useMemo, useState } from "react";
import { Alert, Button, Card, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Table, Tabs, Tag, Typography, message } from "antd";
import { apiRequest } from "../../shared/api/server-api";
import { useResource } from "../../shared/hooks/use-resource";
import type { ModelCapabilitySettings, ModelDiscoveryResult, ModelProvider, ModelSelectOption, RuntimeSettings } from "../../shared/types/settings";
import "./styles.css";

export function SettingsPage({ token }: { token: string }) {
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
