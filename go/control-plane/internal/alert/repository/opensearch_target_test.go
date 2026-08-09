package repository

import (
	"testing"
	"time"
)

func TestOpenSearchRepositoryUsesExactWriteAliasWhenV2Enabled(t *testing.T) {
	r := &OpenSearchRepository{writeTarget: "alerts-v2-write", exactTarget: true}
	if got := r.targetFor(time.Date(2026, 8, 4, 1, 2, 3, 0, time.UTC)); got != "alerts-v2-write" {
		t.Fatalf("targetFor() = %q", got)
	}
}

func TestOpenSearchRepositoryRetainsLegacyDatePartition(t *testing.T) {
	r := &OpenSearchRepository{writeTarget: "traffic-alerts"}
	if got := r.targetFor(time.Date(2026, 8, 4, 1, 2, 3, 0, time.UTC)); got != "traffic-alerts-2026-08-04" {
		t.Fatalf("targetFor() = %q", got)
	}
}
