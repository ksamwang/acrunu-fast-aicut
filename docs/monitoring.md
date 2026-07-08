# 服务端监控

## 监控组件

当前服务端监控采用 Docker Compose 部署，包含：

- `Prometheus`：采集监控指标。
- `Grafana`：Web 监控面板。
- `Node Exporter`：采集服务器 CPU、内存、磁盘、网络等系统指标。
- `cAdvisor`：采集 Docker 容器指标。

## 访问方式

Grafana 对外开放：

```text
http://10.168.10.23:3000
```

默认账号密码来自 `.env`：

```text
GRAFANA_ADMIN_USER=admin
GRAFANA_ADMIN_PASSWORD=admin
```

首次部署后建议立即修改默认密码。

Prometheus 仅绑定服务器本机地址，不直接对外开放：

```text
127.0.0.1:9090
```

## 数据源

Grafana 已通过 provisioning 自动配置 Prometheus 数据源：

```text
http://prometheus:9090
```

## 已验证采集目标

服务器 `10.168.10.23` 已验证以下 Prometheus targets 正常：

- `prometheus`
- `node-exporter`
- `cadvisor`

## 常用维护命令

```bash
cd ~/acrunu-fast-aicut
sudo docker compose ps prometheus grafana node-exporter cadvisor
sudo docker compose logs --tail=100 grafana
sudo docker compose logs --tail=100 prometheus
sudo docker compose logs --tail=100 node-exporter
sudo docker compose logs --tail=100 cadvisor
```

## 后续增强

后续可以在 API 和 worker 中增加 `/metrics` 或独立 metrics endpoint，让 Prometheus 采集业务指标：

- 任务创建数
- 任务成功数
- 任务失败数
- 任务耗时
- worker 队列积压
- 素材上传数量
- 模型网关调用耗时
