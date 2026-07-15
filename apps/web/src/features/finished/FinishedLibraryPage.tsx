import { useEffect, useMemo, useState } from "react";
import { Button, Empty, Input, Modal, Progress, Segmented, Select, Tag, Tooltip, Typography } from "antd";
import { Clapperboard, Eye, LoaderCircle, Play } from "lucide-react";
import { useResource } from "../../shared/hooks/use-resource";
import { formatDateTime, formatDuration } from "../../shared/lib/format";
import type { FinishedWork, FinishedWorkStatus } from "../../shared/types/generation";
import type { Product } from "../../shared/types/product";
import { listProducts } from "../products/api";
import { listPrototypeFinishedWorks } from "../workbench/prototype-api";
import "./styles.css";

type StatusFilter = "all" | FinishedWorkStatus;

function FinishedWorkVisual({ work, compact = false }: { work: FinishedWork; compact?: boolean }) {
  return work.product_cover_url ? (
    <img src={work.product_cover_url} alt={`${work.product_name}成品预览`} />
  ) : (
    <div className={`finished-work-fallback${compact ? " is-compact" : ""}`}>
      <Clapperboard size={compact ? 26 : 36} />
      <span>{work.product_name}</span>
    </div>
  );
}

function emptyDescription(status: StatusFilter) {
  if (status === "generating") {
    return "暂无生成中的成品";
  }
  if (status === "completed") {
    return "暂无已完成成品";
  }
  return "暂无成品";
}

export function FinishedLibraryPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token, [], listProducts);
  const [works, setWorks] = useState<FinishedWork[]>(() => listPrototypeFinishedWorks());
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [productID, setProductID] = useState<string | undefined>();
  const [keyword, setKeyword] = useState("");
  const [previewingWork, setPreviewingWork] = useState<FinishedWork | null>(null);

  useEffect(() => {
    const timer = window.setInterval(() => setWorks(listPrototypeFinishedWorks()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const filteredWorks = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLocaleLowerCase();
    return works.filter((work) => {
      if (statusFilter !== "all" && work.status !== statusFilter) {
        return false;
      }
      if (productID && work.product_id !== productID) {
        return false;
      }
      if (!normalizedKeyword) {
        return true;
      }
      return [work.product_name, work.title, work.hook, work.script_text]
        .join(" ")
        .toLocaleLowerCase()
        .includes(normalizedKeyword);
    });
  }, [keyword, productID, statusFilter, works]);

  return (
    <div className="finished-library-page" data-testid="finished-library-page">
      <section className="finished-toolbar" aria-label="成品筛选">
        <Select
          allowClear
          value={productID}
          placeholder="产品"
          options={(products.data ?? []).filter((product) => product.status !== "archived").map((product) => ({ value: product.id, label: product.name }))}
          onChange={(value) => setProductID(value)}
        />
        <Segmented<StatusFilter>
          value={statusFilter}
          options={[
            { label: "全部", value: "all" },
            { label: "生成中", value: "generating" },
            { label: "已完成", value: "completed" }
          ]}
          onChange={(value) => setStatusFilter(value)}
        />
        <Input value={keyword} allowClear placeholder="搜索成品" onChange={(event) => setKeyword(event.target.value)} />
      </section>

      <main className="finished-work-scroll">
        {filteredWorks.length === 0 ? (
          <div className="finished-empty-state">
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyDescription(statusFilter)} />
          </div>
        ) : (
          <div className="finished-waterfall">
            {filteredWorks.map((work) => {
              const isGenerating = work.status === "generating";
              return (
                <article className={`finished-work-card${isGenerating ? " is-generating" : ""}`} key={work.id} data-status={work.status}>
                  {isGenerating ? (
                    <div className="finished-work-media is-generating" aria-label={`${work.title}正在生成`}>
                      <FinishedWorkVisual work={work} />
                      <span className="finished-work-media-scrim" />
                      <span className="finished-work-generation-state">
                        <LoaderCircle size={22} />
                        <span>{work.stage_label}</span>
                      </span>
                      <span className="finished-work-duration">{formatDuration(work.duration_ms)}</span>
                      <Tag className="finished-work-status" color="processing">生成中</Tag>
                      <span className="finished-work-progress">
                        <Progress percent={work.progress} showInfo={false} size="small" strokeColor="#2f8c83" />
                        <span>{work.progress}%</span>
                      </span>
                    </div>
                  ) : (
                    <button type="button" className="finished-work-media" onClick={() => setPreviewingWork(work)}>
                      <FinishedWorkVisual work={work} />
                      <span className="finished-work-media-scrim" />
                      <span className="finished-work-play"><Play size={18} fill="currentColor" /></span>
                      <span className="finished-work-duration">{formatDuration(work.duration_ms)}</span>
                      <Tag className="finished-work-status" color="green">已完成</Tag>
                    </button>
                  )}
                  <div className="finished-work-body">
                    <Tag className="finished-work-product">{work.product_name}</Tag>
                    <Typography.Text className="finished-work-title">{work.title}</Typography.Text>
                    <Typography.Paragraph className="finished-work-script" ellipsis={{ rows: 2 }}>
                      {work.script_text}
                    </Typography.Paragraph>
                    <div className="finished-work-meta">
                      <span>{isGenerating ? `开始于 ${formatDateTime(work.created_at)}` : `完成于 ${formatDateTime(work.completed_at ?? work.created_at)}`}</span>
                    </div>
                    {!isGenerating ? (
                      <div className="finished-work-actions">
                        <Tooltip title="预览成品">
                          <Button
                            type="text"
                            aria-label="预览成品"
                            icon={<Eye size={17} />}
                            onClick={() => setPreviewingWork(work)}
                          />
                        </Tooltip>
                      </div>
                    ) : null}
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </main>

      <Modal
        open={previewingWork !== null}
        width={720}
        footer={null}
        title={previewingWork?.title}
        onCancel={() => setPreviewingWork(null)}
        className="finished-preview-modal"
      >
        {previewingWork ? (
          <div className="finished-preview-content">
            <div className="finished-preview-stage">
              <FinishedWorkVisual work={previewingWork} />
              <span className="finished-preview-play"><Play size={26} fill="currentColor" /></span>
            </div>
            <div className="finished-preview-meta">
              <Tag>{previewingWork.product_name}</Tag>
              <Typography.Text type="secondary">{formatDuration(previewingWork.duration_ms)}</Typography.Text>
              <Typography.Paragraph>{previewingWork.script_text}</Typography.Paragraph>
            </div>
          </div>
        ) : null}
      </Modal>
    </div>
  );
}
