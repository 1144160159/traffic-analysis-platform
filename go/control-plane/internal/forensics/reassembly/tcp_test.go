package reassembly

import (
	"bytes"
	"testing"
)

func TestReassembleOrdersAndDeduplicatesIdenticalRetransmission(t *testing.T) {
	result, err := Reassemble([]Segment{
		{Sequence: 106, Payload: []byte("world"), PacketIndex: 3},
		{Sequence: 100, Payload: []byte("hello "), PacketIndex: 1},
		{Sequence: 100, Payload: []byte("hello "), PacketIndex: 2},
	}, 1024)
	if err != nil || result.Status != StatusComplete || !bytes.Equal(result.Bytes, []byte("hello world")) {
		t.Fatalf("unexpected result: %#v err=%v", result, err)
	}
	if len(result.PacketIndexes) != 3 || len(result.SourceRanges) != 3 {
		t.Fatalf("source proof lost: %#v", result)
	}
}

func TestReassembleUnequalOverlapIsCorrupt(t *testing.T) {
	result, err := Reassemble([]Segment{
		{Sequence: 10, Payload: []byte("abcdef")},
		{Sequence: 13, Payload: []byte("XYZ")},
	}, 1024)
	if err != nil || result.Status != StatusCorrupt || result.ConflictAt == nil || *result.ConflictAt != 13 {
		t.Fatalf("expected corrupt overlap: %#v err=%v", result, err)
	}
}

func TestReassembleGapNeverInventsBytes(t *testing.T) {
	result, err := Reassemble([]Segment{
		{Sequence: 10, Payload: []byte("abc")},
		{Sequence: 15, Payload: []byte("fg")},
	}, 1024)
	if err != nil || result.Status != StatusPartial || string(result.Bytes) != "abc" {
		t.Fatalf("expected coherent prefix only: %#v err=%v", result, err)
	}
	if len(result.MissingRanges) != 1 || result.MissingRanges[0] != (SequenceRange{Start: 13, End: 15}) {
		t.Fatalf("unexpected missing ranges: %#v", result.MissingRanges)
	}
}

func TestReassembleTruncationAndOversizePrecedence(t *testing.T) {
	truncated, err := Reassemble([]Segment{{
		Sequence: 20, Payload: []byte("visible"), CapturedLength: 7, OriginalLength: 20,
	}}, 1024)
	if err != nil || truncated.Status != StatusTruncated || truncated.TruncationAt == nil {
		t.Fatalf("expected truncation: %#v err=%v", truncated, err)
	}
	oversize, err := Reassemble([]Segment{{Sequence: 20, Payload: []byte("12345678")}}, 4)
	if err != nil || oversize.Status != StatusOversize || len(oversize.Bytes) != 0 {
		t.Fatalf("expected metadata-only oversize: %#v err=%v", oversize, err)
	}
}

func TestReassembleSequenceWraparound(t *testing.T) {
	result, err := Reassemble([]Segment{
		{Sequence: 1, Payload: []byte("cd")},
		{Sequence: ^uint32(0) - 1, Payload: []byte("ab")},
	}, 1024)
	if err != nil || result.Status != StatusPartial {
		t.Fatalf("unexpected wrap result: %#v err=%v", result, err)
	}
	// Sequence fffffffe,ffffffff then a single missing sequence 0, then 1,2.
	if string(result.Bytes) != "ab" || len(result.MissingRanges) != 1 {
		t.Fatalf("wraparound gap was not preserved: %#v", result)
	}
}
