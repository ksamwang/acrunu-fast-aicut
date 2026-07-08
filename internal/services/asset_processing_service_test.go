package services

import (
	"reflect"
	"testing"
)

func TestBuildFrameTimestamps(t *testing.T) {
	tests := []struct {
		name      string
		duration  int
		frameCount int
		want      []int
	}{
		{name: "zero duration", duration: 0, frameCount: 3, want: []int{0}},
		{name: "single frame", duration: 900, frameCount: 1, want: []int{450}},
		{name: "three frames", duration: 1200, frameCount: 3, want: []int{300, 600, 900}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildFrameTimestamps(tt.duration, tt.frameCount)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
