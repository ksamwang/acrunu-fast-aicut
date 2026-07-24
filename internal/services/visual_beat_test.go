package services

import "testing"

func TestVisualBeatDurationClasses(t *testing.T) {
	tests := []struct {
		class      string
		durationMs int
		valid      bool
	}{
		{class: VisualBeatDurationLegacy, durationMs: 500, valid: true},
		{class: VisualBeatDurationBrief, durationMs: 1000, valid: true},
		{class: VisualBeatDurationBrief, durationMs: 2320, valid: true},
		{class: VisualBeatDurationBrief, durationMs: 999, valid: false},
		{class: VisualBeatDurationStandard, durationMs: 1800, valid: true},
		{class: VisualBeatDurationStandard, durationMs: 5000, valid: true},
		{class: VisualBeatDurationStandard, durationMs: 1200, valid: false},
		{class: VisualBeatDurationAction, durationMs: 2800, valid: true},
		{class: VisualBeatDurationAction, durationMs: 7000, valid: true},
		{class: VisualBeatDurationAction, durationMs: 2000, valid: false},
	}
	for _, testCase := range tests {
		if got := isVisualBeatDurationValid(testCase.class, testCase.durationMs); got != testCase.valid {
			t.Fatalf("class %q duration %d validity = %v, want %v", testCase.class, testCase.durationMs, got, testCase.valid)
		}
	}
}
