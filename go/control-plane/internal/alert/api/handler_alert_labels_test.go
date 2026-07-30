package api

import (
	"reflect"
	"testing"
)

func TestNormalizeAlertLabels(t *testing.T) {
	got := normalizeAlertLabels([]string{" C2通信 ", "c2通信", "", "横向移动", "可疑外联"})
	want := []string{"C2通信", "横向移动", "可疑外联"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeAlertLabels() = %#v, want %#v", got, want)
	}
}

func TestNormalizeAlertLabelsCapsLengthAndCount(t *testing.T) {
	got := normalizeAlertLabels([]string{
		"123456789012345678901234567890123456",
		"2", "3", "4", "5", "6", "7", "8", "9",
	})
	if len(got) != 8 {
		t.Fatalf("expected 8 labels, got %d", len(got))
	}
	if len([]rune(got[0])) != 32 {
		t.Fatalf("expected first label to be capped at 32 runes, got %q", got[0])
	}
}
