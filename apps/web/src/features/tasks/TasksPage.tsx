import { useMemo, useState } from "react";
import { Button, Card, Descriptions, Modal, Select, Space, Table, Tag, Typography, message } from "antd";
import { useResource } from "../../shared/hooks/use-resource";
import { formatDateTime } from "../../shared/lib/format";
import { taskStatusLabels, taskTypeLabels, translateValue } from "../../shared/lib/labels";
import type { Task } from "../../shared/types/task";
import { createTestTask, getTask, listTasks } from "./api";

export function TasksPage({ token }: { token: string }) {
  const [taskFilters, setTaskFilters] = useState({
    taskType: "",
    status: ""
  });
  const taskPath = useMemo(() => {
    const params = new URLSearchParams();
    if (taskFilters.taskType) {
      params.set("task_type", taskFilters.taskType);
    }
    if (taskFilters.status) {
      params.set("status", taskFilters.status);
    }
    const query = params.toString();
    return query ? `/api/tasks?${query}` : "/api/tasks";
  }, [taskFilters]);
  const tasks = useResource<Task[]>(taskPath, token, [taskPath], listTasks);
  const [creating, setCreating] = useState(false);
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const taskTypeLabel = (taskType: string) => {
    return translateValue(taskType, taskTypeLabels);
  };

  const createTask = async () => {
    setCreating(true);
    try {
      await createTestTask(token);
      await tasks.reload();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "创建任务失败");
    } finally {
      setCreating(false);
    }
  };

  const openTaskDetail = async (taskID: string) => {
    setDetailLoading(true);
    try {
      const task = await getTask(taskID, token);
      setSelectedTask(task);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "加载任务失败");
    } finally {
      setDetailLoading(false);
    }
  };

  return (
    <div data-testid="tasks-page">
      <Space direction="vertical" size="middle" className="page-stack">
        <Card title="任务筛选">
          <Space wrap>
            <Select
              data-testid="task-filter-type"
              value={taskFilters.taskType || undefined}
              placeholder="任务类型"
              allowClear
              style={{ minWidth: 220 }}
              options={[
                { value: "asset_extract_frames", label: "素材抽帧" },
                { value: "asset_analyze", label: "素材分析" },
                { value: "asset_embedding", label: "素材向量化" },
                { value: "test", label: "测试任务" }
              ]}
              onChange={(value) => setTaskFilters((current) => ({ ...current, taskType: value ?? "" }))}
            />
            <Select
              data-testid="task-filter-status"
              value={taskFilters.status || undefined}
              placeholder="状态"
              allowClear
              style={{ minWidth: 160 }}
              options={[
                { value: "queued", label: "排队中" },
                { value: "running", label: "执行中" },
                { value: "completed", label: "已完成" },
                { value: "failed", label: "失败" }
              ]}
              onChange={(value) => setTaskFilters((current) => ({ ...current, status: value ?? "" }))}
            />
            <Button data-testid="task-filter-reset" onClick={() => setTaskFilters({ taskType: "", status: "" })}>重置</Button>
            <Button data-testid="task-filter-refresh" onClick={tasks.reload}>刷新</Button>
          </Space>
        </Card>
        <Card title="批量剪辑任务" extra={<Button type="primary" loading={creating} onClick={createTask}>创建测试任务</Button>}>
          <Table<Task>
            rowKey="id"
            loading={tasks.loading}
            dataSource={tasks.data ?? []}
            onRow={(record) => ({ onClick: () => void openTaskDetail(record.id) })}
            columns={[
              {
                title: "任务 ID",
                dataIndex: "id",
                render: (value: string, task) => (
                  <Button type="link" className="table-link-button" onClick={() => void openTaskDetail(task.id)}>
                    {value}
                  </Button>
                )
              },
              { title: "类型", dataIndex: "task_type", render: (value) => taskTypeLabel(value) },
              { title: "状态", dataIndex: "status", render: (status) => <Tag>{translateValue(status, taskStatusLabels)}</Tag> },
              { title: "素材 ID", dataIndex: "asset_id", render: (value) => value || "-" },
              { title: "重试次数", dataIndex: "retry_count" },
              { title: "耗时", dataIndex: "duration_ms", render: (value) => (value ? `${value} 毫秒` : "-") },
              { title: "创建时间", dataIndex: "created_at", render: (value) => formatDateTime(value) }
            ]}
          />
        </Card>
      </Space>

      <Modal
        title={selectedTask ? `任务详情：${selectedTask.id}` : "任务详情"}
        open={selectedTask !== null}
        footer={null}
        width={840}
        confirmLoading={detailLoading}
        onCancel={() => setSelectedTask(null)}
      >
        {selectedTask ? (
          <Descriptions bordered column={1} size="small" data-testid="task-detail-modal">
            <Descriptions.Item label="任务类型">{taskTypeLabel(selectedTask.task_type)}</Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag>{translateValue(selectedTask.status, taskStatusLabels)}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="素材 ID">{selectedTask.asset_id || "-"}</Descriptions.Item>
            <Descriptions.Item label="重试次数">{selectedTask.retry_count}</Descriptions.Item>
            <Descriptions.Item label="耗时">{selectedTask.duration_ms ? `${selectedTask.duration_ms} 毫秒` : "-"}</Descriptions.Item>
            <Descriptions.Item label="创建时间">{formatDateTime(selectedTask.created_at)}</Descriptions.Item>
            <Descriptions.Item label="开始时间">{formatDateTime(selectedTask.started_at)}</Descriptions.Item>
            <Descriptions.Item label="结束时间">{formatDateTime(selectedTask.finished_at)}</Descriptions.Item>
            <Descriptions.Item label="错误信息">
              {selectedTask.error_message || <Typography.Text type="secondary">无</Typography.Text>}
            </Descriptions.Item>
            <Descriptions.Item label="负载摘要">
              {selectedTask.payload_summary && Object.keys(selectedTask.payload_summary).length > 0 ? (
                <pre className="json-block">{JSON.stringify(selectedTask.payload_summary, null, 2)}</pre>
              ) : (
                <Typography.Text type="secondary">暂无负载摘要</Typography.Text>
              )}
            </Descriptions.Item>
          </Descriptions>
        ) : null}
      </Modal>
    </div>
  );
}
