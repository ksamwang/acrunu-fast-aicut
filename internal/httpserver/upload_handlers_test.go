package httpserver

import (
	"reflect"
	"testing"
)

func TestSplitCommaSeparated(t *testing.T) {
	got := splitCommaSeparated("sp-1, sp-2 ,,sp-3")
	want := []string{"sp-1", "sp-2", "sp-3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestFirstNonEmptyForm(t *testing.T) {
	if got := firstNonEmptyForm("", "cleaned"); got != "cleaned" {
		t.Fatalf("expected fallback cleaned, got %s", got)
	}
	if got := firstNonEmptyForm("manual", "cleaned"); got != "manual" {
		t.Fatalf("expected manual, got %s", got)
	}
}
