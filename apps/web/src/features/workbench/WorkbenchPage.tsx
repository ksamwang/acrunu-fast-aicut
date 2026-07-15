import { useEffect, useMemo, useRef, useState } from "react";
import { Button, Empty, Input, InputNumber, Modal, Select, Tag, Tooltip, Typography, message } from "antd";
import { Check, CheckCircle2, Circle, Clapperboard, Plus, RefreshCw, RotateCcw, Sparkles, X } from "lucide-react";
import { useResource } from "../../shared/hooks/use-resource";
import { formatDuration } from "../../shared/lib/format";
import type { ScriptVariant, WorkbenchDraft } from "../../shared/types/generation";
import type { Product, SellingPoint } from "../../shared/types/product";
import { listProducts, listSellingPoints } from "../products/api";
import {
  clearWorkbenchVariants,
  generatePrototypeScripts,
  loadWorkbenchDraft,
  saveWorkbenchDraft,
  startPrototypeWorks
} from "./prototype-api";
import "./styles.css";

const sourceTypeLabels = {
  visual_only: "纯画面",
  talking_head: "口播",
  mixed: "混合"
};

function estimateDuration(text: string) {
  return Math.max(8000, Math.round((text.replace(/\s/g, "").length / 4.2) * 1000));
}

function activeSellingPoints(points: SellingPoint[]) {
  return points.filter((point) => point.status !== "archived");
}

export function WorkbenchPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token, [], listProducts);
  const [draft, setDraft] = useState<WorkbenchDraft>(() => loadWorkbenchDraft());
  const [customSellingPointInput, setCustomSellingPointInput] = useState("");
  const [generating, setGenerating] = useState(false);
  const [regeneratingVariantID, setRegeneratingVariantID] = useState<string | null>(null);
  const defaultedProductIDRef = useRef("");

  const sellingPoints = useResource<SellingPoint[]>(
    draft.product_id ? `/api/products/${draft.product_id}/selling-points` : null,
    token,
    [draft.product_id],
    listSellingPoints
  );

  const productList = products.data ?? [];
  const selectedProduct = productList.find((product) => product.id === draft.product_id) ?? null;
  const availableSellingPoints = activeSellingPoints(sellingPoints.data ?? []).filter((point) => point.product_id === draft.product_id);
  const selectedSellingPoints = availableSellingPoints.filter((point) => draft.selling_point_ids.includes(point.id));
  const activeVariant = draft.variants.find((variant) => variant.id === draft.active_variant_id) ?? draft.variants[0] ?? null;
  const confirmedVariants = draft.variants.filter((variant) => variant.status === "confirmed");
  const canGenerate = Boolean(selectedProduct) && (selectedSellingPoints.length > 0 || draft.custom_selling_points.length > 0);

  useEffect(() => {
    saveWorkbenchDraft(draft);
  }, [draft]);

  useEffect(() => {
    if (!draft.product_id || sellingPoints.loading || defaultedProductIDRef.current === draft.product_id) {
      return;
    }
    const activePoints = activeSellingPoints(sellingPoints.data ?? []).filter((point) => point.product_id === draft.product_id);
    if (activePoints.length === 0) {
      return;
    }
    defaultedProductIDRef.current = draft.product_id;
    setDraft((current) =>
      current.product_id === draft.product_id
        ? { ...current, selling_point_ids: activePoints.map((point) => point.id) }
        : current
    );
  }, [draft.product_id, sellingPoints.data, sellingPoints.loading]);

  const setProduct = (productID: string) => {
    const apply = () => {
      defaultedProductIDRef.current = "";
      setDraft((current) => ({
        ...clearWorkbenchVariants(current),
        product_id: productID,
        selling_point_ids: [],
        custom_selling_points: []
      }));
    };
    if (draft.variants.length > 0) {
      Modal.confirm({
        title: "切换产品",
        content: "当前文案将被清空。",
        okText: "切换产品",
        cancelText: "取消",
        onOk: apply
      });
      return;
    }
    apply();
  };

  const addCustomSellingPoints = () => {
    const nextValues = customSellingPointInput
      .split(/[，,\n]/)
      .map((value) => value.trim())
      .filter(Boolean);
    if (nextValues.length === 0) {
      return;
    }
    setDraft((current) => ({
      ...current,
      custom_selling_points: Array.from(new Set([...current.custom_selling_points, ...nextValues]))
    }));
    setCustomSellingPointInput("");
  };

  const removeCustomSellingPoint = (value: string) => {
    setDraft((current) => ({
      ...current,
      custom_selling_points: current.custom_selling_points.filter((point) => point !== value)
    }));
  };

  const generateScripts = async () => {
    if (!selectedProduct) {
      message.warning("请选择产品");
      return;
    }
    if (selectedSellingPoints.length === 0 && draft.custom_selling_points.length === 0) {
      message.warning("请至少选择或补充一个卖点");
      return;
    }
    setGenerating(true);
    try {
      const variants = await generatePrototypeScripts({
        product: selectedProduct,
        selling_points: selectedSellingPoints,
        custom_selling_points: draft.custom_selling_points,
        count: draft.variant_count
      });
      setDraft((current) => ({ ...current, variants, active_variant_id: variants[0]?.id ?? "" }));
      message.success(`已生成 ${variants.length} 条文案`);
    } finally {
      setGenerating(false);
    }
  };

  const updateVariant = (variantID: string, update: (variant: ScriptVariant) => ScriptVariant) => {
    setDraft((current) => ({
      ...current,
      variants: current.variants.map((variant) => (variant.id === variantID ? update(variant) : variant))
    }));
  };

  const toggleVariantConfirmed = (variantID: string) => {
    updateVariant(variantID, (variant) => ({
      ...variant,
      status: variant.status === "confirmed" ? "draft" : "confirmed",
      updated_at: new Date().toISOString()
    }));
  };

  const regenerateActiveVariant = async () => {
    if (!selectedProduct || !activeVariant) {
      return;
    }
    setRegeneratingVariantID(activeVariant.id);
    try {
      const [replacement] = await generatePrototypeScripts({
        product: selectedProduct,
        selling_points: selectedSellingPoints,
        custom_selling_points: draft.custom_selling_points,
        count: 1
      });
      if (!replacement) {
        return;
      }
      updateVariant(activeVariant.id, () => ({
        ...replacement,
        id: activeVariant.id,
        order: activeVariant.order,
        status: "draft"
      }));
      message.success("当前文案已重新生成");
    } finally {
      setRegeneratingVariantID(null);
    }
  };

  const startTasks = () => {
    if (!selectedProduct || confirmedVariants.length === 0) {
      return;
    }
    const started = startPrototypeWorks(selectedProduct, confirmedVariants);
    const nextDraft = clearWorkbenchVariants(draft);
    saveWorkbenchDraft(nextDraft);
    setDraft(nextDraft);
    message.success(`已开始 ${started.length} 条任务`);
    window.location.hash = "#/finished";
  };

  const productOptions = useMemo(
    () => productList.filter((product) => product.status !== "archived").map((product) => ({ value: product.id, label: product.name })),
    [productList]
  );

  return (
    <div className="workbench-page" data-testid="workbench-page">
      <section className="workbench-brief" aria-label="任务输入">
        <div className="workbench-field workbench-product-field">
          <Typography.Text className="workbench-field-label">产品</Typography.Text>
          <Select
            data-testid="workbench-product-select"
            value={draft.product_id || undefined}
            placeholder="选择产品"
            options={productOptions}
            loading={products.loading}
            onChange={setProduct}
          />
        </div>
        <div className="workbench-field workbench-points-field">
          <Typography.Text className="workbench-field-label">卖点</Typography.Text>
          <Select
            data-testid="workbench-selling-points-select"
            mode="multiple"
            value={draft.selling_point_ids}
            placeholder={draft.product_id ? "选择卖点" : "先选择产品"}
            disabled={!draft.product_id}
            loading={sellingPoints.loading}
            options={availableSellingPoints.map((point) => ({ value: point.id, label: point.title }))}
            maxTagCount="responsive"
            onChange={(sellingPointIDs) => setDraft((current) => ({ ...current, selling_point_ids: sellingPointIDs }))}
          />
        </div>
        <div className="workbench-field workbench-custom-points-field">
          <Typography.Text className="workbench-field-label">补充卖点</Typography.Text>
          <div className="workbench-custom-point-input">
            <Input
              value={customSellingPointInput}
              placeholder="输入后回车"
              disabled={!draft.product_id}
              onChange={(event) => setCustomSellingPointInput(event.target.value)}
              onPressEnter={addCustomSellingPoints}
            />
            <Tooltip title="添加卖点">
              <Button
                type="text"
                aria-label="添加卖点"
                icon={<Plus size={17} />}
                disabled={!draft.product_id}
                onClick={addCustomSellingPoints}
              />
            </Tooltip>
          </div>
          {draft.custom_selling_points.length > 0 ? (
            <div className="workbench-custom-point-tags">
              {draft.custom_selling_points.map((point) => (
                <Tag
                  key={point}
                  closable
                  closeIcon={<X size={13} />}
                  onClose={() => removeCustomSellingPoint(point)}
                >
                  {point}
                </Tag>
              ))}
            </div>
          ) : null}
        </div>
        <div className="workbench-field workbench-count-field">
          <Typography.Text className="workbench-field-label">条数</Typography.Text>
          <InputNumber
            min={1}
            max={8}
            precision={0}
            value={draft.variant_count}
            onChange={(value) => setDraft((current) => ({ ...current, variant_count: Number(value ?? 1) }))}
          />
        </div>
        <Button
          type="primary"
          className="workbench-generate-button"
          data-testid="workbench-generate"
          icon={<Sparkles size={17} />}
          loading={generating}
          disabled={!canGenerate}
          onClick={() => void generateScripts()}
        >
          生成文案
        </Button>
      </section>

      <main className="workbench-main">
        {draft.variants.length === 0 ? (
          <div className="workbench-empty-state">
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未生成文案" />
          </div>
        ) : (
          <div className="workbench-editor">
            <aside className="workbench-variant-rail" aria-label="文案版本">
              <div className="workbench-rail-header">
                <Typography.Text>文案</Typography.Text>
                <Tooltip title="清空当前文案">
                  <Button
                    type="text"
                    size="small"
                    aria-label="清空当前文案"
                    icon={<RotateCcw size={16} />}
                    onClick={() => setDraft((current) => clearWorkbenchVariants(current))}
                  />
                </Tooltip>
              </div>
              <div className="workbench-variant-list">
                {draft.variants.map((variant) => (
                  <button
                    type="button"
                    key={variant.id}
                    className={`workbench-variant-row${activeVariant?.id === variant.id ? " is-active" : ""}`}
                    onClick={() => setDraft((current) => ({ ...current, active_variant_id: variant.id }))}
                  >
                    <span className="workbench-variant-index">{String(variant.order).padStart(2, "0")}</span>
                    <span className="workbench-variant-copy">
                      <span className="workbench-variant-hook">{variant.hook}</span>
                      <span>{formatDuration(variant.estimated_duration_ms)}</span>
                    </span>
                    {variant.status === "confirmed" ? <CheckCircle2 size={16} /> : <Circle size={16} />}
                  </button>
                ))}
              </div>
            </aside>

            {activeVariant ? (
              <section className="workbench-script-editor" aria-label={`文案 ${activeVariant.order}`}>
                <div className="workbench-script-toolbar">
                  <div>
                    <Typography.Text className="workbench-script-index">文案 {String(activeVariant.order).padStart(2, "0")}</Typography.Text>
                    <Typography.Text type="secondary">预计 {formatDuration(activeVariant.estimated_duration_ms)}</Typography.Text>
                  </div>
                  <div className="workbench-script-actions">
                    <Tooltip title="重新生成当前文案">
                      <Button
                        type="text"
                        aria-label="重新生成当前文案"
                        icon={<RefreshCw size={17} />}
                        loading={regeneratingVariantID === activeVariant.id}
                        onClick={() => void regenerateActiveVariant()}
                      />
                    </Tooltip>
                    <Button
                      type={activeVariant.status === "confirmed" ? "default" : "primary"}
                      icon={<Check size={16} />}
                      onClick={() => toggleVariantConfirmed(activeVariant.id)}
                    >
                      {activeVariant.status === "confirmed" ? "已确认" : "确认文案"}
                    </Button>
                  </div>
                </div>

                <Input.TextArea
                  data-testid="workbench-script-editor"
                  className="workbench-script-text"
                  value={activeVariant.script_text}
                  autoSize={{ minRows: 7, maxRows: 14 }}
                  onChange={(event) => {
                    const scriptText = event.target.value;
                    updateVariant(activeVariant.id, (variant) => ({
                      ...variant,
                      script_text: scriptText,
                      estimated_duration_ms: estimateDuration(scriptText),
                      status: "draft",
                      intent_stale: true,
                      updated_at: new Date().toISOString()
                    }));
                  }}
                />

                <section className="workbench-intent" aria-label="镜头意图">
                  <div className="workbench-intent-heading">
                    <Typography.Text>镜头意图</Typography.Text>
                    {activeVariant.intent_stale ? <Tag color="gold">待刷新</Tag> : null}
                  </div>
                  <Typography.Paragraph>{activeVariant.editing_intent}</Typography.Paragraph>
                  <ol className="workbench-beat-list">
                    {activeVariant.beats.map((beat) => (
                      <li key={beat.id}>
                        <span className="workbench-beat-label">{beat.label}</span>
                        <span className="workbench-beat-copy">
                          <strong>{beat.selling_point}</strong>
                          <span>{beat.visual_goal}</span>
                        </span>
                        <Tag>{sourceTypeLabels[beat.source_type]}</Tag>
                      </li>
                    ))}
                  </ol>
                </section>
              </section>
            ) : null}
          </div>
        )}
      </main>

      <footer className="workbench-footer">
        <Typography.Text>已确认 {confirmedVariants.length} 条</Typography.Text>
        <Button
          type="primary"
          data-testid="workbench-start-tasks"
          icon={<Clapperboard size={17} />}
          disabled={confirmedVariants.length === 0}
          onClick={startTasks}
        >
          开始 {confirmedVariants.length} 条任务
        </Button>
      </footer>
    </div>
  );
}
