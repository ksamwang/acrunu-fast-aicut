import { useState } from "react";
import { Button, Card, Form, Input, message } from "antd";
import { apiRequest } from "../../shared/api/server-api";
import type { Session } from "../../shared/types/auth";

export function LoginPage({ onLogin }: { onLogin: (session: Session) => void }) {
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

