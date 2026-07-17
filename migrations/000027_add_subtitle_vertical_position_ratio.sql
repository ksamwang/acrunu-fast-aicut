-- +goose Up
UPDATE subtitle_style_presets
SET layouts = jsonb_set(
    jsonb_set(
        layouts,
        '{9:16,vertical_position_ratio}',
        to_jsonb(CASE COALESCE(layouts #>> '{9:16,vertical_position}', 'bottom')
            WHEN 'top' THEN LEAST(0.95, GREATEST(0.05, COALESCE((layouts #>> '{9:16,vertical_offset_ratio}')::numeric, 0) + 0.05))
            WHEN 'center' THEN 0.50
            ELSE LEAST(0.95, GREATEST(0.05, 1 - COALESCE((layouts #>> '{9:16,vertical_offset_ratio}')::numeric, 0) - 0.05))
        END),
        TRUE
    ),
    '{3:4,vertical_position_ratio}',
    to_jsonb(CASE COALESCE(layouts #>> '{3:4,vertical_position}', 'bottom')
        WHEN 'top' THEN LEAST(0.95, GREATEST(0.05, COALESCE((layouts #>> '{3:4,vertical_offset_ratio}')::numeric, 0) + 0.05))
        WHEN 'center' THEN 0.50
        ELSE LEAST(0.95, GREATEST(0.05, 1 - COALESCE((layouts #>> '{3:4,vertical_offset_ratio}')::numeric, 0) - 0.05))
    END),
    TRUE
)
WHERE layouts ? '9:16' AND layouts ? '3:4';

-- +goose Down
UPDATE subtitle_style_presets
SET layouts = (layouts #- '{9:16,vertical_position_ratio}') #- '{3:4,vertical_position_ratio}';
