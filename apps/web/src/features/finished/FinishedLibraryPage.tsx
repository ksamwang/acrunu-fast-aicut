import { useEffect, useMemo, useState } from "react";
import { Button, Checkbox, Dropdown, Empty, Input, message, Modal, Progress, Segmented, Select, Tag } from "antd";
import type { MenuProps } from "antd";
import { CheckSquare2, CircleAlert, Download, LoaderCircle, Play, RotateCcw, Trash2, UserRound, X } from "lucide-react";
import { useResource } from "../../shared/hooks/use-resource";
import { formatDuration } from "../../shared/lib/format";
import type { FinishedWork } from "../../shared/types/generation";
import type { Product } from "../../shared/types/product";
import { listProducts } from "../products/api";
import { deleteVoiceoverWork, listVoiceoverWorks, regenerateVoiceoverWork, retryVoiceoverWork } from "../workbench/api";
import { FinishedWorkDetail } from "./FinishedWorkDetail";
import { FinishedWorkVisual } from "./FinishedWorkVisual";
import { createFinishedWorkDownload } from "./api";
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
  const [actionWorkID, setActionWorkID] = useState<string | null>(null);
  const [selectionMode, setSelectionMode] = useState(false);
  const [selectedWorkIDs, setSelectedWorkIDs] = useState<Set<string>>(() => new Set());
  const [downloading, setDownloading] = useState(false);
  const [retryingSelected, setRetryingSelected] = useState(false);
  const [deletingSelected, setDeletingSelected] = useState(false);

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
  const selectableWorks = filteredWorks.filter((work) => work.status !== "generating");
  const canDownloadWorks = (workIDs: string[]) => {
    const workIDSet = new Set(workIDs);
    const targetWorks = works.filter((work) => workIDSet.has(work.id));
    return targetWorks.length > 0
      && targetWorks.length === workIDSet.size
      && targetWorks.every((work) => work.status === "completed" && Boolean(work.video_url));
  };
  const canRetryWorks = (workIDs: string[]) => {
    const workIDSet = new Set(workIDs);
    const targetWorks = works.filter((work) => workIDSet.has(work.id));
    return targetWorks.length > 0
      && targetWorks.length === workIDSet.size
      && targetWorks.every((work) => work.status !== "generating");
  };
  const canDownloadSelection = canDownloadWorks(Array.from(selectedWorkIDs));
  const canRetrySelection = canRetryWorks(Array.from(selectedWorkIDs));
  const allSelectableSelected = selectableWorks.length > 0 && selectableWorks.every((work) => selectedWorkIDs.has(work.id));

  useEffect(() => {
    const availableIDs = new Set(works.filter((work) => work.status !== "generating").map((work) => work.id));
    setSelectedWorkIDs((current) => {
      const next = new Set(Array.from(current).filter((workID) => availableIDs.has(workID)));
      return next.size === current.size ? current : next;
    });
  }, [works]);

  const toggleWorkSelection = (workID: string) => {
    setSelectedWorkIDs((current) => {
      const next = new Set(current);
      if (next.has(workID)) {
        next.delete(workID);
      } else {
        next.add(workID);
      }
      return next;
    });
  };

  const toggleSelectAll = () => {
    setSelectedWorkIDs((current) => {
      const next = new Set(current);
      if (allSelectableSelected) {
        selectableWorks.forEach((work) => next.delete(work.id));
      } else {
        selectableWorks.forEach((work) => next.add(work.id));
      }
      return next;
    });
  };

  const exitSelectionMode = () => {
    setSelectionMode(false);
    setSelectedWorkIDs(new Set());
  };

  const downloadSelectedWorks = async (workIDs = Array.from(selectedWorkIDs)) => {
    if (!canDownloadWorks(workIDs)) {
      return;
    }
    setDownloading(true);
    try {
      const batch = await createFinishedWorkDownload(workIDs, token);
      const link = document.createElement("a");
      link.href = batch.download_url;
      link.download = batch.file_name;
      document.body.appendChild(link);
      link.click();
      link.remove();
      message.success(`正在下载 ${batch.file_count} 个成品`);
      exitSelectionMode();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "批量下载失败");
    } finally {
      setDownloading(false);
    }
  };

  const retrySelectedWorks = async (workIDs = Array.from(selectedWorkIDs)) => {
    if (!canRetryWorks(workIDs)) {
      return;
    }
    const retriedWorks: FinishedWork[] = [];
    const failedIDs: string[] = [];
    const targetWorks = new Map(works.filter((work) => workIDs.includes(work.id)).map((work) => [work.id, work]));
    setRetryingSelected(true);
    try {
      for (let index = 0; index < workIDs.length; index += 4) {
        const batch = workIDs.slice(index, index + 4);
        const results = await Promise.all(batch.map(async (workID) => {
          try {
            const work = targetWorks.get(workID);
            const next = work?.status === "failed"
              ? await retryVoiceoverWork(workID, token)
              : await regenerateVoiceoverWork(workID, token);
            return { workID, work: next };
          } catch {
            return { workID, work: null };
          }
        }));
        results.forEach((result) => {
          if (result.work) {
            retriedWorks.push(result.work);
          } else {
            failedIDs.push(result.workID);
          }
        });
      }
      if (retriedWorks.length > 0) {
        const retriedByID = new Map(retriedWorks.map((work) => [work.id, work]));
        setWorks((current) => current.map((work) => retriedByID.get(work.id) ?? work));
      }
      if (failedIDs.length === 0) {
        message.success(`已重试 ${retriedWorks.length} 个成品`);
        exitSelectionMode();
      } else {
        setSelectedWorkIDs(new Set(failedIDs));
        const result = `已重试 ${retriedWorks.length} 项，失败 ${failedIDs.length} 项`;
        if (retriedWorks.length > 0) {
          message.warning(result);
        } else {
          message.error(result);
        }
      }
    } finally {
      setRetryingSelected(false);
    }
  };

  const confirmRetrySelected = (workIDs = Array.from(selectedWorkIDs)) => {
    if (!canRetryWorks(workIDs)) {
      return;
    }
    const workIDSet = new Set(workIDs);
    const targetWorks = works.filter((work) => workIDSet.has(work.id));
    const failedCount = targetWorks.filter((work) => work.status === "failed").length;
    const completedCount = targetWorks.filter((work) => work.status === "completed").length;
    Modal.confirm({
      title: `重试 ${workIDs.length} 个成品？`,
      content: `失败成品 ${failedCount} 个将从失败阶段继续，已完成成品 ${completedCount} 个将从配音阶段重新生成并替换现有视频。`,
      okText: "开始重试",
      cancelText: "取消",
      centered: true,
      onOk: () => retrySelectedWorks(workIDs)
    });
  };

  const deleteSelectedWorks = async (workIDs = Array.from(selectedWorkIDs)) => {
    if (workIDs.length === 0) {
      return;
    }
    const deletedIDs: string[] = [];
    const failedIDs: string[] = [];
    setDeletingSelected(true);
    try {
      for (let index = 0; index < workIDs.length; index += 4) {
        const batch = workIDs.slice(index, index + 4);
        const results = await Promise.all(batch.map(async (workID) => {
          try {
            await deleteVoiceoverWork(workID, token);
            return { workID, deleted: true };
          } catch {
            return { workID, deleted: false };
          }
        }));
        results.forEach((result) => {
          (result.deleted ? deletedIDs : failedIDs).push(result.workID);
        });
      }

      if (deletedIDs.length > 0) {
        const deleted = new Set(deletedIDs);
        setWorks((current) => current.filter((work) => !deleted.has(work.id)));
      }
      if (failedIDs.length === 0) {
        message.success(`已删除 ${deletedIDs.length} 个成品`);
        exitSelectionMode();
      } else {
        setSelectedWorkIDs(new Set(failedIDs));
        const result = `已删除 ${deletedIDs.length} 项，失败 ${failedIDs.length} 项`;
        if (deletedIDs.length > 0) {
          message.warning(result);
        } else {
          message.error(result);
        }
      }
    } finally {
      setDeletingSelected(false);
    }
  };

  const confirmDeleteSelected = (workIDs = Array.from(selectedWorkIDs)) => {
    if (workIDs.length === 0) {
      return;
    }
    Modal.confirm({
      title: `删除 ${workIDs.length} 个成品？`,
      content: "所选成品将从成品库移除，已渲染的视频文件也会删除。此操作不可撤销。",
      okText: "删除",
      cancelText: "取消",
      okButtonProps: { danger: true },
      centered: true,
      onOk: () => deleteSelectedWorks(workIDs)
    });
  };

  const regenerateWork = async (workID: string) => {
    setActionWorkID(workID);
    try {
      const regenerated = await regenerateVoiceoverWork(workID, token);
      setWorks((current) => current.map((work) => (work.id === regenerated.id ? regenerated : work)));
      message.success("已开始重新生成");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "重新生成失败");
    } finally {
      setActionWorkID(null);
    }
  };

  const retryWork = async (workID: string) => {
    setActionWorkID(workID);
    try {
      const retried = await retryVoiceoverWork(workID, token);
      setWorks((current) => current.map((work) => (work.id === retried.id ? retried : work)));
      message.success("已开始重试");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "重试失败");
    } finally {
      setActionWorkID(null);
    }
  };

  const deleteWork = async (workID: string) => {
    setActionWorkID(workID);
    try {
      await deleteVoiceoverWork(workID, token);
      setWorks((current) => current.filter((work) => work.id !== workID));
      message.success("成片已删除");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "删除成片失败");
    } finally {
      setActionWorkID(null);
    }
  };

  const confirmRegenerate = (work: FinishedWork) => {
    Modal.confirm({
      title: "重新生成成片？",
      content: "将沿用当前文案、音色和成片样式，从配音阶段重新生成，现有成片会被替换。",
      okText: "重新生成",
      cancelText: "取消",
      centered: true,
      onOk: () => regenerateWork(work.id)
    });
  };

  const confirmDelete = (work: FinishedWork) => {
    Modal.confirm({
      title: "删除成片？",
      content: `“${work.title}”将从成品库移除，已渲染的视频文件也会删除。`,
      okText: "删除",
      cancelText: "取消",
      okButtonProps: { danger: true },
      centered: true,
      onOk: () => deleteWork(work.id)
    });
  };

  const workContextMenu = (work: FinishedWork): MenuProps => {
    const disabled = work.status === "generating" || actionWorkID !== null;
    const retryable = work.status === "failed";
    return {
      items: [
        {
          key: retryable ? "retry" : "regenerate",
          icon: <RotateCcw size={15} />,
          label: retryable ? "重试" : "重新生成",
          disabled
        },
        { type: "divider" },
        {
          key: "delete",
          icon: <Trash2 size={15} />,
          label: "删除成片",
          danger: true,
          disabled
        }
      ],
      onClick: ({ key }) => {
        if (key === "retry") {
          void retryWork(work.id);
        } else if (key === "regenerate") {
          confirmRegenerate(work);
        } else if (key === "delete") {
          confirmDelete(work);
        }
      }
    };
  };

  const batchContextMenu = (work: FinishedWork): MenuProps => {
    const workIDs = selectedWorkIDs.has(work.id) ? Array.from(selectedWorkIDs) : [work.id];
    return {
      items: [
        {
          key: "retry-selected",
          icon: <RotateCcw size={15} />,
          label: "重试选中",
          disabled: !canRetryWorks(workIDs) || retryingSelected || downloading || deletingSelected
        },
        {
          key: "download-selected",
          icon: <Download size={15} />,
          label: "下载选中",
          disabled: !canDownloadWorks(workIDs) || retryingSelected || downloading || deletingSelected
        },
        { type: "divider" },
        {
          key: "delete-selected",
          icon: <Trash2 size={15} />,
          label: "删除选中",
          danger: true,
          disabled: workIDs.length === 0 || retryingSelected || downloading || deletingSelected
        }
      ],
      onClick: ({ key }) => {
        if (key === "retry-selected") {
          confirmRetrySelected(workIDs);
        } else if (key === "download-selected") {
          void downloadSelectedWorks(workIDs);
        } else if (key === "delete-selected") {
          confirmDeleteSelected(workIDs);
        }
      }
    };
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
      <section className={`finished-toolbar${selectionMode ? " is-selection" : ""}`} aria-label={selectionMode ? "批量选择成品" : "成品筛选"}>
        {selectionMode ? (
          <>
            <span className="finished-selection-summary"><CheckSquare2 size={17} />已选 {selectedWorkIDs.size} 项</span>
            <Button onClick={toggleSelectAll} disabled={selectableWorks.length === 0 || retryingSelected || downloading || deletingSelected}>{allSelectableSelected ? "取消全选" : "全选当前结果"}</Button>
            <Button onClick={() => setSelectedWorkIDs(new Set())} disabled={selectedWorkIDs.size === 0 || retryingSelected || downloading || deletingSelected}>清空</Button>
            <span className="finished-toolbar-spacer" />
            <Button icon={<RotateCcw size={16} />} loading={retryingSelected} disabled={!canRetrySelection || downloading || deletingSelected} onClick={() => confirmRetrySelected()}>重试选中</Button>
            <Button danger icon={<Trash2 size={16} />} loading={deletingSelected} disabled={selectedWorkIDs.size === 0 || retryingSelected || downloading} onClick={() => confirmDeleteSelected()}>删除选中</Button>
            <Button type="primary" icon={<Download size={16} />} loading={downloading} disabled={!canDownloadSelection || retryingSelected || deletingSelected} title={selectedWorkIDs.size > 0 && !canDownloadSelection ? "仅支持下载已完成且存在视频的成品" : undefined} onClick={() => void downloadSelectedWorks()}>下载选中</Button>
            <Button icon={<X size={16} />} disabled={retryingSelected || downloading || deletingSelected} onClick={exitSelectionMode}>退出选择</Button>
          </>
        ) : (
          <>
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
            <span className="finished-toolbar-spacer" />
            <Button icon={<CheckSquare2 size={16} />} onClick={() => setSelectionMode(true)}>批量选择</Button>
          </>
        )}
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
              const selectable = work.status !== "generating";
              const selected = selectedWorkIDs.has(work.id);
              return (
                <Dropdown
                  key={work.id}
                  menu={selectionMode ? batchContextMenu(work) : workContextMenu(work)}
                  trigger={["contextMenu"]}
                  disabled={selectionMode && !selectable}
                >
                  <article
                    className={`finished-work-card${isGenerating ? " is-generating" : ""}${isFailed ? " is-failed" : ""}${selectionMode ? " is-selection-mode" : ""}${selected ? " is-selected" : ""}`}
                    data-status={work.status}
                    data-testid={`finished-work-${work.id}`}
                    onContextMenu={() => {
                      if (selectionMode && selectable && !selected) {
                        setSelectedWorkIDs(new Set([work.id]));
                      }
                    }}
                  >
                    {selectionMode ? (
                      <span className="finished-work-selector" onClick={(event) => event.stopPropagation()}>
                        <Checkbox checked={selected} disabled={!selectable} aria-label={`选择 ${work.title}`} onChange={() => selectable && toggleWorkSelection(work.id)} />
                      </span>
                    ) : null}
                    <button
                      type="button"
                      className={`finished-work-media${isGenerating ? " is-generating" : ""}`}
                      aria-label={selectionMode ? `${selected ? "取消选择" : "选择"} ${work.title}` : `查看 ${work.title}`}
                      onClick={() => selectionMode ? (selectable && toggleWorkSelection(work.id)) : writeFinishedWorkID(work.id)}
                    >
                      <FinishedWorkVisual work={work} compact />
                      <span className="finished-work-overlay-top">
                        <span className="finished-work-overlay-labels">
                          <Tag className="finished-work-product">{work.product_name}</Tag>
                          <span className="finished-work-creator"><UserRound size={12} />创建人：{work.created_by_name || "未知用户"}</span>
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
                  </article>
                </Dropdown>
              );
            })}
          </div>
        )}
      </main>
    </div>
  );
}
