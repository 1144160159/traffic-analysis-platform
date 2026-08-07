package persistence

import (
	"testing"
	"time"
)

func TestResolveAlertWriteSchemaPrefersCanonicalInt64Milliseconds(t *testing.T) {
	schema, err := resolveAlertWriteSchema(map[string]string{
		"first_seen": "Int64", "last_seen": "Int64", "updated_at": "Int64",
		"updated_ts": "DateTime64(3)",
	})
	if err != nil {
		t.Fatal(err)
	}
	if schema.updatedColumn != "updated_at" || schema.timestampMode != alertTimestampInt64Millis {
		t.Fatalf("schema=%+v", schema)
	}
	value := time.Date(2026, 8, 7, 12, 0, 0, 123000000, time.UTC)
	if got := schema.timestamp(value); got != value.UnixMilli() {
		t.Fatalf("timestamp=%v", got)
	}
}

func TestResolveAlertWriteSchemaKeepsLegacyDateTime64Compatibility(t *testing.T) {
	schema, err := resolveAlertWriteSchema(map[string]string{
		"first_seen": "DateTime64(3)", "last_seen": "DateTime64(3)", "updated_ts": "DateTime64(3)",
	})
	if err != nil {
		t.Fatal(err)
	}
	if schema.updatedColumn != "updated_ts" || schema.timestampMode != alertTimestampDateTime64 {
		t.Fatalf("schema=%+v", schema)
	}
}

func TestResolveAlertWriteSchemaRejectsMixedOrUnknownTimestampAuthority(t *testing.T) {
	for _, columns := range []map[string]string{
		{"first_seen": "Int64", "last_seen": "Int64"},
		{"first_seen": "DateTime64(3)", "last_seen": "DateTime64(3)", "updated_at": "Int64"},
		{"last_seen": "Int64", "updated_at": "Int64"},
	} {
		if schema, err := resolveAlertWriteSchema(columns); err == nil {
			t.Fatalf("schema %+v should be rejected: %+v", columns, schema)
		}
	}
}
