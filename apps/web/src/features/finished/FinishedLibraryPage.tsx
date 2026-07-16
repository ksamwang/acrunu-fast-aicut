import { useEffect, useMemo, useState } from "react";
import { Button, Empty, Input, message, Progress, Segmented, Select, Tag, Tooltip } from "antd";
import { CircleAlert, LoaderCircle, Play, RotateCcw } from "lucide-react";
import { useResource } from "../../shared/hooks/use-resource";
import { formatDuration } from "../../shared/lib/format";
import type { FinishedWork } from "../../shared/types/generation";
import type { Product } from "../../shared/types/product";
import { listProducts } from "../products/api";
import { listVoiceoverWorks, retryVoiceoverWork } from "../workbench/api";
import { FinishedWorkDetail } from "./FinishedWorkDetail";
import { FinishedWorkVisual } from "./FinishedWorkVisual";
import "./styles.css";

type StatusFilter = "all" | "generating" | "completed";

function readFinishedWorkID() {
  const match = window.location.hash.match(/^#\/finished\/([^/?#]+)/);
  if (!match) {
    return "";
  }
  try {
    return decodeURIComponent(match[1]);
  } catch {
    return "";
  }
}

function writeFinishedWorkID(workID: string) {
  window.location.hash = `#/finished/${encodeURIComponent(workID)}`;
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

function formatCardDate(value?: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  const hours = String(date.getHours()).padStart(2, "0");
  const minutes = String(date.getMinutes()).padStart(2, "0");
  return `${month}-${day} ${hours}:${minutes}`;
}

export function FinishedLibraryPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token, [], listProducts);
  const [works, setWorks] = useState<FinishedWork[]>([]);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [productID, setProductID] = useState<string | undefined>();
  const [keyword, setKeyword] = useState("");
  const [selectedWorkID, setSelectedWorkID] = useState(readFinishedWorkID);
  const [retryingWorkID, setRetryingWorkID] = useState<string | null>(null);

  useEffect(() => {
    let disposed = false;
    const load = async () => {
      try {
        const nextWorks = await listVoiceoverWorks(token);
        if (!disposed) {
          setWorks(nextWorks);
        }
      } catch {
        // Keep the last successful list visible while the service is temporarily unavailable.
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), 3_000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [token]);

  useEffect(() => {
    const syncSelectedWork = () => setSelectedWorkID(readFinishedWorkID());
    window.addEventListener("hashchange", syncSelectedWork);
    return () => window.removeEventListener("hashchange", syncSelectedWork);
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

  const selectedWork = selectedWorkID ? works.find((work) => work.id === selectedWorkID) : undefined;

  const retryWork = async (workID: string) => {
    setRetryingWorkID(workID);
    try {
      const retried = await retryVoiceoverWork(workID, token);
      setWorks((current) => current.map((work) => (work.id === retried.id ? retried : work)));
      message.success("已重新加入生成队列");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "重试失败");
    } finally {
      setRetryingWorkID(null);
    }
  };

  if (selectedWorkID) {
    if (selectedWork) {
      return <FinishedWorkDetail work={selectedWork} onBack={() => { window.location.hash = "#/finished"; }} />;
    }
    return (
      <div className="finished-detail-page">
        <div className="finished-detail-missing">
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="未找到成品" />
          <Button onClick={() => { window.location.hash = "#/finished"; }}>返回成品库</Button>
        </div>
      </div>
    );
  }

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
              const isFailed = work.status === "failed";
              return (
                <article
                  className={`finished-work-card${isGenerating ? " is-generating" : ""}${isFailed ? " is-failed" : ""}`}
                  key={work.id}
                  data-status={work.status}
                  data-testid={`finished-work-${work.id}`}
                >
                  <button
                    type="button"
                    className={`finished-work-media${isGenerating ? " is-generating" : ""}`}
                    aria-label={`查看 ${work.title}`}
                    onClick={() => writeFinishedWorkID(work.id)}
                  >
                    <FinishedWorkVisual work={work} />
                    <span className="finished-work-overlay-top">
                      <span className="finished-work-overlay-labels">
                        <Tag className="finished-work-product">{work.product_name}</Tag>
                      </span>
                      <Tag className="finished-work-status" color={isGenerating ? "processing" : isFailed ? "error" : "green"}>
                        {isGenerating ? "生成中" : isFailed ? "生成失败" : "已完成"}
                      </Tag>
                    </span>
                    {isGenerating ? (
                      <span className="finished-work-generation-state">
                        <LoaderCircle size={22} />
                        <span>{work.stage_label}</span>
                      </span>
                    ) : isFailed ? (
                      <span className="finished-work-generation-state is-failed">
                        <CircleAlert size={22} />
                        <span>{work.error_message || "生成失败"}</span>
                      </span>
                    ) : (
                      <span className="finished-work-play"><Play size={18} fill="currentColor" /></span>
                    )}
                    <span className="finished-work-overlay-bottom">
                      <span className="finished-work-overlay-title">{work.title}</span>
                      <span className="finished-work-overlay-script">{work.script_text}</span>
                      <span className="finished-work-overlay-meta">
                        <span>{formatDuration(work.duration_ms)}</span>
                        <span>{isGenerating ? `${work.progress}% · ${work.stage_label}` : isFailed ? "生成失败" : `完成 ${formatCardDate(work.completed_at ?? work.created_at)}`}</span>
                      </span>
                      {isGenerating ? <Progress percent={work.progress} showInfo={false} size="small" strokeColor="#4fc1b2" /> : null}
                    </span>
                  </button>
                  {isFailed ? (
                    <Tooltip title="沿用当前文案和音色重新生成">
                      <Button
                        type="primary"
                        className="finished-work-retry"
                        icon={<RotateCcw size={15} />}
                        loading={retryingWorkID === work.id}
                        onClick={() => void retryWork(work.id)}
                      >
                        重试
                      </Button>
                    </Tooltip>
                  ) : null}
                </article>
              );
            })}
          </div>
        )}
      </main>
    </div>
  );
}
