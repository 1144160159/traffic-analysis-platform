package trafficv1

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestProbeGroupReadinessReceiptV1WireContract(t *testing.T) {
	descriptor := (&ProbeGroupReadinessReceiptV1{}).ProtoReflect().Descriptor()
	wantFields := []struct {
		name   protoreflect.Name
		number protoreflect.FieldNumber
		kind   protoreflect.Kind
	}{
		{"receipt_id", 1, protoreflect.StringKind},
		{"consumer_group", 2, protoreflect.StringKind},
		{"observed_topic", 3, protoreflect.StringKind},
		{"member_id", 4, protoreflect.StringKind},
		{"generation_id", 5, protoreflect.Int32Kind},
		{"owner_epoch", 6, protoreflect.Int64Kind},
		{"state", 7, protoreflect.EnumKind},
		{"observed_at_ms", 8, protoreflect.Int64Kind},
		{"expires_at_ms", 9, protoreflect.Int64Kind},
		{"publisher_instance_id", 10, protoreflect.StringKind},
	}
	if descriptor.Fields().Len() != len(wantFields) {
		t.Fatalf("field count=%d want=%d", descriptor.Fields().Len(), len(wantFields))
	}
	for _, want := range wantFields {
		field := descriptor.Fields().ByName(want.name)
		if field == nil || field.Number() != want.number || field.Kind() != want.kind {
			t.Fatalf("field %s descriptor=%v want number=%d kind=%s", want.name, field, want.number, want.kind)
		}
	}

	wantStates := map[protoreflect.Name]protoreflect.EnumNumber{
		"PROBE_GROUP_READINESS_STATE_V1_UNSPECIFIED": 0,
		"PROBE_GROUP_READINESS_STATE_V1_ASSIGNED":    1,
		"PROBE_GROUP_READINESS_STATE_V1_READY":       2,
		"PROBE_GROUP_READINESS_STATE_V1_REVOKED":     3,
		"PROBE_GROUP_READINESS_STATE_V1_STOPPED":     4,
	}
	states := ProbeGroupReadinessStateV1(0).Descriptor().Values()
	if states.Len() != len(wantStates) {
		t.Fatalf("state count=%d want=%d", states.Len(), len(wantStates))
	}
	for name, number := range wantStates {
		value := states.ByName(name)
		if value == nil || value.Number() != number {
			t.Fatalf("state %s=%v want=%d", name, value, number)
		}
	}
}

func TestProbeGroupReadinessReceiptV1RoundTripAndUnknownFieldCompatibility(t *testing.T) {
	want := &ProbeGroupReadinessReceiptV1{
		ReceiptId: "receipt-1", ConsumerGroup: "probe-control-v2",
		ObservedTopic: "probe.operation.commands.v1", MemberId: "member-1",
		GenerationId: 19, OwnerEpoch: 42,
		State:        ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY,
		ObservedAtMs: 1_725_000_000_000, ExpiresAtMs: 1_725_000_030_000,
		PublisherInstanceId: "publisher-1",
	}
	wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	// Field 127, varint 1. Older consumers must retain an unknown additive
	// field so a decode/re-encode relay does not destroy future metadata.
	unknown := []byte{0xf8, 0x07, 0x01}
	wire = append(wire, unknown...)

	got := &ProbeGroupReadinessReceiptV1{}
	if err := (proto.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(wire, got); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(want, got) {
		gotWithoutUnknown := proto.Clone(got).(*ProbeGroupReadinessReceiptV1)
		gotWithoutUnknown.ProtoReflect().SetUnknown(nil)
		if !proto.Equal(want, gotWithoutUnknown) {
			t.Fatalf("round trip mismatch: got=%v want=%v", gotWithoutUnknown, want)
		}
	}
	if gotUnknown := got.ProtoReflect().GetUnknown(); !bytes.Equal(gotUnknown, unknown) {
		t.Fatalf("unknown wire=%x want=%x", gotUnknown, unknown)
	}
	reencoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	redecoded := &ProbeGroupReadinessReceiptV1{}
	if err := proto.Unmarshal(reencoded, redecoded); err != nil {
		t.Fatal(err)
	}
	if gotUnknown := redecoded.ProtoReflect().GetUnknown(); !bytes.Equal(gotUnknown, unknown) {
		t.Fatalf("unknown field lost after relay: %x", gotUnknown)
	}
}
