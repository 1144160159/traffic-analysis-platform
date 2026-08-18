package trafficv1

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestPcapIndexMetaManifestWireContractIsAdditive(t *testing.T) {
	descriptor := (&PcapIndexMeta{}).ProtoReflect().Descriptor()
	wantFields := []struct {
		name   protoreflect.Name
		number protoreflect.FieldNumber
		kind   protoreflect.Kind
		list   bool
	}{
		{"tenant_id", 1, protoreflect.StringKind, false},
		{"probe_id", 2, protoreflect.StringKind, false},
		{"file_key", 3, protoreflect.StringKind, false},
		{"ts_start", 4, protoreflect.Int64Kind, false},
		{"ts_end", 5, protoreflect.Int64Kind, false},
		{"byte_size", 6, protoreflect.Uint64Kind, false},
		{"zstd_level", 7, protoreflect.Uint32Kind, false},
		{"sha256", 8, protoreflect.StringKind, false},
		{"community_id", 9, protoreflect.StringKind, false},
		{"flow_id", 10, protoreflect.StringKind, false},
		{"offset_start", 11, protoreflect.Uint64Kind, false},
		{"offset_end", 12, protoreflect.Uint64Kind, false},
		{"bloom_filter_b64", 13, protoreflect.StringKind, false},
		{"community_ids", 14, protoreflect.StringKind, true},
		{"created_ts", 15, protoreflect.Int64Kind, false},
		{"bucket", 16, protoreflect.StringKind, false},
		{"object_version", 17, protoreflect.StringKind, false},
		{"etag", 18, protoreflect.StringKind, false},
		{"original_size", 19, protoreflect.Uint64Kind, false},
		{"stored_size", 20, protoreflect.Uint64Kind, false},
		{"compression", 21, protoreflect.StringKind, false},
		{"manifest_version", 22, protoreflect.Uint32Kind, false},
		// 加法式契约补全:归档窗口包数(旧生产者缺省 0,wire 兼容)。
		{"packet_count", 23, protoreflect.Uint64Kind, false},
	}
	if descriptor.Fields().Len() != len(wantFields) {
		t.Fatalf("field count=%d want=%d", descriptor.Fields().Len(), len(wantFields))
	}
	for _, want := range wantFields {
		field := descriptor.Fields().ByName(want.name)
		if field == nil || field.Number() != want.number || field.Kind() != want.kind || field.IsList() != want.list {
			t.Fatalf("field %s descriptor=%v want number=%d kind=%s list=%v", want.name, field, want.number, want.kind, want.list)
		}
	}
	for _, forbidden := range []protoreflect.Name{"kafka_topic", "kafka_partition", "kafka_offset", "raw_sha256"} {
		if descriptor.Fields().ByName(forbidden) != nil {
			t.Fatalf("transport authority field %s must not enter PcapIndexMeta", forbidden)
		}
	}
}
