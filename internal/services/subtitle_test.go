package services

import "testing"

func TestSubtitleDisplayTextRemovesPunctuationWithoutChangingWords(t *testing.T) {
	t.Parallel()
	input := "骑车时，裤脚蹭链条？A-B、反光……更安全！"
	want := "骑车时裤脚蹭链条AB反光更安全"
	if got := SubtitleDisplayText(input); got != want {
		t.Fatalf("SubtitleDisplayText() = %q, want %q", got, want)
	}
}
