import { useState } from "react";
import { Button, Card, Descriptions, Modal, Table, Tag, Typography, message } from "antd";
import { useResource } from "../../shared/hooks/use-resource";
import { formatDateTime } from "../../shared/lib/format";
import { taskStatusLabels, taskTypeLabels, translateValue } from "../../shared/lib/labels";
import type { Task } from "../../shared/types/task";
import { getTask, listTasks } from "./api";
import "./styles.css";

export function TasksPage({ token }: { token: string }) {
  const tasks = useResource<Task[]>("/api/tasks", token, [], listTasks);
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const taskTypeLabel = (taskType: string) => {
    return translateValue(taskType, taskTypeLabels);
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
    <div data-testid="tasks-page" className="tasks-page">
      <div className="page-stack tasks-page-stack">
        <Card className="task-list-card" title="任务日志">
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
      </div>

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
