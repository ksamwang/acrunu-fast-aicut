import { useEffect, useMemo, useState } from "react";
import { Alert, Button, Card, Descriptions, Divider, Drawer, Empty, Form, Input, InputNumber, Modal, Pagination, Popconfirm, Select, Space, Table, Tabs, Tag, Tooltip, Typography, message } from "antd";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useResource } from "../../shared/hooks/use-resource";
import { assetDisplayTitle, assetFileDisplayName, assetVideoURL } from "../../shared/lib/asset-display";
import { formatDateTime, formatDuration, formatTimestamp } from "../../shared/lib/format";
import { analysisStatusLabels, assetStatusLabels, cameraMovementLabels, manualCleanStatusLabels, shotSizeLabels, sourceTypeLabels, translateValue, usabilityStatusLabels } from "../../shared/lib/labels";
import type { Asset, AssetEmbeddingListResponse, AssetEmbeddingObject, AssetEmbeddingRunResult, AssetEmbeddingTarget, AssetFrameResponse, AssetFrameSnapshot, AssetListResponse, AssetReviewPayload, AssetSemanticPreview, AssetSellingPointPayload, AssetSpeechSegment } from "../../shared/types/asset";
import type { Product, SellingPoint } from "../../shared/types/product";
import { AssetGrid } from "./AssetGrid";
import { archiveAssets, createAssetEmbeddings, getAssetEmbeddings, getAssetFrames, getAssetSellingPoints, getSemanticPreview, getSpeechSegments, listAssets, listAssetSelection, listProducts, listSellingPoints, saveAssetReview, saveAssetSellingPoints as persistAssetSellingPoints, updateAssetArchiveState as persistAssetArchiveState } from "./api";
import "./styles.css";

function renderTagList(items?: string[], emptyText = "-") {
  if (!items || items.length === 0) {
    return <Typography.Text type="secondary">{emptyText}</Typography.Text>;
  }

  return (
    <Space wrap size={[6, 6]}>
      {items.map((item) => (
        <Tag key={item}>{item}</Tag>
      ))}
    </Space>
  );
}

type AssetPosition = {
  page: number;
  index: number;
};

function assetPathForPage(path: string, page: number, pageSize: number) {
  const [pathname, query = ""] = path.split("?", 2);
  const params = new URLSearchParams(query);
  params.set("page", String(page));
  params.set("page_size", String(pageSize));
  return `${pathname}?${params.toString()}`;
}

function assetSelectionPathForList(path: string) {
  const [, query = ""] = path.split("?", 2);
  const params = new URLSearchParams(query);
  params.delete("page");
  params.delete("page_size");
  const selectionQuery = params.toString();
  return selectionQuery ? `/api/assets/selection?${selectionQuery}` : "/api/assets/selection";
}

export function AssetsPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token, [], listProducts);
  const [productForSellingPoints, setProductForSellingPoints] = useState<string>("");
  const [selectedAsset, setSelectedAsset] = useState<Asset | null>(null);
  const [selectedAssetPosition, setSelectedAssetPosition] = useState<AssetPosition | null>(null);
  const [navigatingAsset, setNavigatingAsset] = useState(false);
  const sellingPoints = useResource<SellingPoint[]>(
    productForSellingPoints ? `/api/products/${productForSellingPoints}/selling-points` : null,
    token,
    [productForSellingPoints],
    listSellingPoints
  );
  const [filters, setFilters] = useState({
    productID: "",
    sellingPointID: "",
    sourceType: "",
    status: "",
    usabilityStatus: "",
    shotSize: "",
    tag: "",
    minDurationMs: "",
    maxDurationMs: "",
    hasAudio: "",
    likelyHasSpeech: "",
    excludeDiscarded: "",
    sortBy: ""
  });
  const [semanticSearchText, setSemanticSearchText] = useState("");
  const [semanticQuery, setSemanticQuery] = useState("");
  const [assetFiltersExpanded, setAssetFiltersExpanded] = useState(false);
  const [assetPage, setAssetPage] = useState(1);
  const [assetPageSize, setAssetPageSize] = useState(20);
  const [frames, setFrames] = useState<AssetFrameSnapshot[]>([]);
  const [speechSegments, setSpeechSegments] = useState<AssetSpeechSegment[]>([]);
  const [semanticPreview, setSemanticPreview] = useState<AssetSemanticPreview | null>(null);
  const [assetEmbeddings, setAssetEmbeddings] = useState<AssetEmbeddingObject[]>([]);
  const [assetSellingPoints, setAssetSellingPoints] = useState<SellingPoint[]>([]);
  const [assetDetailSellingPoints, setAssetDetailSellingPoints] = useState<SellingPoint[]>([]);
  const [framesLoading, setFramesLoading] = useState(false);
  const [speechSegmentsLoading, setSpeechSegmentsLoading] = useState(false);
  const [semanticPreviewLoading, setSemanticPreviewLoading] = useState(false);
  const [assetEmbeddingsLoading, setAssetEmbeddingsLoading] = useState(false);
  const [vectorizingAsset, setVectorizingAsset] = useState(false);
  const [editingAnalysis, setEditingAnalysis] = useState(false);
  const [savingAnalysis, setSavingAnalysis] = useState(false);
  const [updatingArchive, setUpdatingArchive] = useState(false);
  const [selectionMode, setSelectionMode] = useState(false);
  const [selectedAssetIDs, setSelectedAssetIDs] = useState<Set<string>>(new Set());
  const [selectionResultTotal, setSelectionResultTotal] = useState<number | null>(null);
  const [selectingAllAssets, setSelectingAllAssets] = useState(false);
  const [archivingSelectedAssets, setArchivingSelectedAssets] = useState(false);
  const [savingSellingPoints, setSavingSellingPoints] = useState(false);
  const [reviewDirty, setReviewDirty] = useState(false);
  const [sellingPointsDirty, setSellingPointsDirty] = useState(false);
  const [reviewForm] = Form.useForm<AssetReviewPayload>();
  const [sellingPointForm] = Form.useForm<AssetSellingPointPayload>();

  const assetPath = useMemo(() => {
    const params = new URLSearchParams();
    if (filters.productID) {
      params.set("product_id", filters.productID);
    }
    if (filters.sellingPointID) {
      params.set("selling_point_id", filters.sellingPointID);
    }
    if (filters.sourceType) {
      params.set("source_type", filters.sourceType);
    }
    if (filters.status) {
      params.set("status", filters.status);
    }
    if (filters.usabilityStatus) {
      params.set("usability_status", filters.usabilityStatus);
    }
    if (filters.shotSize) {
      params.set("shot_size", filters.shotSize);
    }
    if (filters.tag) {
      params.set("tag", filters.tag);
    }
    if (semanticQuery) {
      params.set("semantic_query", semanticQuery);
    }
    if (filters.minDurationMs) {
      params.set("min_duration_ms", filters.minDurationMs);
    }
    if (filters.maxDurationMs) {
      params.set("max_duration_ms", filters.maxDurationMs);
    }
    if (filters.hasAudio) {
      params.set("has_audio", filters.hasAudio);
    }
    if (filters.likelyHasSpeech) {
      params.set("likely_has_speech", filters.likelyHasSpeech);
    }
    if (filters.excludeDiscarded) {
      params.set("exclude_discarded", filters.excludeDiscarded);
    }
    if (filters.sortBy) {
      params.set("sort_by", filters.sortBy);
    }
    params.set("page", String(assetPage));
    params.set("page_size", String(assetPageSize));
    const query = params.toString();
    return query ? `/api/assets?${query}` : "/api/assets";
  }, [assetPage, assetPageSize, filters, semanticQuery]);

  const assets = useResource<AssetListResponse>(assetPath, token, [assetPath], listAssets);
  const assetSelectionPath = useMemo(() => assetSelectionPathForList(assetPath), [assetPath]);
  const productNameByID = useMemo(() => {
    const map = new Map<string, string>();
    for (const product of products.data ?? []) {
      map.set(product.id, product.name);
    }
    return map;
  }, [products.data]);

  useEffect(() => {
    setSelectedAssetIDs(new Set());
    setSelectionResultTotal(null);
  }, [assetSelectionPath]);

  useEffect(() => {
    if (!selectedAsset) {
      setFrames([]);
      setSpeechSegments([]);
      setSemanticPreview(null);
      setAssetEmbeddings([]);
      setAssetSellingPoints([]);
      setAssetDetailSellingPoints([]);
      return;
    }

    let active = true;
    setFrames([]);
    setSpeechSegments([]);
    setSemanticPreview(null);
    setAssetEmbeddings([]);
    setAssetSellingPoints([]);
    setAssetDetailSellingPoints([]);

    const loadFrames = async () => {
      setFramesLoading(true);
      try {
        const response = await getAssetFrames(selectedAsset.id, token);
        if (active) {
          setFrames(response.frames);
        }
      } catch (error) {
        if (active) {
          setFrames([]);
          message.error(error instanceof Error ? error.message : "加载抽帧预览失败");
        }
      } finally {
        if (active) {
          setFramesLoading(false);
        }
      }
    };

    void loadFrames();

    const loadSemanticPreview = async () => {
      setSemanticPreviewLoading(true);
      try {
        const response = await getSemanticPreview(selectedAsset.id, token);
        if (active) {
          setSemanticPreview(response);
        }
      } catch (error) {
        if (active) {
          setSemanticPreview(null);
          message.error(error instanceof Error ? error.message : "加载开放语义描述失败");
        }
      } finally {
        if (active) {
          setSemanticPreviewLoading(false);
        }
      }
    };

    void loadSemanticPreview();

    const loadAssetEmbeddings = async () => {
      setAssetEmbeddingsLoading(true);
      try {
        const response = await getAssetEmbeddings(selectedAsset.id, token);
        if (active) {
          setAssetEmbeddings(response.items);
        }
      } catch {
        if (active) {
          setAssetEmbeddings([]);
        }
      } finally {
        if (active) {
          setAssetEmbeddingsLoading(false);
        }
      }
    };

    void loadAssetEmbeddings();

    const loadSpeechSegments = async () => {
      if (selectedAsset.source_type !== "talking_head") {
        setSpeechSegments([]);
        setSpeechSegmentsLoading(false);
        return;
      }

      setSpeechSegmentsLoading(true);
      try {
        const response = await getSpeechSegments(selectedAsset.id, token);
        if (active) {
          setSpeechSegments(response);
        }
      } catch (error) {
        if (active) {
          setSpeechSegments([]);
          message.error(error instanceof Error ? error.message : "加载口播句段失败");
        }
      } finally {
        if (active) {
          setSpeechSegmentsLoading(false);
        }
      }
    };

    void loadSpeechSegments();

    const loadAssetSellingPoints = async () => {
      try {
        const response = await getAssetSellingPoints(selectedAsset.id, token);
        if (active) {
          setAssetSellingPoints(response);
          sellingPointForm.setFieldsValue({
            selling_point_ids: response.map((item) => item.id)
          });
          setSellingPointsDirty(false);
        }
      } catch (error) {
        if (active) {
          setAssetSellingPoints([]);
          message.error(error instanceof Error ? error.message : "加载素材卖点失败");
        }
      }
    };

    void loadAssetSellingPoints();

    const loadAssetDetailSellingPoints = async () => {
      try {
        const response = await listSellingPoints(`/api/products/${selectedAsset.product_id}/selling-points`, token);
        if (active) {
          setAssetDetailSellingPoints(response);
        }
      } catch (error) {
        if (active) {
          setAssetDetailSellingPoints([]);
          message.error(error instanceof Error ? error.message : "加载产品卖点失败");
        }
      }
    };

    void loadAssetDetailSellingPoints();
    return () => {
      active = false;
    };
  }, [selectedAsset, token]);

  useEffect(() => {
    if (!selectedAsset) {
      reviewForm.resetFields();
      sellingPointForm.resetFields();
      setEditingAnalysis(false);
      setReviewDirty(false);
      setSellingPointsDirty(false);
      return;
    }

    reviewForm.setFieldsValue({
      scene_description: selectedAsset.scene_description || "",
      action_description: selectedAsset.action_description || "",
      shot_size: selectedAsset.shot_size || "",
      camera_movement: selectedAsset.camera_movement || "",
      subjects: selectedAsset.subjects || [],
      scene_tags: selectedAsset.scene_tags || [],
      quality_tags: selectedAsset.quality_tags || [],
      usability_status: selectedAsset.usability_status || "usable",
      reviewer_notes: selectedAsset.reviewer_notes || ""
    });
    setEditingAnalysis(false);
    setReviewDirty(false);
    setSellingPointsDirty(false);
  }, [reviewForm, selectedAsset, sellingPointForm]);

  useEffect(() => {
    sellingPointForm.setFieldsValue({
      selling_point_ids: assetSellingPoints.map((item) => item.id)
    });
  }, [assetSellingPoints, sellingPointForm]);

  const saveAnalysisReview = async () => {
    if (!selectedAsset) {
      return;
    }

    const values = await reviewForm.validateFields();
    setSavingAnalysis(true);
    try {
      const updated = await saveAssetReview(selectedAsset.id, values, token);
      setSelectedAsset(updated);
      setEditingAnalysis(false);
      setReviewDirty(false);
      await assets.reload();
      try {
        setSemanticPreview(await getSemanticPreview(selectedAsset.id, token));
      } catch {
        message.warning("复核已保存，但开放语义预览刷新失败");
      }
      message.success("素材复核已更新");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "更新素材复核失败");
    } finally {
      setSavingAnalysis(false);
    }
  };

  const updateAssetArchiveState = async (asset: Asset, action: "archive" | "restore") => {
    setUpdatingArchive(true);
    try {
      const updated = await persistAssetArchiveState(asset.id, action, token);
      setSelectedAsset(updated);
      await assets.reload();
      message.success(action === "archive" ? "素材已归档" : "素材已恢复");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "更新素材状态失败");
    } finally {
      setUpdatingArchive(false);
    }
  };

  const saveAssetSellingPoints = async () => {
    if (!selectedAsset) {
      return;
    }

    const values = await sellingPointForm.validateFields();
    setSavingSellingPoints(true);
    try {
      const updated = await persistAssetSellingPoints(selectedAsset.id, values, token);
      setAssetSellingPoints(updated);
      setSellingPointsDirty(false);
      message.success("素材卖点关联已更新");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "更新素材卖点失败");
    } finally {
      setSavingSellingPoints(false);
    }
  };

  const vectorizeSelectedAsset = async () => {
    if (!selectedAsset) {
      return;
    }
    setVectorizingAsset(true);
    try {
      const result = await createAssetEmbeddings(selectedAsset.id, token);
      setAssetEmbeddings(result.objects);
      message.success(`已生成 ${result.objects.length} 个向量对象`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "生成向量失败");
    } finally {
      setVectorizingAsset(false);
    }
  };

  const assetItems = assets.data?.items ?? [];
  const assetTotal = assets.data?.total ?? 0;
  const activeFilterCount = Object.values(filters).filter(Boolean).length;
  const selectedAssetNumber = selectedAssetPosition
    ? (selectedAssetPosition.page - 1) * (assets.data?.page_size ?? assetPageSize) + selectedAssetPosition.index + 1
    : 0;
  const assetNavigationBusy = navigatingAsset || savingAnalysis || updatingArchive || savingSellingPoints || vectorizingAsset;

  const openAssetDetail = (asset: Asset) => {
    const index = assetItems.findIndex((item) => item.id === asset.id);
    setSelectedAssetPosition({
      page: assets.data?.page ?? assetPage,
      index: index >= 0 ? index : 0
    });
    setSelectedAsset(asset);
  };

  const navigateAsset = async (direction: -1 | 1) => {
    if (!selectedAsset || !selectedAssetPosition || assetNavigationBusy) {
      return;
    }
    const pageSize = assets.data?.page_size ?? assetPageSize;
    const pageStart = (selectedAssetPosition.page - 1) * pageSize;
    const selectedIndexOnLoadedPage = (assets.data?.page ?? assetPage) === selectedAssetPosition.page
      ? assetItems.findIndex((item) => item.id === selectedAsset.id)
      : -1;
    const currentNumber = pageStart + (selectedIndexOnLoadedPage >= 0 ? selectedIndexOnLoadedPage : selectedAssetPosition.index);
    const targetNumber = selectedIndexOnLoadedPage < 0 && direction === 1 ? currentNumber : currentNumber + direction;
    if (targetNumber < 0 || targetNumber >= assetTotal) {
      return;
    }
    const targetPage = Math.floor(targetNumber / pageSize) + 1;
    const targetIndex = targetNumber % pageSize;

    setNavigatingAsset(true);
    try {
      let targetItems = assetItems;
      if (targetPage !== (assets.data?.page ?? assetPage)) {
        const response = await listAssets(assetPathForPage(assetPath, targetPage, pageSize), token);
        targetItems = response.items ?? [];
      }
      const targetAsset = targetItems[targetIndex];
      if (!targetAsset) {
        throw new Error("目标素材不存在，列表可能已更新");
      }
      if (targetPage !== assetPage) {
        setAssetPage(targetPage);
      }
      setSelectedAssetPosition({ page: targetPage, index: targetIndex });
      setSelectedAsset(targetAsset);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "切换素材失败");
      await assets.reload();
    } finally {
      setNavigatingAsset(false);
    }
  };

  const requestAssetNavigation = (direction: -1 | 1) => {
    if (!reviewDirty && !sellingPointsDirty) {
      void navigateAsset(direction);
      return;
    }
    Modal.confirm({
      title: "放弃未保存的修改？",
      content: "当前素材的修改尚未保存，切换后将丢失这些内容。",
      okText: "放弃并切换",
      cancelText: "继续编辑",
      onOk: () => {
        setReviewDirty(false);
        setSellingPointsDirty(false);
        void navigateAsset(direction);
      }
    });
  };

  const exitAssetSelectionMode = () => {
    setSelectionMode(false);
    setSelectedAssetIDs(new Set());
    setSelectionResultTotal(null);
  };

  const toggleAssetSelection = (asset: Asset) => {
    if (asset.status === "archived") {
      return;
    }
    setSelectedAssetIDs((current) => {
      const next = new Set(current);
      if (next.has(asset.id)) {
        next.delete(asset.id);
      } else {
        next.add(asset.id);
      }
      return next;
    });
  };

  const togglePageAssetSelection = (assetIDs: string[]) => {
    setSelectedAssetIDs((current) => {
      const next = new Set(current);
      const allSelected = assetIDs.length > 0 && assetIDs.every((assetID) => current.has(assetID));
      assetIDs.forEach((assetID) => allSelected ? next.delete(assetID) : next.add(assetID));
      return next;
    });
  };

  const toggleAllFilteredAssets = async () => {
    const allSelected = selectionResultTotal !== null && selectionResultTotal > 0 && selectedAssetIDs.size === selectionResultTotal;
    if (allSelected) {
      setSelectedAssetIDs(new Set());
      return;
    }
    setSelectingAllAssets(true);
    try {
      const result = await listAssetSelection(assetSelectionPath, token);
      setSelectedAssetIDs(new Set(result.asset_ids));
      setSelectionResultTotal(result.total);
      if (result.total === 0) {
        message.info("当前筛选结果中没有可归档素材");
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : "全选素材失败");
    } finally {
      setSelectingAllAssets(false);
    }
  };

  const archiveSelectedAssetIDs = async (assetIDs: string[]) => {
    setArchivingSelectedAssets(true);
    try {
      const result = await archiveAssets(assetIDs, token);
      const archivedByID = new Map(result.archived.map((asset) => [asset.id, asset]));
      if (selectedAsset) {
        setSelectedAsset(archivedByID.get(selectedAsset.id) ?? selectedAsset);
      }
      await assets.reload();

      const failedIDs = result.failures.map((failure) => failure.asset_id);
      const summary = [`已归档 ${result.archived.length} 项`];
      if (result.skipped_ids.length > 0) {
        summary.push(`跳过 ${result.skipped_ids.length} 项`);
      }
      if (result.failures.length > 0) {
        summary.push(`失败 ${result.failures.length} 项`);
        setSelectedAssetIDs(new Set(failedIDs));
        setSelectionResultTotal(null);
        message.warning(summary.join("，"));
      } else {
        message.success(summary.join("，"));
        exitAssetSelectionMode();
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : "批量归档素材失败");
    } finally {
      setArchivingSelectedAssets(false);
    }
  };

  const confirmArchiveSelectedAssets = () => {
    const assetIDs = Array.from(selectedAssetIDs);
    if (assetIDs.length === 0) {
      return;
    }
    Modal.confirm({
      title: `归档 ${assetIDs.length} 个素材？`,
      content: "归档后素材不会再参与检索和自动剪辑，可以在“已归档”筛选中恢复。",
      okText: "归档",
      cancelText: "取消",
      centered: true,
      onOk: () => archiveSelectedAssetIDs(assetIDs)
    });
  };

  const allFilteredAssetsSelected = selectionResultTotal !== null && selectionResultTotal > 0 && selectedAssetIDs.size === selectionResultTotal;

  return (
    <div data-testid="assets-page" className="asset-library-page">
      <Space direction="vertical" size="middle" className="page-stack asset-library-stack">
        <Card className="asset-filter-card" bodyStyle={{ padding: 12 }}>
          <div className="asset-filter-toolbar">
            <Input.Search
              data-testid="asset-filter-keyword"
              value={semanticSearchText}
              allowClear
              enterButton
              placeholder="一句话搜索素材"
              className="asset-filter-search"
              onChange={(event) => {
                const nextValue = event.target.value;
                setSemanticSearchText(nextValue);
                if (!nextValue.trim()) {
                  setAssetPage(1);
                  setSemanticQuery("");
                }
              }}
              onSearch={(value) => {
                setAssetPage(1);
                setSemanticQuery(value.trim());
              }}
            />
            <Select
              data-testid="asset-filter-product"
              value={filters.productID || undefined}
              placeholder="产品"
              allowClear
              className="asset-filter-select"
              options={(products.data ?? []).map((product) => ({ value: product.id, label: product.name }))}
              onChange={(value) => {
                const nextValue = value ?? "";
                setAssetPage(1);
                setFilters((current) => ({ ...current, productID: nextValue, sellingPointID: "" }));
                setProductForSellingPoints(nextValue);
              }}
            />
            <Button onClick={() => setAssetFiltersExpanded((current) => !current)}>
              {assetFiltersExpanded ? "收起筛选" : `更多筛选${activeFilterCount > 0 ? `(${activeFilterCount})` : ""}`}
            </Button>
            <Button
              data-testid="asset-filter-reset"
              onClick={() => {
                setAssetPage(1);
                setAssetPageSize(20);
                setFilters({
                  productID: "",
                  sellingPointID: "",
                  sourceType: "",
                  status: "",
                  usabilityStatus: "",
                  shotSize: "",
                  tag: "",
                  minDurationMs: "",
                  maxDurationMs: "",
                  hasAudio: "",
                  likelyHasSpeech: "",
                  excludeDiscarded: "",
                  sortBy: ""
                });
                setSemanticSearchText("");
                setSemanticQuery("");
                setProductForSellingPoints("");
              }}
            >
              重置
            </Button>
            <Button data-testid="asset-filter-refresh" onClick={assets.reload} loading={assets.loading}>刷新</Button>
          </div>
          {assetFiltersExpanded ? (
            <div className="asset-filter-advanced">
              <Select
                data-testid="asset-filter-selling-point"
                value={filters.sellingPointID || undefined}
                placeholder="卖点"
                allowClear
                className="asset-filter-select"
                disabled={!filters.productID}
                options={(sellingPoints.data ?? []).map((item) => ({ value: item.id, label: item.title }))}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, sellingPointID: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-source-type"
                value={filters.sourceType || undefined}
                placeholder="素材类型"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "visual_only", label: "纯画面" },
                  { value: "talking_head", label: "口播" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, sourceType: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-status"
                value={filters.status || undefined}
                placeholder="状态"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "ready", label: "已完成" },
                  { value: "failed", label: "失败" },
                  { value: "uploaded", label: "已上传" },
                  { value: "archived", label: "已归档" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, status: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-usability-status"
                value={filters.usabilityStatus || undefined}
                placeholder="可用状态"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "usable", label: "可用" },
                  { value: "needs_review", label: "待复核" },
                  { value: "discarded", label: "废弃" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, usabilityStatus: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-shot-size"
                value={filters.shotSize || undefined}
                placeholder="景别"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "close_up", label: "特写" },
                  { value: "medium_close_up", label: "近景" },
                  { value: "medium_shot", label: "中景" },
                  { value: "full_shot", label: "全景" },
                  { value: "wide_shot", label: "远景" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, shotSize: value ?? "" }));
                }}
              />
              <Input
                data-testid="asset-filter-tag"
                value={filters.tag}
                placeholder="标签"
                className="asset-filter-compact"
                onChange={(event) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, tag: event.target.value }));
                }}
              />
              <Input
                data-testid="asset-filter-min-duration"
                value={filters.minDurationMs}
                placeholder="最小时长 ms"
                className="asset-filter-duration"
                onChange={(event) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, minDurationMs: event.target.value }));
                }}
              />
              <Input
                data-testid="asset-filter-max-duration"
                value={filters.maxDurationMs}
                placeholder="最大时长 ms"
                className="asset-filter-duration"
                onChange={(event) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, maxDurationMs: event.target.value }));
                }}
              />
              <Select
                data-testid="asset-filter-has-audio"
                value={filters.hasAudio || undefined}
                placeholder="音频"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "true", label: "包含音频" },
                  { value: "false", label: "无音频" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, hasAudio: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-likely-has-speech"
                value={filters.likelyHasSpeech || undefined}
                placeholder="人声"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "true", label: "有人声" },
                  { value: "false", label: "无人声" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, likelyHasSpeech: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-exclude-discarded"
                value={filters.excludeDiscarded || undefined}
                placeholder="可用性过滤"
                allowClear
                className="asset-filter-select"
                options={[{ value: "true", label: "排除废弃" }]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, excludeDiscarded: value ?? "" }));
                }}
              />
              <Select
                data-testid="asset-filter-sort"
                value={filters.sortBy || undefined}
                placeholder="排序"
                allowClear
                className="asset-filter-select"
                options={[
                  { value: "updated_at_desc", label: "最近更新时间" },
                  { value: "analyzed_at_desc", label: "最近分析时间" }
                ]}
                onChange={(value) => {
                  setAssetPage(1);
                  setFilters((current) => ({ ...current, sortBy: value ?? "" }));
                }}
              />
            </div>
          ) : null}
        </Card>

        <AssetGrid
          assets={assetItems}
          result={assets.data}
          loading={assets.loading}
          page={assetPage}
          pageSize={assetPageSize}
          semanticQuery={semanticQuery}
          productNameByID={productNameByID}
          selectionMode={selectionMode}
          selectedAssetIDs={selectedAssetIDs}
          selectingAll={selectingAllAssets}
          archivingSelected={archivingSelectedAssets}
          allResultsSelected={allFilteredAssetsSelected}
          onSelect={openAssetDetail}
          onEnterSelectionMode={() => setSelectionMode(true)}
          onExitSelectionMode={exitAssetSelectionMode}
          onToggleSelection={toggleAssetSelection}
          onTogglePageSelection={togglePageAssetSelection}
          onToggleAllResults={() => void toggleAllFilteredAssets()}
          onArchiveSelected={confirmArchiveSelectedAssets}
          onPageChange={(page, pageSize) => {
            setAssetPage(page);
            setAssetPageSize(pageSize);
          }}
        />
      </Space>

      <Modal
        title={selectedAsset ? `素材详情：${assetDisplayTitle(selectedAsset)}` : "素材详情"}
        open={selectedAsset !== null}
        footer={null}
        width="86vw"
        className="asset-detail-modal"
        onCancel={() => {
          setSelectedAsset(null);
          setSelectedAssetPosition(null);
        }}
      >
        {selectedAsset ? (
          <div className="asset-detail-shell" data-testid="asset-detail-modal">
            <div className="asset-detail-workspace">
              <section className="asset-detail-preview">
                <video key={selectedAsset.id} src={assetVideoURL(selectedAsset)} controls preload="metadata" />
                <div className="asset-detail-title-block">
                  <div>
                    <Typography.Title level={4}>{assetDisplayTitle(selectedAsset)}</Typography.Title>
                    <Typography.Text type="secondary">{assetFileDisplayName(selectedAsset)}</Typography.Text>
                  </div>
                  <Space wrap>
                    <Tag>{translateValue(selectedAsset.source_type, sourceTypeLabels)}</Tag>
                    <Tag>{translateValue(selectedAsset.status, assetStatusLabels)}</Tag>
                    {selectedAsset.analysis_status ? <Tag color="blue">{translateValue(selectedAsset.analysis_status, analysisStatusLabels)}</Tag> : null}
                  </Space>
                </div>
                <div className="asset-detail-quick-meta">
                  <span>时长 {formatDuration(selectedAsset.duration_ms)}</span>
                  <span>{selectedAsset.width && selectedAsset.height ? `${selectedAsset.width}x${selectedAsset.height}` : "未知分辨率"}</span>
                  <span>{selectedAsset.has_audio ? "含音频" : "无音频"}</span>
                  <span>{translateValue(selectedAsset.usability_status, usabilityStatusLabels)}</span>
                </div>
              </section>

              <section className="asset-detail-analysis-card">
                <div className="asset-detail-section-head">
                  <div>
                    <Typography.Title level={5}>分析与维护</Typography.Title>
                    <Typography.Text type="secondary">画面标签、可用性和人工复核</Typography.Text>
                  </div>
                  <Space wrap>
                    <div className="asset-detail-navigation" aria-label="素材切换">
                      <Tooltip title="上一条">
                        <Button
                          size="small"
                          icon={<ChevronLeft size={16} />}
                          aria-label="上一条素材"
                          disabled={assetNavigationBusy || selectedAssetNumber <= 1}
                          onClick={() => requestAssetNavigation(-1)}
                        />
                      </Tooltip>
                      <span className="asset-detail-navigation-count">
                        {Math.min(selectedAssetNumber, assetTotal)} / {assetTotal}
                      </span>
                      <Tooltip title="下一条">
                        <Button
                          size="small"
                          icon={<ChevronRight size={16} />}
                          aria-label="下一条素材"
                          disabled={assetNavigationBusy || selectedAssetNumber >= assetTotal}
                          onClick={() => requestAssetNavigation(1)}
                        />
                      </Tooltip>
                    </div>
                    <Divider type="vertical" className="asset-detail-section-divider" />
                    <Button
                      size="small"
                      onClick={() => {
                        if (editingAnalysis) {
                          setEditingAnalysis(false);
                          setReviewDirty(false);
                          return;
                        }
                        setEditingAnalysis(true);
                        setReviewDirty(false);
                        reviewForm.setFieldsValue({
                          scene_description: selectedAsset.scene_description || "",
                          action_description: selectedAsset.action_description || "",
                          shot_size: selectedAsset.shot_size || "",
                          camera_movement: selectedAsset.camera_movement || "",
                          subjects: selectedAsset.subjects || [],
                          scene_tags: selectedAsset.scene_tags || [],
                          quality_tags: selectedAsset.quality_tags || [],
                          usability_status: selectedAsset.usability_status || "usable",
                          reviewer_notes: selectedAsset.reviewer_notes || ""
                        });
                      }}
                    >
                      {editingAnalysis ? "取消编辑" : "编辑标签"}
                    </Button>
                    {editingAnalysis ? (
                      <Button type="primary" size="small" loading={savingAnalysis} onClick={saveAnalysisReview} data-testid="save-asset-review">
                        保存复核
                      </Button>
                    ) : null}
                    {selectedAsset.status === "archived" ? (
                      <Button
                        size="small"
                        loading={updatingArchive}
                        onClick={() => void updateAssetArchiveState(selectedAsset, "restore")}
                        data-testid="restore-asset"
                      >
                        恢复素材
                      </Button>
                    ) : (
                      <Button
                        size="small"
                        danger
                        loading={updatingArchive}
                        onClick={() => void updateAssetArchiveState(selectedAsset, "archive")}
                        data-testid="archive-asset"
                      >
                        归档素材
                      </Button>
                    )}
                  </Space>
                </div>

                {editingAnalysis ? (
                  <Form
                    form={reviewForm}
                    layout="vertical"
                    data-testid="asset-review-form"
                    className="asset-detail-review-form"
                    onValuesChange={() => setReviewDirty(true)}
                  >
                    <Form.Item name="scene_description" label="画面描述">
                      <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item name="action_description" label="动作描述">
                      <Input.TextArea rows={3} placeholder="描述画面中实际发生的动作，例如：人物双手反复拉伸和放松束裤带，展示弹性" />
                    </Form.Item>
                    <div className="asset-detail-form-grid">
                      <Form.Item name="shot_size" label="景别">
                        <Select
                          allowClear
                          options={[
                            { value: "close_up", label: "特写" },
                            { value: "medium_close_up", label: "近景" },
                            { value: "medium_shot", label: "中景" },
                            { value: "full_shot", label: "全景" },
                            { value: "wide_shot", label: "远景" }
                          ]}
                        />
                      </Form.Item>
                      <Form.Item name="camera_movement" label="运镜">
                        <Select
                          allowClear
                          options={[
                            { value: "static", label: "固定机位" },
                            { value: "pan", label: "水平摇镜" },
                            { value: "tilt", label: "垂直摇镜" },
                            { value: "push_in", label: "推进" },
                            { value: "pull_out", label: "拉远" },
                            { value: "tracking", label: "跟拍/平移" },
                            { value: "orbit", label: "环绕" },
                            { value: "zoom", label: "变焦" },
                            { value: "handheld", label: "手持" },
                            { value: "mixed", label: "复合运镜" },
                            { value: "unknown", label: "无法判断" }
                          ]}
                        />
                      </Form.Item>
                    </div>
                    <Form.Item name="subjects" label="主体标签">
                      <Select mode="tags" tokenSeparators={[","]} open={false} />
                    </Form.Item>
                    <Form.Item name="scene_tags" label="场景标签">
                      <Select mode="tags" tokenSeparators={[","]} open={false} />
                    </Form.Item>
                    <Form.Item name="quality_tags" label="质量标签">
                      <Select mode="tags" tokenSeparators={[","]} open={false} />
                    </Form.Item>
                    <Form.Item name="usability_status" label="可用性">
                      <Select
                        options={[
                          { value: "usable", label: "可用" },
                          { value: "needs_review", label: "待复核" },
                          { value: "discarded", label: "废弃" }
                        ]}
                      />
                    </Form.Item>
                    <Form.Item name="reviewer_notes" label="复核备注">
                      <Input.TextArea rows={2} />
                    </Form.Item>
                  </Form>
                ) : (
                  <Descriptions bordered column={1} size="small" data-testid="asset-analysis-panel" className="asset-detail-analysis-summary">
                    <Descriptions.Item label="画面描述">
                      {selectedAsset.scene_description || <Typography.Text type="secondary">暂无分析结果。</Typography.Text>}
                    </Descriptions.Item>
                    <Descriptions.Item label="动作描述">
                      {selectedAsset.action_description || <Typography.Text type="secondary">暂无动作描述。</Typography.Text>}
                    </Descriptions.Item>
                    <Descriptions.Item label="景别">{translateValue(selectedAsset.shot_size, shotSizeLabels)}</Descriptions.Item>
                    <Descriptions.Item label="运镜">{translateValue(selectedAsset.camera_movement, cameraMovementLabels)}</Descriptions.Item>
                    <Descriptions.Item label="主体标签">{renderTagList(selectedAsset.subjects, "暂无主体标签")}</Descriptions.Item>
                    <Descriptions.Item label="场景标签">{renderTagList(selectedAsset.scene_tags, "暂无场景标签")}</Descriptions.Item>
                    <Descriptions.Item label="质量标签">{renderTagList(selectedAsset.quality_tags, "暂无质量问题")}</Descriptions.Item>
                    <Descriptions.Item label="复核备注">
                      {selectedAsset.reviewer_notes || <Typography.Text type="secondary">无</Typography.Text>}
                    </Descriptions.Item>
                  </Descriptions>
                )}
              </section>
            </div>

            <Tabs
              className="asset-detail-tabs"
              defaultActiveKey="basic"
              items={[
                {
                  key: "basic",
                  label: "基础信息",
                  forceRender: true,
                  children: (
                    <div className="asset-detail-tab-panel">
                      <Descriptions bordered column={2} size="small">
                        <Descriptions.Item label="文件">{assetFileDisplayName(selectedAsset)}</Descriptions.Item>
                        <Descriptions.Item label="素材类型">{translateValue(selectedAsset.source_type, sourceTypeLabels)}</Descriptions.Item>
                        <Descriptions.Item label="状态">{translateValue(selectedAsset.status, assetStatusLabels)}</Descriptions.Item>
                        <Descriptions.Item label="分析状态">{translateValue(selectedAsset.analysis_status, analysisStatusLabels)}</Descriptions.Item>
                        <Descriptions.Item label="时长">{formatDuration(selectedAsset.duration_ms)}</Descriptions.Item>
                        <Descriptions.Item label="分辨率">
                          {selectedAsset.width && selectedAsset.height ? `${selectedAsset.width}x${selectedAsset.height}` : "-"}
                        </Descriptions.Item>
                        <Descriptions.Item label="视频编码">{selectedAsset.codec || "-"}</Descriptions.Item>
                        <Descriptions.Item label="是否含音频">{selectedAsset.has_audio ? "是" : "否"}</Descriptions.Item>
                        <Descriptions.Item label="音频编码">{selectedAsset.audio_codec || "-"}</Descriptions.Item>
                        <Descriptions.Item label="码率">{selectedAsset.bitrate_kbps ? `${selectedAsset.bitrate_kbps} kbps` : "-"}</Descriptions.Item>
                        <Descriptions.Item label="人工清洗状态">
                          {translateValue(selectedAsset.manual_clean_status, manualCleanStatusLabels)}
                        </Descriptions.Item>
                        <Descriptions.Item label="可用性">{translateValue(selectedAsset.usability_status, usabilityStatusLabels)}</Descriptions.Item>
                      </Descriptions>
                    </div>
                  )
                },
                {
                  key: "ai",
                  label: "AI 分析",
                  forceRender: true,
                  children: (
                    <div className="asset-detail-tab-panel">
                      <Descriptions bordered column={1} size="small">
                        <Descriptions.Item label="分析错误">
                          {selectedAsset.analysis_error || <Typography.Text type="secondary">无</Typography.Text>}
                        </Descriptions.Item>
                        <Descriptions.Item label="模型原始结果">
                          {selectedAsset.model_result && Object.keys(selectedAsset.model_result).length > 0 ? (
                            <pre className="json-block">{JSON.stringify(selectedAsset.model_result, null, 2)}</pre>
                          ) : (
                            <Typography.Text type="secondary">暂无模型原始输出</Typography.Text>
                          )}
                        </Descriptions.Item>
                        <Descriptions.Item label="模型标准化标签">
                          {selectedAsset.model_labels && Object.keys(selectedAsset.model_labels).length > 0 ? (
                            <pre className="json-block">{JSON.stringify(selectedAsset.model_labels, null, 2)}</pre>
                          ) : (
                            <Typography.Text type="secondary">暂无标准化模型标签</Typography.Text>
                          )}
                        </Descriptions.Item>
                        <Descriptions.Item label="人工覆盖结果">
                          {selectedAsset.review_overrides && Object.keys(selectedAsset.review_overrides).length > 0 ? (
                            <pre className="json-block">{JSON.stringify(selectedAsset.review_overrides, null, 2)}</pre>
                          ) : (
                            <Typography.Text type="secondary">暂无人工覆盖</Typography.Text>
                          )}
                        </Descriptions.Item>
                      </Descriptions>
                    </div>
                  )
                },
                {
                  key: "selling-points",
                  label: "卖点关联",
                  forceRender: true,
                  children: (
                    <div className="asset-detail-tab-panel">
                      <Card
                        title="卖点关联"
                        extra={
                          <Button
                            type="primary"
                            size="small"
                            loading={savingSellingPoints}
                            onClick={saveAssetSellingPoints}
                            data-testid="save-asset-selling-points"
                          >
                            保存卖点关联
                          </Button>
                        }
                      >
                        <Space direction="vertical" className="wide-space">
                          <Form form={sellingPointForm} layout="vertical" onValuesChange={() => setSellingPointsDirty(true)}>
                            <Form.Item name="selling_point_ids" label="关联卖点">
                              <Select
                                mode="multiple"
                                allowClear
                                placeholder="请选择卖点"
                                options={assetDetailSellingPoints.map((item) => ({
                                  value: item.id,
                                  label: item.title
                                }))}
                                data-testid="asset-selling-points-select"
                              />
                            </Form.Item>
                          </Form>
                          <Descriptions bordered column={1} size="small">
                            <Descriptions.Item label="当前关联">
                              {assetSellingPoints.length > 0 ? (
                                <Space wrap size={[6, 6]}>
                                  {assetSellingPoints.map((item) => (
                                    <Tag key={item.id}>{item.title}</Tag>
                                  ))}
                                </Space>
                              ) : (
                                <Typography.Text type="secondary">暂无关联卖点。</Typography.Text>
                              )}
                            </Descriptions.Item>
                          </Descriptions>
                        </Space>
                      </Card>
                    </div>
                  )
                },
                {
                  key: "frames",
                  label: "抽帧预览",
                  forceRender: true,
                  children: (
                    <div className="asset-detail-tab-panel">
                      <Card title="抽帧预览" loading={framesLoading}>
                        {frames.length === 0 ? (
                          <Empty description="暂无抽帧结果" />
                        ) : (
                          <div className="frame-grid">
                            {frames.map((frame) => (
                              <div key={frame.id} className="frame-card" data-testid="frame-card">
                                <img
                                  className="frame-image"
                                  src={`/storage/${encodeURI(frame.storage_key)}`}
                                  alt={`frame-${frame.frame_index}`}
                                />
                                <Typography.Text strong>{formatTimestamp(frame.timestamp_ms)}</Typography.Text>
                                <Typography.Text type="secondary">第 {frame.frame_index} 帧</Typography.Text>
                              </div>
                            ))}
                          </div>
                        )}
                      </Card>
                    </div>
                  )
                },
                ...(selectedAsset.source_type === "talking_head"
                  ? [
                      {
                        key: "speech",
                        label: "口播句段",
                        forceRender: true,
                        children: (
                          <div className="asset-detail-tab-panel">
                            <Card title="口播句段" loading={speechSegmentsLoading}>
                              {speechSegments.length === 0 ? (
                                <Empty description="暂无已入库的口播句段" />
                              ) : (
                                <Table<AssetSpeechSegment>
                                  rowKey="id"
                                  dataSource={speechSegments}
                                  pagination={false}
                                  size="small"
                                  columns={[
                                    {
                                      title: "时间",
                                      render: (_, segment) => `${formatTimestamp(segment.start_ms)} - ${formatTimestamp(segment.end_ms)}`
                                    },
                                    {
                                      title: "文本",
                                      dataIndex: "transcript"
                                    },
                                    {
                                      title: "来源",
                                      dataIndex: "source"
                                    },
                                    {
                                      title: "状态",
                                      dataIndex: "status"
                                    }
                                  ]}
                                />
                              )}
                            </Card>
                          </div>
                        )
                      }
                    ]
                  : []),
                {
                  key: "semantic",
                  label: "向量预览",
                  forceRender: true,
                  children: (
                    <div className="asset-detail-tab-panel">
                      <Card
                        title="开放语义与向量化对象"
                        loading={semanticPreviewLoading}
                        extra={
                          <Button type="primary" loading={vectorizingAsset} onClick={() => void vectorizeSelectedAsset()}>
                            生成/更新向量
                          </Button>
                        }
                      >
                        <Descriptions bordered column={1} size="small">
                          <Descriptions.Item label="开放语义描述">
                            {semanticPreview?.open_semantic_description ? (
                              semanticPreview.open_semantic_description
                            ) : (
                              <Typography.Text type="secondary">暂无开放语义描述</Typography.Text>
                            )}
                          </Descriptions.Item>
                          <Descriptions.Item label="向量化对象预览">
                            {semanticPreview && semanticPreview.embedding_targets.length > 0 ? (
                              <Table<AssetEmbeddingTarget>
                                rowKey="object_id"
                                dataSource={semanticPreview.embedding_targets}
                                pagination={false}
                                size="small"
                                columns={[
                                  {
                                    title: "对象类型",
                                    render: (_, item) => item.object_type
                                  },
                                  {
                                    title: "对象文本",
                                    dataIndex: "text"
                                  },
                                  {
                                    title: "元数据",
                                    render: (_, item) =>
                                      item.metadata && Object.keys(item.metadata).length > 0 ? (
                                        <pre className="json-block">{JSON.stringify(item.metadata, null, 2)}</pre>
                                      ) : (
                                        <Typography.Text type="secondary">无</Typography.Text>
                                      )
                                  }
                                ]}
                              />
                            ) : (
                              <Typography.Text type="secondary">暂无向量化对象预览</Typography.Text>
                            )}
                          </Descriptions.Item>
                          <Descriptions.Item label="已保存向量对象">
                            {assetEmbeddings.length > 0 ? (
                              <Table<AssetEmbeddingObject>
                                rowKey="id"
                                dataSource={assetEmbeddings}
                                loading={assetEmbeddingsLoading}
                                pagination={false}
                                size="small"
                                columns={[
                                  {
                                    title: "对象类型",
                                    dataIndex: "object_type"
                                  },
                                  {
                                    title: "模型",
                                    dataIndex: "model"
                                  },
                                  {
                                    title: "维度",
                                    dataIndex: "dimension"
                                  },
                                  {
                                    title: "状态",
                                    dataIndex: "status"
                                  },
                                  {
                                    title: "更新时间",
                                    dataIndex: "updated_at",
                                    render: (value: string) => new Date(value).toLocaleString()
                                  }
                                ]}
                              />
                            ) : (
                              <Typography.Text type="secondary">{assetEmbeddingsLoading ? "加载中" : "暂未生成向量"}</Typography.Text>
                            )}
                          </Descriptions.Item>
                        </Descriptions>
                      </Card>
                    </div>
                  )
                }
              ]}
            />
          </div>
        ) : null}
      </Modal>
    </div>
  );
}
