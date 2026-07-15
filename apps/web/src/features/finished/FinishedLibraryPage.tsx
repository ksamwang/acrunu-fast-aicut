import { useEffect, useMemo, useState } from "react";
import { Button, Checkbox, Empty, Input, Modal, Segmented, Select, Tag, Tooltip, Typography, message } from "antd";
import { Check, Clapperboard, Eye, Play, Send } from "lucide-react";
import { useResource } from "../../shared/hooks/use-resource";
import { formatDateTime, formatDuration } from "../../shared/lib/format";
import type { FinishedWork, FinishedWorkStatus } from "../../shared/types/generation";
import type { Product } from "../../shared/types/product";
import { listProducts } from "../products/api";
import { listPrototypeFinishedWorks, submitPrototypeFinishedWorks } from "../workbench/prototype-api";
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

export function FinishedLibraryPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token, [], listProducts);
  const [works, setWorks] = useState<FinishedWork[]>(() => listPrototypeFinishedWorks());
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("ready_to_submit");
  const [productID, setProductID] = useState<string | undefined>();
  const [keyword, setKeyword] = useState("");
  const [selectedIDs, setSelectedIDs] = useState<string[]>([]);
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

  const selectableWorks = filteredWorks.filter((work) => work.status === "ready_to_submit");

  const toggleSelected = (workID: string, checked: boolean) => {
    setSelectedIDs((current) => (checked ? Array.from(new Set([...current, workID])) : current.filter((id) => id !== workID)));
  };

  const submitWorks = (workIDs: string[]) => {
    const eligibleIDs = workIDs.filter((workID) => works.some((work) => work.id === workID && work.status === "ready_to_submit"));
    if (eligibleIDs.length === 0) {
      return;
    }
    setWorks(submitPrototypeFinishedWorks(eligibleIDs));
    setSelectedIDs((current) => current.filter((workID) => !eligibleIDs.includes(workID)));
    message.success(`已提交 ${eligibleIDs.length} 条成品`);
  };

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
            { label: "待提交", value: "ready_to_submit" },
            { label: "已提交", value: "submitted" },
            { label: "全部", value: "all" }
          ]}
          onChange={(value) => setStatusFilter(value)}
        />
        <Input value={keyword} allowClear placeholder="搜索成品" onChange={(event) => setKeyword(event.target.value)} />
        {selectedIDs.length > 0 ? (
          <Button type="primary" icon={<Send size={16} />} onClick={() => submitWorks(selectedIDs)}>
            提交 {selectedIDs.length} 条
          </Button>
        ) : null}
      </section>

      <main className="finished-work-scroll">
        {filteredWorks.length === 0 ? (
          <div className="finished-empty-state">
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={statusFilter === "submitted" ? "暂无已提交成品" : "暂无待提交成品"} />
          </div>
        ) : (
          <div className="finished-waterfall">
            {filteredWorks.map((work) => {
              const selected = selectedIDs.includes(work.id);
              const isReady = work.status === "ready_to_submit";
              return (
                <article className={`finished-work-card${selected ? " is-selected" : ""}`} key={work.id}>
                  {isReady ? (
                    <Checkbox
                      className="finished-work-select"
                      checked={selected}
                      aria-label={`选择 ${work.title}`}
                      onChange={(event) => toggleSelected(work.id, event.target.checked)}
                    />
                  ) : null}
                  <button type="button" className="finished-work-media" onClick={() => setPreviewingWork(work)}>
                    <FinishedWorkVisual work={work} />
                    <span className="finished-work-media-scrim" />
                    <span className="finished-work-play"><Play size={18} fill="currentColor" /></span>
                    <span className="finished-work-duration">{formatDuration(work.duration_ms)}</span>
                    <Tag className="finished-work-status" color={isReady ? "gold" : "green"}>
                      {isReady ? "待提交" : "已提交"}
                    </Tag>
                  </button>
                  <div className="finished-work-body">
                    <Tag className="finished-work-product">{work.product_name}</Tag>
                    <Typography.Text className="finished-work-title">{work.title}</Typography.Text>
                    <Typography.Paragraph className="finished-work-script" ellipsis={{ rows: 2 }}>
                      {work.script_text}
                    </Typography.Paragraph>
                    <div className="finished-work-meta">
                      <span>{formatDateTime(work.created_at)}</span>
                      {work.submitted_at ? <span>{formatDateTime(work.submitted_at)}</span> : null}
                    </div>
                    <div className="finished-work-actions">
                      <Tooltip title="预览成品">
                        <Button
                          type="text"
                          aria-label="预览成品"
                          icon={<Eye size={17} />}
                          onClick={() => setPreviewingWork(work)}
                        />
                      </Tooltip>
                      {isReady ? (
                        <Button type="primary" size="small" icon={<Send size={15} />} onClick={() => submitWorks([work.id])}>
                          提交
                        </Button>
                      ) : (
                        <span className="finished-work-submitted"><Check size={15} /> 已提交</span>
                      )}
                    </div>
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
