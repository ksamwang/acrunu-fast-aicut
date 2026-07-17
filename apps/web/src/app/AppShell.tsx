import { Button, Layout, Menu, Space, Tag, Typography } from "antd";
import { Clapperboard, Library, Music2, Package, Scissors, Settings, UsersRound, WandSparkles } from "lucide-react";
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
    { key: "workbench", icon: <WandSparkles size={17} />, label: "工作台" },
    { key: "finished", icon: <Clapperboard size={17} />, label: "成品库" },
    { key: "products", icon: <Package size={17} />, label: "产品" },
    { key: "preprocess", icon: <Scissors size={17} />, label: "预处理" },
    { key: "assets", icon: <Library size={17} />, label: "素材" },
    { key: "bgm", icon: <Music2 size={17} />, label: "音乐库" },
    ...(user.role === "admin"
      ? [
          { key: "settings", icon: <Settings size={17} />, label: "设置" },
          { key: "users", icon: <UsersRound size={17} />, label: "用户管理" }
        ]
      : [])
  ];

  return (
    <Layout className="app-shell" data-testid="console-app">
      <Layout.Sider className="app-sider" width={220} theme="light">
        <div className="brand">AICut</div>
        <Menu className="app-menu" selectedKeys={[view]} items={menuItems} onClick={(item) => onNavigate(item.key as ViewKey)} />
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
