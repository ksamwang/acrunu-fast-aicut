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
import { SettingsPage } from "./features/settings/SettingsPage";
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
