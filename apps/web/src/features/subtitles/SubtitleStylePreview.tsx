import { Fragment, type CSSProperties } from "react";
import type { OutputRatio, SubtitleStylePreset } from "../../shared/types/subtitle";
import "./styles.css";

const previewText = "这款束裤带来帮你";

function hexToRGBA(color: string, opacity: number) {
  const value = color.replace("#", "");
  const red = Number.parseInt(value.slice(0, 2), 16) || 0;
  const green = Number.parseInt(value.slice(2, 4), 16) || 0;
  const blue = Number.parseInt(value.slice(4, 6), 16) || 0;
  return `rgba(${red}, ${green}, ${blue}, ${opacity})`;
}

function previewFontFamily(fontFamily: string) {
  return fontFamily === "Noto Sans CJK SC" ? "Noto Sans SC" : fontFamily;
}

function wrapPreviewText(text: string, maxLines: number, maxCharsPerLine: number) {
  const characters = Array.from(text.trim());
  if (characters.length === 0) {
    return [];
  }
  const lineCount = Math.min(maxLines, Math.max(1, Math.ceil(characters.length / maxCharsPerLine)));
  const charsPerLine = Math.ceil(characters.length / lineCount);
  const lines: string[] = [];
  for (let start = 0; start < characters.length; start += charsPerLine) {
    lines.push(characters.slice(start, start + charsPerLine).join("").trim());
  }
  return lines.filter(Boolean);
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
  const shadowOffset = layout ? 2 * previewWidth / layout.width : 0;
  const shadowOpacity = preset.background_opacity > 0 ? preset.background_opacity : 0.72;
  const textShadow = preset.shadow
    ? `${shadowOffset}px ${shadowOffset}px 0 ${hexToRGBA(preset.background_color, shadowOpacity)}`
    : "none";
  const lines = wrapPreviewText(previewText, preset.max_lines, layout?.max_chars_per_line ?? 16);
  const frameStyle = {
    "--subtitle-preview-aspect": ratio === "9:16" ? "9 / 16" : "3 / 4"
  } as CSSProperties;
  const captionStyle: CSSProperties = {
    width: `${(layout?.max_width_ratio ?? 0.84) * 100}%`,
    color: preset.text_color,
    fontFamily: `"${previewFontFamily(preset.font_family)}", sans-serif`,
    fontWeight: preset.font_weight,
    fontSize: `${(layout?.font_size_ratio ?? 0.054) * previewWidth}px`,
    textAlign: layout?.text_align ?? "center",
    textShadow,
    WebkitTextStroke: rawOutline > 0 ? `${rawOutline}px ${preset.outline_color}` : undefined
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
        style={{ top: `${(layout?.vertical_position_ratio ?? 0.5) * 100}%` }}
      >
        <span className="subtitle-style-preview-caption" style={captionStyle}>
          {lines.map((line, index) => (
            <Fragment key={`${index}-${line}`}>
              {index > 0 ? <br /> : null}
              <span style={captionTextStyle}>{line}</span>
            </Fragment>
          ))}
        </span>
      </div>
      <span className="subtitle-style-preview-ratio">{ratio}</span>
    </div>
  );
}
