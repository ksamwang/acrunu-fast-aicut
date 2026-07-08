package ffmpeg

import "testing"

func TestBitsPerSecondStringToKbps(t *testing.T) {
	if got := bitsPerSecondStringToKbps("3200000"); got != 3200 {
		t.Fatalf("expected 3200, got %d", got)
	}
	if got := bitsPerSecondStringToKbps("invalid"); got != 0 {
		t.Fatalf("expected 0 for invalid input, got %d", got)
	}
}
