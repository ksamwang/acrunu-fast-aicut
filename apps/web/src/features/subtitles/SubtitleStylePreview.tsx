import type { CSSProperties } from "react";
import type { OutputRatio, SubtitleStylePreset } from "../../shared/types/subtitle";
import "./styles.css";

function hexToRGBA(color: string, opacity: number) {
  const value = color.replace("#", "");
  const red = Number.parseInt(value.slice(0, 2), 16) || 0;
  const green = Number.parseInt(value.slice(2, 4), 16) || 0;
  const blue = Number.parseInt(value.slice(4, 6), 16) || 0;
  return `rgba(${red}, ${green}, ${blue}, ${opacity})`;
}

function previewAlignment(position: string) {
  if (position === "top") {
    return "flex-start";
  }
  if (position === "center") {
    return "center";
  }
  return "flex-end";
}

export function SubtitleStylePreview({
  preset,
  ratio,
  compact = false
}: {
  preset: SubtitleStylePreset;
  ratio: OutputRatio;
  compact?: boolean;
}) {
  const layout = preset.layouts[ratio];
  const previewWidth = compact ? 150 : 220;
  const rawOutline = layout ? preset.outline_width * previewWidth / layout.width : 0;
  const outline = rawOutline > 0 ? Math.max(0.5, rawOutline) : 0;
  const textShadow = outline > 0
    ? `-${outline}px 0 ${preset.outline_color}, ${outline}px 0 ${preset.outline_color}, 0 -${outline}px ${preset.outline_color}, 0 ${outline}px ${preset.outline_color}`
    : preset.shadow ? "0 2px 5px rgba(0, 0, 0, 0.72)" : "none";
  const frameStyle = {
    "--subtitle-preview-aspect": ratio === "9:16" ? "9 / 16" : "3 / 4",
    "--subtitle-preview-offset": `${(layout?.vertical_offset_ratio ?? 0.1) * 100}%`
  } as CSSProperties;
  const captionStyle: CSSProperties = {
    width: `${(layout?.max_width_ratio ?? 0.84) * 100}%`,
    color: preset.text_color,
    fontFamily: `"${preset.font_family}", sans-serif`,
    fontWeight: preset.font_weight,
    fontSize: `${Math.max(compact ? 9 : 10, (layout?.font_size_ratio ?? 0.054) * previewWidth)}px`,
    textAlign: layout?.text_align ?? "center",
    textShadow
  };
  const captionTextStyle: CSSProperties = {
    background: hexToRGBA(preset.background_color, preset.background_opacity)
  };

  return (
    <div
      className={`subtitle-style-preview${compact ? " is-compact" : ""}`}
      data-ratio={ratio}
      style={frameStyle}
      aria-label={`${preset.name} ${ratio} 字幕预览`}
    >
      <div className="subtitle-style-preview-scene">
        <span className="subtitle-style-preview-horizon" />
        <span className="subtitle-style-preview-subject" />
      </div>
      <div
        className="subtitle-style-preview-caption-layer"
        style={{ alignItems: previewAlignment(layout?.vertical_position ?? "bottom") }}
      >
        <span className="subtitle-style-preview-caption" style={captionStyle}>
          <span style={captionTextStyle}>骑行裤脚不再蹭链条</span>
          <br />
          <span style={captionTextStyle}>夜间出行更安心</span>
        </span>
      </div>
      <span className="subtitle-style-preview-ratio">{ratio}</span>
    </div>
  );
}
