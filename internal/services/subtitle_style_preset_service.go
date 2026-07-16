package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	OutputRatioNineSixteen = "9:16"
	OutputRatioThreeFour   = "3:4"
)

var (
	ErrSubtitleStylePresetNotFound = errors.New("subtitle style preset not found")
	ErrSubtitleStylePresetConflict = errors.New("subtitle style preset conflict")
	ErrSubtitleStylePresetDefault  = errors.New("default subtitle style preset cannot be disabled or deleted")
	subtitleHexColorPattern        = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

type SubtitleStyleLayout struct {
	Width               int     `json:"width"`
	Height              int     `json:"height"`
	FPS                 int     `json:"fps"`
	VerticalPosition    string  `json:"vertical_position"`
	TextAlign           string  `json:"text_align"`
	VerticalOffsetRatio float64 `json:"vertical_offset_ratio"`
	MaxWidthRatio       float64 `json:"max_width_ratio"`
	FontSizeRatio       float64 `json:"font_size_ratio"`
	MaxCharsPerLine     int     `json:"max_chars_per_line"`
}

type SubtitleStylePreset struct {
	ID                string                         `json:"id"`
	Name              string                         `json:"name"`
	FontFamily        string                         `json:"font_family"`
	FontWeight        int                            `json:"font_weight"`
	TextColor         string                         `json:"text_color"`
	BackgroundColor   string                         `json:"background_color"`
	BackgroundOpacity float64                        `json:"background_opacity"`
	OutlineColor      string                         `json:"outline_color"`
	OutlineWidth      float64                        `json:"outline_width"`
	Shadow            bool                           `json:"shadow"`
	MaxLines          int                            `json:"max_lines"`
	Layouts           map[string]SubtitleStyleLayout `json:"layouts"`
	Status            string                         `json:"status"`
	IsDefault         bool                           `json:"is_default"`
	Version           int                            `json:"version"`
	CreatedAt         time.Time                      `json:"created_at"`
	UpdatedAt         time.Time                      `json:"updated_at"`
}

type SubtitleStylePresetInput struct {
	Name              string                         `json:"name"`
	FontFamily        string                         `json:"font_family"`
	FontWeight        int                            `json:"font_weight"`
	TextColor         string                         `json:"text_color"`
	BackgroundColor   string                         `json:"background_color"`
	BackgroundOpacity float64                        `json:"background_opacity"`
	OutlineColor      string                         `json:"outline_color"`
	OutlineWidth      float64                        `json:"outline_width"`
	Shadow            bool                           `json:"shadow"`
	MaxLines          int                            `json:"max_lines"`
	Layouts           map[string]SubtitleStyleLayout `json:"layouts"`
	Status            string                         `json:"status"`
}

type ResolvedSubtitleStyle struct {
	FontFamily        string  `json:"font_family"`
	FontWeight        int     `json:"font_weight"`
	TextColor         string  `json:"text_color"`
	BackgroundColor   string  `json:"background_color"`
	BackgroundOpacity float64 `json:"background_opacity"`
	OutlineColor      string  `json:"outline_color"`
	OutlineWidth      float64 `json:"outline_width"`
	Shadow            bool    `json:"shadow"`
	MaxLines          int     `json:"max_lines"`
	VerticalPosition  string  `json:"vertical_position"`
	TextAlign         string  `json:"text_align"`
	VerticalOffset    float64 `json:"vertical_offset_ratio"`
	MaxWidthRatio     float64 `json:"max_width_ratio"`
	FontSizeRatio     float64 `json:"font_size_ratio"`
	MaxCharsPerLine   int     `json:"max_chars_per_line"`
}

type SubtitleRenderConfig struct {
	OutputRatio   string                `json:"output_ratio"`
	OutputWidth   int                   `json:"output_width"`
	OutputHeight  int                   `json:"output_height"`
	OutputFPS     int                   `json:"output_fps"`
	PresetID      string                `json:"subtitle_preset_id"`
	PresetName    string                `json:"subtitle_preset_name"`
	PresetVersion int                   `json:"subtitle_preset_version"`
	Style         ResolvedSubtitleStyle `json:"subtitle_style"`
}

func (c SubtitleRenderConfig) Snapshot() map[string]any {
	styleBytes, _ := json.Marshal(c.Style)
	style := map[string]any{}
	_ = json.Unmarshal(styleBytes, &style)
	return map[string]any{
		"output_ratio":            c.OutputRatio,
		"output_width":            c.OutputWidth,
		"output_height":           c.OutputHeight,
		"output_fps":              c.OutputFPS,
		"subtitle_preset_id":      c.PresetID,
		"subtitle_preset_name":    c.PresetName,
		"subtitle_preset_version": c.PresetVersion,
		"subtitle_style":          style,
	}
}

type subtitlePresetScanner interface {
	Scan(dest ...any) error
}

type SubtitleStylePresetService struct {
	pool      *pgxpool.Pool
	mu        sync.RWMutex
	memory    map[string]SubtitleStylePreset
	defaultID string
}

func NewSubtitleStylePresetService() *SubtitleStylePresetService {
	preset := defaultSubtitleStylePreset()
	return &SubtitleStylePresetService{
		memory:    map[string]SubtitleStylePreset{preset.ID: preset},
		defaultID: preset.ID,
	}
}

func NewSubtitleStylePresetServiceWithPool(pool *pgxpool.Pool) *SubtitleStylePresetService {
	if pool == nil {
		return NewSubtitleStylePresetService()
	}
	return &SubtitleStylePresetService{pool: pool}
}

func DefaultSubtitleStylePresetInput() SubtitleStylePresetInput {
	preset := defaultSubtitleStylePreset()
	return SubtitleStylePresetInput{
		Name: preset.Name, FontFamily: preset.FontFamily, FontWeight: preset.FontWeight,
		TextColor: preset.TextColor, BackgroundColor: preset.BackgroundColor,
		BackgroundOpacity: preset.BackgroundOpacity, OutlineColor: preset.OutlineColor,
		OutlineWidth: preset.OutlineWidth, Shadow: preset.Shadow, MaxLines: preset.MaxLines,
		Layouts: cloneSubtitleLayouts(preset.Layouts), Status: preset.Status,
	}
}

func (s *SubtitleStylePresetService) List(ctx context.Context, includeDisabled bool) ([]SubtitleStylePreset, error) {
	if s.pool == nil {
		s.mu.RLock()
		items := make([]SubtitleStylePreset, 0, len(s.memory))
		for _, preset := range s.memory {
			if includeDisabled || preset.Status == "enabled" {
				items = append(items, cloneSubtitlePreset(preset))
			}
		}
		s.mu.RUnlock()
		sort.Slice(items, func(i, j int) bool {
			if items[i].IsDefault != items[j].IsDefault {
				return items[i].IsDefault
			}
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		})
		return items, nil
	}
	query := subtitlePresetSelect + " ORDER BY is_default DESC, created_at ASC"
	if !includeDisabled {
		query = subtitlePresetSelect + " WHERE status = 'enabled' ORDER BY is_default DESC, created_at ASC"
	}
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SubtitleStylePreset{}
	for rows.Next() {
		preset, scanErr := scanSubtitlePreset(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, preset)
	}
	return items, rows.Err()
}

func (s *SubtitleStylePresetService) Get(ctx context.Context, presetID string) (SubtitleStylePreset, error) {
	presetID = strings.TrimSpace(presetID)
	if presetID == "" {
		return SubtitleStylePreset{}, ErrSubtitleStylePresetNotFound
	}
	if s.pool == nil {
		s.mu.RLock()
		preset, ok := s.memory[presetID]
		s.mu.RUnlock()
		if !ok {
			return SubtitleStylePreset{}, ErrSubtitleStylePresetNotFound
		}
		return cloneSubtitlePreset(preset), nil
	}
	preset, err := scanSubtitlePreset(s.pool.QueryRow(ctx, subtitlePresetSelect+" WHERE id = $1::uuid", presetID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SubtitleStylePreset{}, ErrSubtitleStylePresetNotFound
	}
	return preset, err
}

func (s *SubtitleStylePresetService) Default(ctx context.Context) (SubtitleStylePreset, error) {
	if s.pool == nil {
		s.mu.RLock()
		preset, ok := s.memory[s.defaultID]
		s.mu.RUnlock()
		if !ok {
			return SubtitleStylePreset{}, ErrSubtitleStylePresetNotFound
		}
		return cloneSubtitlePreset(preset), nil
	}
	preset, err := scanSubtitlePreset(s.pool.QueryRow(ctx, subtitlePresetSelect+" WHERE is_default = TRUE AND status = 'enabled' LIMIT 1"))
	if errors.Is(err, pgx.ErrNoRows) {
		return SubtitleStylePreset{}, ErrSubtitleStylePresetNotFound
	}
	return preset, err
}

func (s *SubtitleStylePresetService) Create(ctx context.Context, input SubtitleStylePresetInput) (SubtitleStylePreset, error) {
	input = normalizeSubtitlePresetInput(input)
	if err := validateSubtitlePresetInput(input); err != nil {
		return SubtitleStylePreset{}, err
	}
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, existing := range s.memory {
			if strings.EqualFold(existing.Name, input.Name) {
				return SubtitleStylePreset{}, ErrSubtitleStylePresetConflict
			}
		}
		now := time.Now()
		preset := subtitlePresetFromInput(uuid.NewString(), input, false, 1, now, now)
		s.memory[preset.ID] = preset
		return cloneSubtitlePreset(preset), nil
	}
	layouts, _ := json.Marshal(input.Layouts)
	preset, err := scanSubtitlePreset(s.pool.QueryRow(ctx, `
		INSERT INTO subtitle_style_presets (
			name, font_family, font_weight, text_color, background_color, background_opacity,
			outline_color, outline_width, shadow, max_lines, layouts, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12)
		RETURNING `+subtitlePresetColumns,
		input.Name, input.FontFamily, input.FontWeight, input.TextColor, input.BackgroundColor,
		input.BackgroundOpacity, input.OutlineColor, input.OutlineWidth, input.Shadow,
		input.MaxLines, layouts, input.Status,
	))
	if isUniqueViolation(err) {
		return SubtitleStylePreset{}, ErrSubtitleStylePresetConflict
	}
	return preset, err
}

func (s *SubtitleStylePresetService) Update(ctx context.Context, presetID string, input SubtitleStylePresetInput) (SubtitleStylePreset, error) {
	input = normalizeSubtitlePresetInput(input)
	if err := validateSubtitlePresetInput(input); err != nil {
		return SubtitleStylePreset{}, err
	}
	current, err := s.Get(ctx, presetID)
	if err != nil {
		return SubtitleStylePreset{}, err
	}
	if current.IsDefault && input.Status != "enabled" {
		return SubtitleStylePreset{}, ErrSubtitleStylePresetDefault
	}
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for id, existing := range s.memory {
			if id != presetID && strings.EqualFold(existing.Name, input.Name) {
				return SubtitleStylePreset{}, ErrSubtitleStylePresetConflict
			}
		}
		updated := subtitlePresetFromInput(current.ID, input, current.IsDefault, current.Version+1, current.CreatedAt, time.Now())
		s.memory[current.ID] = updated
		return cloneSubtitlePreset(updated), nil
	}
	layouts, _ := json.Marshal(input.Layouts)
	preset, err := scanSubtitlePreset(s.pool.QueryRow(ctx, `
		UPDATE subtitle_style_presets SET
			name = $2, font_family = $3, font_weight = $4, text_color = $5,
			background_color = $6, background_opacity = $7, outline_color = $8,
			outline_width = $9, shadow = $10, max_lines = $11, layouts = $12::jsonb,
			status = $13, version = version + 1, updated_at = now()
		WHERE id = $1::uuid
		RETURNING `+subtitlePresetColumns,
		presetID, input.Name, input.FontFamily, input.FontWeight, input.TextColor,
		input.BackgroundColor, input.BackgroundOpacity, input.OutlineColor, input.OutlineWidth,
		input.Shadow, input.MaxLines, layouts, input.Status,
	))
	if isUniqueViolation(err) {
		return SubtitleStylePreset{}, ErrSubtitleStylePresetConflict
	}
	return preset, err
}

func (s *SubtitleStylePresetService) SetDefault(ctx context.Context, presetID string) (SubtitleStylePreset, error) {
	preset, err := s.Get(ctx, presetID)
	if err != nil {
		return SubtitleStylePreset{}, err
	}
	if preset.Status != "enabled" {
		return SubtitleStylePreset{}, fmt.Errorf("default subtitle style preset must be enabled")
	}
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for id, existing := range s.memory {
			existing.IsDefault = id == presetID
			existing.UpdatedAt = time.Now()
			s.memory[id] = existing
		}
		s.defaultID = presetID
		return cloneSubtitlePreset(s.memory[presetID]), nil
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SubtitleStylePreset{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "UPDATE subtitle_style_presets SET is_default = FALSE WHERE is_default = TRUE"); err != nil {
		return SubtitleStylePreset{}, err
	}
	updated, err := scanSubtitlePreset(tx.QueryRow(ctx, "UPDATE subtitle_style_presets SET is_default = TRUE, updated_at = now() WHERE id = $1::uuid AND status = 'enabled' RETURNING "+subtitlePresetColumns, presetID))
	if err != nil {
		return SubtitleStylePreset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SubtitleStylePreset{}, err
	}
	return updated, nil
}

func (s *SubtitleStylePresetService) Delete(ctx context.Context, presetID string) error {
	preset, err := s.Get(ctx, presetID)
	if err != nil {
		return err
	}
	if preset.IsDefault {
		return ErrSubtitleStylePresetDefault
	}
	if s.pool == nil {
		s.mu.Lock()
		delete(s.memory, presetID)
		s.mu.Unlock()
		return nil
	}
	command, err := s.pool.Exec(ctx, "DELETE FROM subtitle_style_presets WHERE id = $1::uuid AND is_default = FALSE", presetID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrSubtitleStylePresetNotFound
	}
	return nil
}

func (s *SubtitleStylePresetService) Resolve(ctx context.Context, presetID string, outputRatio string) (SubtitleRenderConfig, error) {
	outputRatio = normalizeOutputRatio(outputRatio)
	if outputRatio == "" {
		return SubtitleRenderConfig{}, fmt.Errorf("output_ratio must be 9:16 or 3:4")
	}
	var preset SubtitleStylePreset
	var err error
	if strings.TrimSpace(presetID) == "" {
		preset, err = s.Default(ctx)
	} else {
		preset, err = s.Get(ctx, presetID)
	}
	if err != nil {
		return SubtitleRenderConfig{}, err
	}
	if preset.Status != "enabled" {
		return SubtitleRenderConfig{}, fmt.Errorf("subtitle style preset is disabled")
	}
	layout, ok := preset.Layouts[outputRatio]
	if !ok {
		return SubtitleRenderConfig{}, fmt.Errorf("subtitle style preset does not support output ratio %s", outputRatio)
	}
	return SubtitleRenderConfig{
		OutputRatio: outputRatio, OutputWidth: layout.Width, OutputHeight: layout.Height, OutputFPS: layout.FPS,
		PresetID: preset.ID, PresetName: preset.Name, PresetVersion: preset.Version,
		Style: ResolvedSubtitleStyle{
			FontFamily: preset.FontFamily, FontWeight: preset.FontWeight, TextColor: preset.TextColor,
			BackgroundColor: preset.BackgroundColor, BackgroundOpacity: preset.BackgroundOpacity,
			OutlineColor: preset.OutlineColor, OutlineWidth: preset.OutlineWidth, Shadow: preset.Shadow,
			MaxLines: preset.MaxLines, VerticalPosition: layout.VerticalPosition, TextAlign: layout.TextAlign,
			VerticalOffset: layout.VerticalOffsetRatio, MaxWidthRatio: layout.MaxWidthRatio,
			FontSizeRatio: layout.FontSizeRatio, MaxCharsPerLine: layout.MaxCharsPerLine,
		},
	}, nil
}

const subtitlePresetColumns = `id::text, name, font_family, font_weight, text_color, background_color,
	background_opacity::float8, outline_color, outline_width::float8, shadow, max_lines, layouts,
	status, is_default, version, created_at, updated_at`
const subtitlePresetSelect = "SELECT " + subtitlePresetColumns + " FROM subtitle_style_presets"

func scanSubtitlePreset(row subtitlePresetScanner) (SubtitleStylePreset, error) {
	var preset SubtitleStylePreset
	var layouts []byte
	if err := row.Scan(
		&preset.ID, &preset.Name, &preset.FontFamily, &preset.FontWeight, &preset.TextColor,
		&preset.BackgroundColor, &preset.BackgroundOpacity, &preset.OutlineColor, &preset.OutlineWidth,
		&preset.Shadow, &preset.MaxLines, &layouts, &preset.Status, &preset.IsDefault, &preset.Version,
		&preset.CreatedAt, &preset.UpdatedAt,
	); err != nil {
		return SubtitleStylePreset{}, err
	}
	if err := json.Unmarshal(layouts, &preset.Layouts); err != nil {
		return SubtitleStylePreset{}, err
	}
	return preset, nil
}

func defaultSubtitleStylePreset() SubtitleStylePreset {
	now := time.Now()
	return SubtitleStylePreset{
		ID: uuid.NewString(), Name: "信息流白字", FontFamily: "Noto Sans CJK SC", FontWeight: 700,
		TextColor: "#FFFFFF", BackgroundColor: "#000000", BackgroundOpacity: 0.3,
		OutlineColor: "#000000", OutlineWidth: 0, Shadow: false, MaxLines: 2,
		Layouts: map[string]SubtitleStyleLayout{
			OutputRatioNineSixteen: {Width: 1080, Height: 1920, FPS: 30, VerticalPosition: "bottom", TextAlign: "center", VerticalOffsetRatio: 0.14, MaxWidthRatio: 0.84, FontSizeRatio: 0.054, MaxCharsPerLine: 16},
			OutputRatioThreeFour:   {Width: 1080, Height: 1440, FPS: 30, VerticalPosition: "bottom", TextAlign: "center", VerticalOffsetRatio: 0.10, MaxWidthRatio: 0.88, FontSizeRatio: 0.052, MaxCharsPerLine: 18},
		},
		Status: "enabled", IsDefault: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
}

func normalizeSubtitlePresetInput(input SubtitleStylePresetInput) SubtitleStylePresetInput {
	input.Name = strings.TrimSpace(input.Name)
	input.FontFamily = strings.TrimSpace(input.FontFamily)
	input.TextColor = strings.ToUpper(strings.TrimSpace(input.TextColor))
	input.BackgroundColor = strings.ToUpper(strings.TrimSpace(input.BackgroundColor))
	input.OutlineColor = strings.ToUpper(strings.TrimSpace(input.OutlineColor))
	input.Status = strings.TrimSpace(input.Status)
	if input.Status == "" {
		input.Status = "enabled"
	}
	return input
}

func validateSubtitlePresetInput(input SubtitleStylePresetInput) error {
	if input.Name == "" || len([]rune(input.Name)) > 80 {
		return fmt.Errorf("subtitle preset name is required and must not exceed 80 characters")
	}
	if input.FontFamily == "" || strings.ContainsAny(input.FontFamily, ",\r\n") {
		return fmt.Errorf("subtitle font_family is invalid")
	}
	if input.FontWeight < 100 || input.FontWeight > 900 {
		return fmt.Errorf("subtitle font_weight must be between 100 and 900")
	}
	for name, value := range map[string]string{"text_color": input.TextColor, "background_color": input.BackgroundColor, "outline_color": input.OutlineColor} {
		if !subtitleHexColorPattern.MatchString(value) {
			return fmt.Errorf("subtitle %s must use #RRGGBB", name)
		}
	}
	if input.BackgroundOpacity < 0 || input.BackgroundOpacity > 1 || input.OutlineWidth < 0 || input.OutlineWidth > 8 {
		return fmt.Errorf("subtitle opacity or outline width is out of range")
	}
	if input.MaxLines < 1 || input.MaxLines > 3 {
		return fmt.Errorf("subtitle max_lines must be between 1 and 3")
	}
	if input.Status != "enabled" && input.Status != "disabled" {
		return fmt.Errorf("subtitle preset status must be enabled or disabled")
	}
	for _, ratio := range []string{OutputRatioNineSixteen, OutputRatioThreeFour} {
		layout, ok := input.Layouts[ratio]
		if !ok {
			return fmt.Errorf("subtitle layout %s is required", ratio)
		}
		if err := validateSubtitleLayout(ratio, layout); err != nil {
			return err
		}
	}
	return nil
}

func validateSubtitleLayout(ratio string, layout SubtitleStyleLayout) error {
	wantHeight := 1920
	if ratio == OutputRatioThreeFour {
		wantHeight = 1440
	}
	if layout.Width != 1080 || layout.Height != wantHeight || layout.FPS < 1 || layout.FPS > 60 {
		return fmt.Errorf("subtitle layout %s dimensions or fps are invalid", ratio)
	}
	if layout.VerticalPosition != "bottom" && layout.VerticalPosition != "center" && layout.VerticalPosition != "top" {
		return fmt.Errorf("subtitle layout %s vertical_position is invalid", ratio)
	}
	if layout.TextAlign != "left" && layout.TextAlign != "center" && layout.TextAlign != "right" {
		return fmt.Errorf("subtitle layout %s text_align is invalid", ratio)
	}
	if layout.VerticalOffsetRatio < 0 || layout.VerticalOffsetRatio > 0.4 || layout.MaxWidthRatio < 0.3 || layout.MaxWidthRatio > 0.96 || layout.FontSizeRatio < 0.02 || layout.FontSizeRatio > 0.12 {
		return fmt.Errorf("subtitle layout %s ratios are out of range", ratio)
	}
	if layout.MaxCharsPerLine < 4 || layout.MaxCharsPerLine > 40 {
		return fmt.Errorf("subtitle layout %s max_chars_per_line must be between 4 and 40", ratio)
	}
	return nil
}

func normalizeOutputRatio(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return OutputRatioNineSixteen
	}
	if value == OutputRatioNineSixteen || value == OutputRatioThreeFour {
		return value
	}
	return ""
}

func subtitlePresetFromInput(id string, input SubtitleStylePresetInput, isDefault bool, version int, createdAt time.Time, updatedAt time.Time) SubtitleStylePreset {
	return SubtitleStylePreset{
		ID: id, Name: input.Name, FontFamily: input.FontFamily, FontWeight: input.FontWeight,
		TextColor: input.TextColor, BackgroundColor: input.BackgroundColor,
		BackgroundOpacity: input.BackgroundOpacity, OutlineColor: input.OutlineColor,
		OutlineWidth: input.OutlineWidth, Shadow: input.Shadow, MaxLines: input.MaxLines,
		Layouts: cloneSubtitleLayouts(input.Layouts), Status: input.Status, IsDefault: isDefault,
		Version: version, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}

func cloneSubtitlePreset(preset SubtitleStylePreset) SubtitleStylePreset {
	preset.Layouts = cloneSubtitleLayouts(preset.Layouts)
	return preset
}

func cloneSubtitleLayouts(layouts map[string]SubtitleStyleLayout) map[string]SubtitleStyleLayout {
	result := make(map[string]SubtitleStyleLayout, len(layouts))
	for ratio, layout := range layouts {
		result[ratio] = layout
	}
	return result
}

func isUniqueViolation(err error) bool {
	var databaseError *pgconn.PgError
	return errors.As(err, &databaseError) && databaseError.Code == "23505"
}
