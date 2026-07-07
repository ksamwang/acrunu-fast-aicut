import React, { useState } from "react";
import ReactDOM from "react-dom/client";
import { Button, Card, ConfigProvider, Form, Input, Layout, Menu, Space, Table, Tag, Typography, message } from "antd";
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

type ViewKey = "products" | "assets" | "tasks" | "settings";

const apiBase = "";

async function apiRequest<T>(path: string, options: RequestInit = {}, token?: string): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
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

function LoginPage({ onLogin }: { onLogin: (session: Session) => void }) {
  const [loading, setLoading] = useState(false);

  return (
    <div className="login-shell">
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
          <Button type="primary" htmlType="submit" loading={loading} block>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  );
}

function ProductsPage() {
  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>产品管理</Typography.Title>
      <Card title="产品列表" extra={<Button type="primary">新建产品</Button>}>
        <Table
          rowKey="id"
          dataSource={[]}
          columns={[
            { title: "产品", dataIndex: "name" },
            { title: "分类", dataIndex: "category" },
            { title: "状态", dataIndex: "status" }
          ]}
        />
      </Card>
      <Card title="卖点管理">
        <Table
          rowKey="id"
          dataSource={[]}
          columns={[
            { title: "卖点", dataIndex: "title" },
            { title: "优先级", dataIndex: "priority" },
            { title: "状态", dataIndex: "status" }
          ]}
        />
      </Card>
    </Space>
  );
}

function AssetsPage() {
  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>共享素材库</Typography.Title>
      <Card title="本地 Agent 上传入口">
        <Space direction="vertical">
          <Input placeholder="Local Agent 地址，例如 http://127.0.0.1:58721" />
          <Button type="primary">选择本地素材并预处理</Button>
        </Space>
      </Card>
      <Card title="素材列表">
        <Table
          rowKey="id"
          dataSource={[]}
          columns={[
            { title: "文件", dataIndex: "file_name" },
            { title: "类型", dataIndex: "source_type" },
            { title: "状态", dataIndex: "status" },
            { title: "时长", dataIndex: "duration_ms" }
          ]}
        />
      </Card>
    </Space>
  );
}

function TasksPage() {
  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>任务列表</Typography.Title>
      <Card title="批量剪辑任务" extra={<Button type="primary">创建测试任务</Button>}>
        <Table
          rowKey="id"
          dataSource={[]}
          columns={[
            { title: "任务", dataIndex: "id" },
            { title: "类型", dataIndex: "task_type" },
            { title: "状态", dataIndex: "status" },
            { title: "创建时间", dataIndex: "created_at" }
          ]}
        />
      </Card>
    </Space>
  );
}

function SettingsPage() {
  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>系统配置</Typography.Title>
      <Card title="模型与并发配置">
        <Table
          rowKey="key"
          dataSource={[]}
          columns={[
            { title: "配置项", dataIndex: "key" },
            { title: "值", dataIndex: "value" },
            { title: "类型", dataIndex: "type" }
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
    <Layout className="app-shell">
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
          {view === "products" && <ProductsPage />}
          {view === "assets" && <AssetsPage />}
          {view === "tasks" && <TasksPage />}
          {view === "settings" && session.user.role === "admin" && <SettingsPage />}
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
