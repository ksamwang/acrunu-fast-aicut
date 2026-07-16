import { useState } from "react";
import { Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Tooltip, Typography, message } from "antd";
import { Pencil, Trash2, UserPlus } from "lucide-react";
import { useResource } from "../../shared/hooks/use-resource";
import { formatDateTime } from "../../shared/lib/format";
import type { User } from "../../shared/types/auth";
import type { ManagedUser, UserMutation } from "../../shared/types/managed-user";
import { createUser, deleteUser, listUsers, updateUser } from "./api";
import "./styles.css";

type UserFormValues = {
  username: string;
  display_name: string;
  email?: string;
  password?: string;
  role: User["role"];
};

export function UsersPage({ token, currentUser }: { token: string; currentUser: User }) {
  const users = useResource<ManagedUser[]>("/api/admin/users", token, [], listUsers);
  const [form] = Form.useForm<UserFormValues>();
  const [editingUser, setEditingUser] = useState<ManagedUser | null>(null);
  const [editorOpen, setEditorOpen] = useState(false);
  const [saving, setSaving] = useState(false);

  const openCreate = () => {
    setEditingUser(null);
    form.resetFields();
    form.setFieldsValue({ role: "user" });
    setEditorOpen(true);
  };

  const openEdit = (user: ManagedUser) => {
    setEditingUser(user);
    form.setFieldsValue({
      username: user.username,
      display_name: user.display_name,
      email: user.email,
      password: "",
      role: user.role
    });
    setEditorOpen(true);
  };

  const closeEditor = () => {
    setEditorOpen(false);
    setEditingUser(null);
    form.resetFields();
  };

  const saveUser = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      const payload: UserMutation = {
        username: values.username,
        display_name: values.display_name,
        email: values.email?.trim() || undefined,
        password: values.password || undefined,
        role: values.role
      };
      if (editingUser) {
        await updateUser(editingUser.id, payload, token);
      } else {
        await createUser(payload, token);
      }
      closeEditor();
      await users.reload();
      message.success(editingUser ? "用户已更新" : "用户已创建");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存用户失败");
    } finally {
      setSaving(false);
    }
  };

  const removeUser = async (user: ManagedUser) => {
    try {
      await deleteUser(user.id, token);
      await users.reload();
      message.success("用户已删除");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "删除用户失败");
    }
  };

  return (
    <div className="users-page" data-testid="users-page">
      <Card
        className="users-list-card"
        title="用户管理"
        extra={
          <Button type="primary" icon={<UserPlus size={16} />} onClick={openCreate}>
            新增用户
          </Button>
        }
      >
        <Table<ManagedUser>
          rowKey="id"
          loading={users.loading}
          dataSource={users.data ?? []}
          pagination={{ pageSize: 20, showSizeChanger: true, showTotal: (total) => `共 ${total} 位用户` }}
          locale={{ emptyText: "暂无用户" }}
          columns={[
            {
              title: "用户",
              dataIndex: "display_name",
              render: (value: string, record) => (
                <Space direction="vertical" size={0}>
                  <Typography.Text strong>{value}</Typography.Text>
                  <Typography.Text type="secondary">{record.username}</Typography.Text>
                </Space>
              )
            },
            { title: "邮箱", dataIndex: "email", render: (value: string | undefined) => value || "-" },
            {
              title: "角色",
              dataIndex: "role",
              width: 116,
              render: (role: User["role"]) => <Tag color={role === "admin" ? "blue" : "default"}>{role === "admin" ? "管理员" : "用户"}</Tag>
            },
            { title: "最近登录", dataIndex: "last_login_at", width: 180, render: (value?: string) => formatDateTime(value) },
            { title: "创建时间", dataIndex: "created_at", width: 180, render: (value: string) => formatDateTime(value) },
            {
              title: "操作",
              key: "actions",
              width: 112,
              align: "right",
              render: (_, record) => (
                <Space size={4}>
                  <Tooltip title="编辑">
                    <Button type="text" icon={<Pencil size={16} />} aria-label={`编辑 ${record.username}`} onClick={() => openEdit(record)} />
                  </Tooltip>
                  <Tooltip title={record.id === currentUser.id ? "不能删除当前登录用户" : "删除"}>
                    <Popconfirm
                      title="删除用户"
                      description={`确认删除 ${record.display_name}？历史素材和任务会保留。`}
                      okText="删除"
                      cancelText="取消"
                      okButtonProps={{ danger: true }}
                      disabled={record.id === currentUser.id}
                      onConfirm={() => void removeUser(record)}
                    >
                      <Button type="text" danger icon={<Trash2 size={16} />} aria-label={`删除 ${record.username}`} disabled={record.id === currentUser.id} />
                    </Popconfirm>
                  </Tooltip>
                </Space>
              )
            }
          ]}
        />
      </Card>

      <Modal
        title={editingUser ? `编辑用户：${editingUser.username}` : "新增用户"}
        open={editorOpen}
        onCancel={closeEditor}
        onOk={() => void saveUser()}
        okText="保存"
        cancelText="取消"
        confirmLoading={saving}
        destroyOnClose
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: "请输入用户名" }, { min: 2, message: "用户名至少 2 个字符" }]}>
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item name="display_name" label="显示名称" rules={[{ required: true, message: "请输入显示名称" }]}>
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item name="email" label="邮箱" rules={[{ type: "email", message: "请输入有效邮箱" }]}>
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="password"
            label={editingUser ? "新密码" : "密码"}
            extra={editingUser ? "留空则保持当前密码" : undefined}
            rules={[
              { required: !editingUser, message: "请输入密码" },
              { min: 6, message: "密码至少 6 个字符" }
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="role" label="角色" rules={[{ required: true, message: "请选择角色" }]}>
            <Select options={[{ value: "admin", label: "管理员" }, { value: "user", label: "用户" }]} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
