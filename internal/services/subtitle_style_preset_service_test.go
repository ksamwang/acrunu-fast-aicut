package services

import (
	"context"
	"errors"
	"testing"
)

func TestSubtitleStylePresetServiceResolvesRatioSnapshot(t *testing.T) {
	t.Parallel()
	service := NewSubtitleStylePresetService()
	config, err := service.Resolve(context.Background(), "", OutputRatioThreeFour)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if config.OutputWidth != 1080 || config.OutputHeight != 1440 || config.OutputFPS != 30 {
		t.Fatalf("unexpected output config %#v", config)
	}
	if config.Style.VerticalOffset != 0.10 || config.Style.MaxWidthRatio != 0.88 || config.Style.TextAlign != "center" {
		t.Fatalf("unexpected resolved subtitle style %#v", config.Style)
	}
	snapshot := config.Snapshot()
	if snapshot["output_ratio"] != OutputRatioThreeFour || snapshot["subtitle_preset_id"] == "" {
		t.Fatalf("unexpected snapshot %#v", snapshot)
	}
	style, ok := snapshot["subtitle_style"].(map[string]any)
	if !ok || style["font_family"] != "Noto Sans CJK SC" {
		t.Fatalf("unexpected style snapshot %#v", snapshot["subtitle_style"])
	}
}

func TestSubtitleStylePresetServiceManagesDefaultAndValidation(t *testing.T) {
	t.Parallel()
	service := NewSubtitleStylePresetService()
	input := DefaultSubtitleStylePresetInput()
	input.Name = "无背景白字"
	input.BackgroundOpacity = 0
	created, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.IsDefault || created.Version != 1 {
		t.Fatalf("unexpected created preset %#v", created)
	}
	created, err = service.SetDefault(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("SetDefault() error = %v", err)
	}
	if !created.IsDefault {
		t.Fatalf("expected created preset to be default %#v", created)
	}
	if err := service.Delete(context.Background(), created.ID); !errors.Is(err, ErrSubtitleStylePresetDefault) {
		t.Fatalf("Delete(default) error = %v", err)
	}
	input.Layouts[OutputRatioNineSixteen] = SubtitleStyleLayout{}
	if _, err := service.Create(context.Background(), input); err == nil {
		t.Fatal("expected invalid layout to be rejected")
	}
}
