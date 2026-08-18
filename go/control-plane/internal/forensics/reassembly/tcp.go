package reassembly

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Status is deliberately smaller than the restoration terminal status set.
// Protocol support and application framing are decided by extractor packages.
type Status string

const (
	StatusComplete  Status = "complete"
	StatusPartial   Status = "partial"
	StatusTruncated Status = "truncated"
	StatusCorrupt   Status = "corrupt"
	StatusOversize  Status = "oversize"
)

var ErrInvalidLimit = errors.New("max stream bytes must be positive")

// Segment contains TCP payload only. Sequence is the sequence number of the
// first payload octet (the caller accounts for SYN consuming one sequence).
type Segment struct {
	Sequence         uint32
	Payload          []byte
	CapturedLength   int
	OriginalLength   int
	PacketIndex      uint64
	CapturedAt       time.Time
	ObjectBucket     string
	ObjectKey        string
	ObjectVersion    string
	ObjectRangeStart uint64
	ObjectRangeEnd   uint64
	ObjectRangeExact bool
}

type SequenceRange struct {
	Start uint32 `json:"start"`
	End   uint32 `json:"end"` // exclusive
}

type SourceRange struct {
	SequenceStart    uint32 `json:"sequence_start"`
	SequenceEnd      uint32 `json:"sequence_end"`
	PacketIndex      uint64 `json:"packet_index"`
	ObjectBucket     string `json:"object_bucket"`
	ObjectKey        string `json:"object_key"`
	ObjectVersion    string `json:"object_version"`
	ObjectRangeStart uint64 `json:"object_range_start"`
	ObjectRangeEnd   uint64 `json:"object_range_end"`
	ObjectRangeExact bool   `json:"object_range_exact"`
	Length           int    `json:"length"`
}

type Result struct {
	Status          Status
	Bytes           []byte
	BaseSequence    uint32
	EndSequence     uint32
	MissingRanges   []SequenceRange
	SourceRanges    []SourceRange
	PacketIndexes   []uint64
	TruncationAt    *uint32
	ConflictAt      *uint32
	ObservedSpan    uint64
	CapturedAtStart time.Time
	CapturedAtEnd   time.Time
}

type placedSegment struct {
	segment Segment
	offset  uint64
}

// sequenceBefore is valid for one bounded TCP stream whose span is below
// 2^31. The size guard below rejects a stream that could make ordering
// ambiguous.
func sequenceBefore(left, right uint32) bool {
	return int32(left-right) < 0
}

func statusWithPrecedence(conflict, oversize, truncated bool, missing int) Status {
	switch {
	case oversize:
		return StatusOversize
	case conflict:
		return StatusCorrupt
	case truncated:
		return StatusTruncated
	case missing > 0:
		return StatusPartial
	default:
		return StatusComplete
	}
}

// Reassemble deterministically deduplicates identical retransmissions,
// rejects unequal overlaps, records exact gaps, and never synthesizes bytes.
// For a gapped stream Bytes is the coherent prefix before the first gap;
// later visible bytes remain represented by SourceRanges and MissingRanges.
func Reassemble(input []Segment, maxStreamBytes uint64) (Result, error) {
	if maxStreamBytes == 0 {
		return Result{}, ErrInvalidLimit
	}
	segments := make([]Segment, 0, len(input))
	for _, segment := range input {
		if len(segment.Payload) == 0 {
			continue
		}
		segment.Payload = bytes.Clone(segment.Payload)
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return Result{Status: StatusComplete}, nil
	}
	sort.SliceStable(segments, func(i, j int) bool {
		if segments[i].Sequence == segments[j].Sequence {
			return segments[i].PacketIndex < segments[j].PacketIndex
		}
		return sequenceBefore(segments[i].Sequence, segments[j].Sequence)
	})
	base := segments[0].Sequence
	placed := make([]placedSegment, 0, len(segments))
	var span uint64
	truncated := false
	var truncationAt *uint32
	packetSet := make(map[uint64]struct{})
	startAt, endAt := segments[0].CapturedAt, segments[0].CapturedAt
	for _, segment := range segments {
		offset := uint64(uint32(segment.Sequence - base))
		end := offset + uint64(len(segment.Payload))
		if offset >= 1<<31 || end >= 1<<31 {
			return Result{Status: StatusOversize, BaseSequence: base, ObservedSpan: end}, nil
		}
		if end > span {
			span = end
		}
		placed = append(placed, placedSegment{segment: segment, offset: offset})
		packetSet[segment.PacketIndex] = struct{}{}
		if !segment.CapturedAt.IsZero() && (startAt.IsZero() || segment.CapturedAt.Before(startAt)) {
			startAt = segment.CapturedAt
		}
		if segment.CapturedAt.After(endAt) {
			endAt = segment.CapturedAt
		}
		if segment.OriginalLength > 0 && segment.CapturedLength < segment.OriginalLength {
			truncated = true
			position := segment.Sequence + uint32(len(segment.Payload))
			if truncationAt == nil || sequenceBefore(position, *truncationAt) {
				copyPosition := position
				truncationAt = &copyPosition
			}
		}
	}
	result := Result{
		BaseSequence: base, EndSequence: base + uint32(span), ObservedSpan: span,
		TruncationAt: truncationAt, CapturedAtStart: startAt, CapturedAtEnd: endAt,
	}
	if span > maxStreamBytes {
		result.Status = StatusOversize
		return result, nil
	}
	data := make([]byte, int(span))
	present := make([]bool, int(span))
	for _, placedSegment := range placed {
		segment := placedSegment.segment
		for index, value := range segment.Payload {
			position := int(placedSegment.offset) + index
			if present[position] && data[position] != value {
				conflict := base + uint32(position)
				result.ConflictAt = &conflict
				result.Status = StatusCorrupt
				return result, nil
			}
			data[position] = value
			present[position] = true
		}
		result.SourceRanges = append(result.SourceRanges, SourceRange{
			SequenceStart:    segment.Sequence,
			SequenceEnd:      segment.Sequence + uint32(len(segment.Payload)),
			PacketIndex:      segment.PacketIndex,
			ObjectBucket:     segment.ObjectBucket,
			ObjectKey:        segment.ObjectKey,
			ObjectVersion:    segment.ObjectVersion,
			ObjectRangeStart: segment.ObjectRangeStart,
			ObjectRangeEnd:   segment.ObjectRangeEnd,
			ObjectRangeExact: segment.ObjectRangeExact,
			Length:           len(segment.Payload),
		})
	}
	firstGap := len(data)
	for offset := 0; offset < len(present); {
		if present[offset] {
			offset++
			continue
		}
		start := offset
		for offset < len(present) && !present[offset] {
			offset++
		}
		if start < firstGap {
			firstGap = start
		}
		result.MissingRanges = append(result.MissingRanges, SequenceRange{
			Start: base + uint32(start), End: base + uint32(offset),
		})
	}
	result.Bytes = bytes.Clone(data[:firstGap])
	result.PacketIndexes = make([]uint64, 0, len(packetSet))
	for packet := range packetSet {
		result.PacketIndexes = append(result.PacketIndexes, packet)
	}
	sort.Slice(result.PacketIndexes, func(i, j int) bool { return result.PacketIndexes[i] < result.PacketIndexes[j] })
	sort.Slice(result.SourceRanges, func(i, j int) bool {
		if result.SourceRanges[i].SequenceStart == result.SourceRanges[j].SequenceStart {
			return result.SourceRanges[i].PacketIndex < result.SourceRanges[j].PacketIndex
		}
		return sequenceBefore(result.SourceRanges[i].SequenceStart, result.SourceRanges[j].SequenceStart)
	})
	result.Status = statusWithPrecedence(false, false, truncated, len(result.MissingRanges))
	return result, nil
}

func (result Result) Validate() error {
	if result.Status == StatusComplete && len(result.MissingRanges) > 0 {
		return errors.New("complete stream contains missing ranges")
	}
	if result.Status == StatusCorrupt && result.ConflictAt == nil {
		return errors.New("corrupt stream lacks conflict position")
	}
	for index, missing := range result.MissingRanges {
		if missing.Start == missing.End {
			return fmt.Errorf("missing range %d is empty", index)
		}
		if index > 0 && !sequenceBefore(result.MissingRanges[index-1].End, missing.Start) {
			return errors.New("missing ranges overlap or are unsorted")
		}
	}
	return nil
}
