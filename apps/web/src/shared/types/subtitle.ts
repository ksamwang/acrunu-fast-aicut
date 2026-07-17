export type OutputRatio = "9:16" | "3:4";

export type SubtitleVerticalPosition = "top" | "center" | "bottom";
export type SubtitleTextAlign = "left" | "center" | "right";

export type SubtitleStyleLayout = {
  width: number;
  height: number;
  fps: number;
  vertical_position: SubtitleVerticalPosition;
  text_align: SubtitleTextAlign;
  vertical_offset_ratio: number;
  vertical_position_ratio?: number;
  max_width_ratio: number;
  font_size_ratio: number;
  max_chars_per_line: number;
};

export type SubtitleStylePreset = {
  id: string;
  name: string;
  font_family: string;
  font_weight: number;
  text_color: string;
  background_color: string;
  background_opacity: number;
  outline_color: string;
  outline_width: number;
  shadow: boolean;
  max_lines: number;
  layouts: Record<OutputRatio, SubtitleStyleLayout>;
  status: "enabled" | "disabled";
  is_default: boolean;
  version: number;
  created_at: string;
  updated_at: string;
};

export type SubtitleStylePresetInput = Omit<SubtitleStylePreset, "id" | "is_default" | "version" | "created_at" | "updated_at">;
