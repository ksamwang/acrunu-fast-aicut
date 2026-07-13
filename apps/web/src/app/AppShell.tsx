import { Button, Layout, Menu, Space, Tag, Typography } from "antd";
import type { ReactNode } from "react";
import type { User } from "../shared/types/auth";
import type { ViewKey } from "./routes";

type AppShellProps = {
  user: User;
  view: ViewKey;
  roleLabel: string;
  onNavigate: (view: ViewKey) => void;
  onLogout: () => void;
  children: ReactNode;
};

export function AppShell({ user, view, roleLabel, onNavigate, onLogout, children }: AppShellProps) {
  const menuItems = [
    { key: "products", label: "产品" },
    { key: "preprocess", label: "预处理" },
    { key: "assets", label: "素材" },
    { key: "tasks", label: "任务" },
    ...(user.role === "admin" ? [{ key: "settings", label: "设置" }] : [])
  ];

  return (
    <Layout className="app-shell" data-testid="console-app">
      <Layout.Sider width={220} theme="light">
        <div className="brand">AICut</div>
        <Menu selectedKeys={[view]} items={menuItems} onClick={(item) => onNavigate(item.key as ViewKey)} />
      </Layout.Sider>
      <Layout>
        <Layout.Header className="topbar">
          <Space>
            <Tag color={user.role === "admin" ? "blue" : "default"}>{roleLabel}</Tag>
            <Typography.Text>{user.display_name}</Typography.Text>
            <Button onClick={onLogout}>退出登录</Button>
          </Space>
        </Layout.Header>
        <Layout.Content className="content">{children}</Layout.Content>
      </Layout>
    </Layout>
  );
}
