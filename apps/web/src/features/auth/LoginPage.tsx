import { useState } from "react";
import { Button, Form, Input } from "antd";
import { ArrowRight, LockKeyhole, Scissors, UserRound } from "lucide-react";
import { apiRequest } from "../../shared/api/server-api";
import type { Session } from "../../shared/types/auth";
import { ArchitectureFlow } from "./ArchitectureFlow";
import "./styles.css";

export function LoginPage({ onLogin }: { onLogin: (session: Session) => void }) {
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");

  return (
    <div className="login-shell" data-testid="login-page">
      <section className="login-visual" aria-labelledby="login-brand-title">
        <header className="login-brand-lockup">
          <span className="login-brand-mark"><Scissors size={20} /></span>
          <div>
            <h1 id="login-brand-title">ACRUNU Fast Cut</h1>
            <p>艾锐伦快剪辑系统</p>
          </div>
        </header>
        <ArchitectureFlow />
      </section>

      <aside className="login-panel">
        <div className="login-panel-status">
          <span>ACRUNU / DESKTOP WORKSPACE</span>
          <i>SECURE</i>
        </div>
        <div className="login-form-wrap">
          <div className="login-form-heading">
            <span>ACCOUNT ACCESS</span>
            <h2>登录系统</h2>
          </div>
          <Form
            className="login-form"
            layout="vertical"
            autoComplete="off"
            onFinish={async (values) => {
              setLoading(true);
              setErrorMessage("");
              try {
                const session = await apiRequest<Session>("/api/auth/login", {
                  method: "POST",
                  body: JSON.stringify(values)
                });
                onLogin(session);
              } catch (error) {
                setErrorMessage(error instanceof Error ? error.message : "登录失败，请检查账户信息");
              } finally {
                setLoading(false);
              }
            }}
          >
            <Form.Item name="username" label="用户名" rules={[{ required: true, message: "请输入用户名" }]}>
              <Input prefix={<UserRound size={17} />} autoComplete="one-time-code" placeholder="请输入用户名" />
            </Form.Item>
            <Form.Item name="password" label="密码" rules={[{ required: true, message: "请输入密码" }]}>
              <Input.Password prefix={<LockKeyhole size={17} />} autoComplete="new-password" placeholder="请输入密码" />
            </Form.Item>
            {errorMessage ? <div className="login-error" role="alert">{errorMessage}</div> : null}
            <Button type="primary" htmlType="submit" loading={loading} block data-testid="login-submit" icon={<ArrowRight size={17} />} iconPosition="end">
              进入系统
            </Button>
          </Form>
        </div>
        <footer className="login-panel-footer">
          <span>ACRUNU Fast Cut</span>
          <span>SECURE WORKSPACE</span>
        </footer>
      </aside>
    </div>
  );
}
