import { useEffect, useMemo, useState } from "react";
import { Button, Form, Input, InputNumber, Popconfirm, Segmented, Select, Slider, Switch, Tag, Typography, message } from "antd";
import { Plus, Save, Star, Trash2 } from "lucide-react";
import { useResource } from "../../shared/hooks/use-resource";
import type { OutputRatio, SubtitleStylePreset, SubtitleStylePresetInput } from "../../shared/types/subtitle";
import { deleteSubtitleStylePreset, listSubtitleStylePresets, saveSubtitleStylePreset, setDefaultSubtitleStylePreset } from "./api";
import { SubtitleStylePreview } from "./SubtitleStylePreview";

const ratios: OutputRatio[] = ["9:16", "3:4"];

function createFormDefaults() {
  return {
    name: "新字幕样式",
    font_family: "Noto Sans CJK SC",
    font_weight: 700,
    text_color: "#FFFFFF",
    background_enabled: true,
    background_color: "#000000",
    background_opacity_percent: 30,
    outline_enabled: false,
    outline_color: "#000000",
    outline_width: 2,
    shadow: false,
    max_lines: 2,
    enabled: true,
    layouts: {
      "9:16": {
        width: 1080,
        height: 1920,
        fps: 30,
        vertical_position: "center",
        text_align: "center",
        vertical_offset_percent: 0,
        vertical_position_percent: 82,
        max_width_percent: 84,
        font_size_percent: 5.4,
        max_chars_per_line: 16
      },
      "3:4": {
        width: 1080,
        height: 1440,
        fps: 30,
        vertical_position: "center",
        text_align: "center",
        vertical_offset_percent: 0,
        vertical_position_percent: 84,
        max_width_percent: 88,
        font_size_percent: 5.2,
        max_chars_per_line: 18
      }
    }
  };
}

function snapshotFormValues(values: ReturnType<typeof createFormDefaults>) {
  return {
    ...values,
    layouts: {
      "9:16": { ...values.layouts["9:16"] },
      "3:4": { ...values.layouts["3:4"] }
    }
  };
}

function legacyVerticalPositionRatio(layout: SubtitleStylePreset["layouts"][OutputRatio]) {
  if (layout.vertical_position === "top") {
    return Math.min(0.95, Math.max(0.05, layout.vertical_offset_ratio + 0.05));
  }
  if (layout.vertical_position === "bottom") {
    return Math.min(0.95, Math.max(0.05, 1 - layout.vertical_offset_ratio - 0.05));
  }
  return 0.5;
}

function presetToForm(preset: SubtitleStylePreset): ReturnType<typeof createFormDefaults> {
  const values = createFormDefaults();
  const formLayout = (ratio: OutputRatio) => {
    const layout = preset.layouts[ratio];
    return {
      ...layout,
      vertical_offset_percent: Number((layout.vertical_offset_ratio * 100).toFixed(1)),
      vertical_position_percent: Number(((layout.vertical_position_ratio ?? legacyVerticalPositionRatio(layout)) * 100).toFixed(1)),
      max_width_percent: Number((layout.max_width_ratio * 100).toFixed(1)),
      font_size_percent: Number((layout.font_size_ratio * 100).toFixed(1))
    };
  };
  return {
    ...values,
    name: preset.name,
    font_family: preset.font_family,
    font_weight: preset.font_weight,
    text_color: preset.text_color,
    background_enabled: preset.background_opacity > 0,
    background_color: preset.background_color,
    background_opacity_percent: preset.background_opacity > 0
      ? Math.round(preset.background_opacity * 100)
      : values.background_opacity_percent,
    outline_enabled: preset.outline_width > 0,
    outline_color: preset.outline_color,
    outline_width: preset.outline_width > 0 ? preset.outline_width : values.outline_width,
    shadow: preset.shadow,
    max_lines: preset.max_lines,
    enabled: preset.status === "enabled",
    layouts: {
      "9:16": formLayout("9:16"),
      "3:4": formLayout("3:4")
    }
  };
}

function formToPresetInput(values: ReturnType<typeof createFormDefaults>): SubtitleStylePresetInput {
  return {
    name: values.name.trim(),
    font_family: values.font_family,
    font_weight: values.font_weight,
    text_color: values.text_color.toUpperCase(),
    background_color: values.background_color.toUpperCase(),
    background_opacity: values.background_enabled ? values.background_opacity_percent / 100 : 0,
    outline_color: values.outline_color.toUpperCase(),
    outline_width: values.outline_enabled ? values.outline_width : 0,
    shadow: values.shadow,
    max_lines: values.max_lines,
    status: values.enabled ? "enabled" : "disabled",
    layouts: Object.fromEntries(ratios.map((ratio) => {
      const layout = values.layouts[ratio];
      return [ratio, {
        width: ratio === "9:16" ? 1080 : 1080,
        height: ratio === "9:16" ? 1920 : 1440,
        fps: 30,
        vertical_position: "center",
        text_align: layout.text_align,
        vertical_offset_ratio: 0,
        vertical_position_ratio: layout.vertical_position_percent / 100,
        max_width_ratio: layout.max_width_percent / 100,
        font_size_ratio: layout.font_size_percent / 100,
        max_chars_per_line: layout.max_chars_per_line
      }];
    })) as SubtitleStylePresetInput["layouts"]
  };
}

function previewPreset(values: ReturnType<typeof createFormDefaults>, source?: SubtitleStylePreset): SubtitleStylePreset {
  const input = formToPresetInput(values);
  return {
    ...input,
    id: source?.id ?? "preview",
    is_default: source?.is_default ?? false,
    version: source?.version ?? 1,
    created_at: source?.created_at ?? "",
    updated_at: source?.updated_at ?? ""
  };
}

function VerticalPositionControl({
  value = 50,
  onChange
}: {
  value?: number;
  onChange?: (value: number) => void;
}) {
  const update = (next: number | null) => {
    if (next !== null) {
      onChange?.(next);
    }
  };
  return (
    <div className="subtitle-vertical-position-control">
      <Slider
        ariaLabelForHandle="垂直位置滑块"
        min={5}
        max={95}
        step={1}
        marks={{ 5: "上", 50: "中", 95: "下" }}
        value={value}
        onChange={update}
      />
      <InputNumber
        aria-label="垂直位置数值"
        min={5}
        max={95}
        step={1}
        addonAfter="%"
        value={value}
        onChange={update}
      />
    </div>
  );
}

export function SubtitleStylesSettingsPanel({ token }: { token: string }) {
  const presetsResource = useResource<SubtitleStylePreset[]>("/api/admin/subtitle-presets", token, [], listSubtitleStylePresets);
  const [form] = Form.useForm<ReturnType<typeof createFormDefaults>>();
  const [editingID, setEditingID] = useState<string | null>(null);
  const [layoutRatio, setLayoutRatio] = useState<OutputRatio>("9:16");
  const [saving, setSaving] = useState(false);
  const [previewValues, setPreviewValues] = useState(createFormDefaults);
  const presets = presetsResource.data ?? [];
  const selectedPreset = presets.find((preset) => preset.id === editingID);

  useEffect(() => {
    if (editingID === null && presets.length > 0) {
      setEditingID((presets.find((preset) => preset.is_default) ?? presets[0]).id);
    }
  }, [editingID, presets]);

  useEffect(() => {
    if (selectedPreset) {
      const values = presetToForm(selectedPreset);
      form.setFieldsValue(values);
      setPreviewValues(values);
    }
  }, [form, selectedPreset]);

  const currentPreview = useMemo(() => {
    try {
      return previewPreset(previewValues, selectedPreset);
    } catch {
      return selectedPreset ?? previewPreset(createFormDefaults());
    }
  }, [previewValues, selectedPreset]);

  const createNew = () => {
    const values = createFormDefaults();
    setEditingID("");
    form.setFieldsValue(values);
    setPreviewValues(values);
  };

  const save = async () => {
    await form.validateFields();
    const values = form.getFieldsValue(true);
    setSaving(true);
    try {
      const stored = await saveSubtitleStylePreset(editingID || undefined, formToPresetInput(values), token);
      setEditingID(stored.id);
      await presetsResource.reload();
      message.success("字幕样式已保存");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存字幕样式失败");
    } finally {
      setSaving(false);
    }
  };

  const setDefault = async () => {
    if (!selectedPreset) {
      return;
    }
    try {
      await setDefaultSubtitleStylePreset(selectedPreset.id, token);
      await presetsResource.reload();
      message.success("默认字幕样式已更新");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "设置默认样式失败");
    }
  };

  const remove = async () => {
    if (!selectedPreset) {
      return;
    }
    try {
      await deleteSubtitleStylePreset(selectedPreset.id, token);
      setEditingID(null);
      await presetsResource.reload();
      message.success("字幕样式已删除");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "删除字幕样式失败");
    }
  };

  return (
    <section className="subtitle-settings" data-testid="subtitle-style-settings">
      <aside className="subtitle-preset-rail">
        <div className="subtitle-preset-rail-header">
          <Typography.Text>字幕样式</Typography.Text>
          <Button type="text" aria-label="新增字幕样式" icon={<Plus size={17} />} onClick={createNew} />
        </div>
        <div className="subtitle-preset-list">
          {presets.map((preset) => (
            <button
              type="button"
              className={`subtitle-preset-row${editingID === preset.id ? " is-active" : ""}`}
              key={preset.id}
              onClick={() => setEditingID(preset.id)}
            >
              <span>{preset.name}</span>
              <span>
                {preset.is_default ? <Tag color="green">默认</Tag> : null}
                {preset.status === "disabled" ? <Tag>停用</Tag> : null}
              </span>
            </button>
          ))}
        </div>
      </aside>

      <div className="subtitle-preset-editor">
        <header className="subtitle-preset-editor-header">
          <div>
            <Typography.Text strong>{editingID ? selectedPreset?.name ?? "字幕样式" : "新增字幕样式"}</Typography.Text>
            {selectedPreset ? <Typography.Text type="secondary">v{selectedPreset.version}</Typography.Text> : null}
          </div>
          <div className="subtitle-preset-actions">
            {selectedPreset && !selectedPreset.is_default ? (
              <Button icon={<Star size={15} />} onClick={() => void setDefault()}>设为默认</Button>
            ) : null}
            {selectedPreset && !selectedPreset.is_default ? (
              <Popconfirm title="删除该字幕样式？" onConfirm={() => void remove()}>
                <Button danger aria-label="删除字幕样式" icon={<Trash2 size={15} />} />
              </Popconfirm>
            ) : null}
            <Button type="primary" icon={<Save size={15} />} loading={saving} onClick={() => void save()}>保存</Button>
          </div>
        </header>

        <div className="subtitle-preset-editor-body">
          <Form
            form={form}
            layout="vertical"
            requiredMark={false}
            initialValues={createFormDefaults()}
            onValuesChange={(_, values) => setPreviewValues(snapshotFormValues(values))}
          >
            <div className="subtitle-style-form-grid">
              <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]}>
                <Input maxLength={80} />
              </Form.Item>
              <Form.Item name="font_family" label="字体" rules={[{ required: true }]}>
                <Select options={[{ value: "Noto Sans CJK SC", label: "Noto Sans CJK SC" }]} />
              </Form.Item>
              <Form.Item name="font_weight" label="字重">
                <Select options={[{ value: 400, label: "常规" }, { value: 700, label: "粗体" }]} />
              </Form.Item>
              <Form.Item name="max_lines" label="最大行数">
                <Segmented block options={[1, 2, 3]} />
              </Form.Item>
              <Form.Item name="text_color" label="文字颜色">
                <Input type="color" className="subtitle-color-input" />
              </Form.Item>
              <Form.Item name="background_enabled" label="背景" valuePropName="checked">
                <Switch aria-label="背景" checkedChildren="开启" unCheckedChildren="关闭" />
              </Form.Item>
              <Form.Item name="background_color" label="背景颜色">
                <Input type="color" className="subtitle-color-input" disabled={!previewValues.background_enabled} />
              </Form.Item>
              <Form.Item name="background_opacity_percent" label="背景不透明度">
                <InputNumber min={0} max={100} addonAfter="%" disabled={!previewValues.background_enabled} />
              </Form.Item>
              <Form.Item name="outline_enabled" label="描边" valuePropName="checked">
                <Switch aria-label="描边" checkedChildren="开启" unCheckedChildren="关闭" />
              </Form.Item>
              <Form.Item name="outline_width" label="描边宽度">
                <InputNumber min={0.5} max={8} step={0.5} addonAfter="px" disabled={!previewValues.outline_enabled} />
              </Form.Item>
              <Form.Item name="outline_color" label="描边颜色">
                <Input type="color" className="subtitle-color-input" disabled={!previewValues.outline_enabled} />
              </Form.Item>
              <Form.Item name="shadow" label="阴影" valuePropName="checked">
                <Switch />
              </Form.Item>
              <Form.Item name="enabled" label="启用" valuePropName="checked">
                <Switch disabled={selectedPreset?.is_default} />
              </Form.Item>
            </div>

            <div className="subtitle-layout-editor">
              <div className="subtitle-layout-controls">
                <Segmented<OutputRatio> block value={layoutRatio} options={ratios} onChange={setLayoutRatio} />
                <Typography.Text type="secondary">
                  {layoutRatio === "9:16" ? "1080 × 1920" : "1080 × 1440"} · 30 fps
                </Typography.Text>
                <Form.Item name={["layouts", layoutRatio, "vertical_position_percent"]} label="垂直位置">
                  <VerticalPositionControl />
                </Form.Item>
                <Form.Item name={["layouts", layoutRatio, "text_align"]} label="文字对齐">
                  <Segmented block options={[{ value: "left", label: "左" }, { value: "center", label: "中" }, { value: "right", label: "右" }]} />
                </Form.Item>
                <div className="subtitle-layout-number-grid">
                  <Form.Item name={["layouts", layoutRatio, "max_width_percent"]} label="最大宽度">
                    <InputNumber min={30} max={96} step={1} addonAfter="%" />
                  </Form.Item>
                  <Form.Item name={["layouts", layoutRatio, "font_size_percent"]} label="字号比例">
                    <InputNumber min={2} max={12} step={0.1} addonAfter="%" />
                  </Form.Item>
                  <Form.Item name={["layouts", layoutRatio, "max_chars_per_line"]} label="每行参考字数">
                    <InputNumber min={4} max={40} />
                  </Form.Item>
                </div>
              </div>
              <div className="subtitle-layout-preview">
                <SubtitleStylePreview preset={currentPreview} ratio={layoutRatio} />
              </div>
            </div>
          </Form>
        </div>
      </div>
    </section>
  );
}
