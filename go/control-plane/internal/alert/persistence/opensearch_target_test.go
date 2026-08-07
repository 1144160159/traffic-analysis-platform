package persistence

import (
	"testing"
	"time"
)

func TestOpenSearchWriterUsesExactAliasWhenV2Enabled(t *testing.T) {
	w := &OpenSearchWriter{writeTarget: "alerts-v2-write", exactTarget: true}
	if got := w.targetFor(time.Date(2026, 8, 4, 1, 2, 3, 0, time.UTC)); got != "alerts-v2-write" {
		t.Fatalf("targetFor() = %q", got)
	}
}

func TestOpenSearchWriterRetainsLegacyDatePartition(t *testing.T) {
	w := &OpenSearchWriter{writeTarget: "traffic-alerts"}
	if got := w.targetFor(time.Date(2026, 8, 4, 1, 2, 3, 0, time.UTC)); got != "traffic-alerts-2026-08-04" {
		t.Fatalf("targetFor() = %q", got)
	}
}
