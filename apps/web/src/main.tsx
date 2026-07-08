import React, { useEffect, useState } from "react";
import ReactDOM from "react-dom/client";
import {
  Button,
  Card,
  ConfigProvider,
  Form,
  Input,
  InputNumber,
  Layout,
  Menu,
  Modal,
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
  file_name: string;
  source_type: string;
  status: string;
  duration_ms?: number;
  width?: number;
  height?: number;
};

type Task = {
  id: string;
  task_type: string;
  status: string;
  error_message?: string;
  retry_count: number;
  created_at: string;
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
    throw new Error(payload?.error?.message ?? "请求失败");
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
      message.error(error instanceof Error ? error.message : "加载失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
  }, deps);

  return { data, loading, reload };
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
              message.error(error instanceof Error ? error.message : "登录失败");
            } finally {
              setLoading(false);
            }
          }}
        >
          <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true }]}>
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

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>产品管理</Typography.Title>
      <Card title="产品列表" extra={<Button type="primary" onClick={() => setProductOpen(true)}>新建产品</Button>}>
        <Table<Product>
          rowKey="id"
          loading={products.loading}
          dataSource={products.data ?? []}
          onRow={(record) => ({ onClick: () => setSelectedProductID(record.id) })}
          rowClassName={(record) => (record.id === selectedProductID ? "selected-row" : "")}
          columns={[
            { title: "产品", dataIndex: "name" },
            { title: "分类", dataIndex: "category" },
            { title: "状态", dataIndex: "status", render: (status) => <Tag>{status}</Tag> }
          ]}
        />
      </Card>
      <Card
        title="卖点管理"
        extra={<Button disabled={!selectedProductID} onClick={() => setSellingPointOpen(true)}>新建卖点</Button>}
      >
        <Table<SellingPoint>
          rowKey="id"
          loading={sellingPoints.loading}
          dataSource={selectedProductID ? sellingPoints.data ?? [] : []}
          columns={[
            { title: "卖点", dataIndex: "title" },
            { title: "优先级", dataIndex: "priority" },
            { title: "状态", dataIndex: "status", render: (status) => <Tag>{status}</Tag> }
          ]}
        />
      </Card>

      <Modal title="新建产品" open={productOpen} onOk={createProduct} onCancel={() => setProductOpen(false)}>
        <Form form={productForm} layout="vertical">
          <Form.Item name="name" label="产品名称" rules={[{ required: true }]}>
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

      <Modal title="新建卖点" open={sellingPointOpen} onOk={createSellingPoint} onCancel={() => setSellingPointOpen(false)}>
        <Form form={sellingPointForm} layout="vertical">
          <Form.Item name="title" label="卖点" rules={[{ required: true }]}>
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
  const assets = useResource<Asset[]>("/api/assets", token);

  return (
    <div data-testid="assets-page">
      <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>共享素材库</Typography.Title>
      <Card title="本地 Agent 上传入口">
        <Space direction="vertical" className="wide-space">
          <Input defaultValue="http://127.0.0.1:58721" />
          <Button type="primary">选择本地素材并预处理</Button>
        </Space>
      </Card>
      <Card title="素材列表" extra={<Button onClick={assets.reload}>刷新</Button>}>
        <Table<Asset>
          rowKey="id"
          loading={assets.loading}
          dataSource={assets.data ?? []}
          columns={[
            { title: "文件", dataIndex: "file_name" },
            { title: "类型", dataIndex: "source_type" },
            { title: "状态", dataIndex: "status", render: (status) => <Tag>{status}</Tag> },
            { title: "时长", dataIndex: "duration_ms" },
            { title: "尺寸", render: (_, item) => (item.width && item.height ? `${item.width}x${item.height}` : "-") }
          ]}
        />
      </Card>
      </Space>
    </div>
  );
}

function TasksPage({ token }: { token: string }) {
  const tasks = useResource<Task[]>("/api/tasks", token);
  const [creating, setCreating] = useState(false);

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

  return (
    <div data-testid="tasks-page">
      <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>任务列表</Typography.Title>
      <Card title="批量剪辑任务" extra={<Button type="primary" loading={creating} onClick={createTask}>创建测试任务</Button>}>
        <Table<Task>
          rowKey="id"
          loading={tasks.loading}
          dataSource={tasks.data ?? []}
          columns={[
            { title: "任务", dataIndex: "id" },
            { title: "类型", dataIndex: "task_type" },
            { title: "状态", dataIndex: "status", render: (status) => <Tag>{status}</Tag> },
            { title: "重试", dataIndex: "retry_count" },
            { title: "创建时间", dataIndex: "created_at" }
          ]}
        />
      </Card>
      </Space>
    </div>
  );
}

function SettingsPage({ token }: { token: string }) {
  const configs = useResource<SystemConfig[]>("/api/admin/system-configs", token);

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>系统配置</Typography.Title>
      <Card title="模型与并发配置" extra={<Button onClick={configs.reload}>刷新</Button>}>
        <Table<SystemConfig>
          rowKey="key"
          loading={configs.loading}
          dataSource={configs.data ?? []}
          columns={[
            { title: "配置项", dataIndex: "key" },
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
    ...(session.user.role === "admin" ? [{ key: "settings", label: "系统配置" }] : [])
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
            <Button onClick={onLogout}>退出</Button>
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
