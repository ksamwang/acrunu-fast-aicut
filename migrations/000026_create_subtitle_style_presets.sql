-- +goose Up
CREATE TABLE IF NOT EXISTS subtitle_style_presets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (length(btrim(name)) > 0),
    font_family TEXT NOT NULL,
    font_weight INTEGER NOT NULL CHECK (font_weight BETWEEN 100 AND 900),
    text_color TEXT NOT NULL,
    background_color TEXT NOT NULL,
    background_opacity NUMERIC(4, 3) NOT NULL CHECK (background_opacity BETWEEN 0 AND 1),
    outline_color TEXT NOT NULL,
    outline_width NUMERIC(5, 2) NOT NULL CHECK (outline_width BETWEEN 0 AND 8),
    shadow BOOLEAN NOT NULL DEFAULT FALSE,
    max_lines INTEGER NOT NULL CHECK (max_lines BETWEEN 1 AND 3),
    layouts JSONB NOT NULL CHECK (jsonb_typeof(layouts) = 'object'),
    status TEXT NOT NULL DEFAULT 'enabled' CHECK (status IN ('enabled', 'disabled')),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_subtitle_style_presets_name
    ON subtitle_style_presets (lower(name));
CREATE UNIQUE INDEX IF NOT EXISTS idx_subtitle_style_presets_single_default
    ON subtitle_style_presets (is_default)
    WHERE is_default;
CREATE INDEX IF NOT EXISTS idx_subtitle_style_presets_status
    ON subtitle_style_presets (status, is_default DESC, created_at ASC);

INSERT INTO subtitle_style_presets (
    name,
    font_family,
    font_weight,
    text_color,
    background_color,
    background_opacity,
    outline_color,
    outline_width,
    shadow,
    max_lines,
    layouts,
    status,
    is_default
)
SELECT
    '信息流白字',
    'Noto Sans CJK SC',
    700,
    '#FFFFFF',
    '#000000',
    0.300,
    '#000000',
    0,
    FALSE,
    2,
    '{
      "9:16": {
        "width": 1080,
        "height": 1920,
        "fps": 30,
        "vertical_position": "bottom",
        "text_align": "center",
        "vertical_offset_ratio": 0.14,
        "max_width_ratio": 0.84,
        "font_size_ratio": 0.054,
        "max_chars_per_line": 16
      },
      "3:4": {
        "width": 1080,
        "height": 1440,
        "fps": 30,
        "vertical_position": "bottom",
        "text_align": "center",
        "vertical_offset_ratio": 0.10,
        "max_width_ratio": 0.88,
        "font_size_ratio": 0.052,
        "max_chars_per_line": 18
      }
    }'::jsonb,
    'enabled',
    TRUE
WHERE NOT EXISTS (SELECT 1 FROM subtitle_style_presets);

-- +goose Down
DROP TABLE IF EXISTS subtitle_style_presets;
