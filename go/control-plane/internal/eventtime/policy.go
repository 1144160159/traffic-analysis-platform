// Package eventtime implements the process-independent event-time contract used
// by control-plane projectors. All values are Unix epoch milliseconds.
package eventtime

import "fmt"

const UninitializedWatermark int64 = -1 << 63

type Status string

const (
	Accept                Status = "ACCEPT"
	InvalidEventTime      Status = "INVALID_EVENT_TIME"
	InvalidIngestTime     Status = "INVALID_INGEST_TIME"
	InvalidProcessingTime Status = "INVALID_PROCESSING_TIME"
	FutureEvent           Status = "FUTURE_EVENT"
	ClockRollback         Status = "CLOCK_ROLLBACK"
	LateEvent             Status = "LATE_EVENT"
)

type Policy struct {
	AllowedLatenessMS  int64
	MaxFutureSkewMS    int64
	MaxClockRollbackMS int64
}

type Input struct {
	EventTimeMS            int64
	IngestTimeMS           int64
	ProcessingTimeMS       int64
	WatermarkMS            int64
	PreviousMaxEventTimeMS *int64
}

type Decision struct {
	Status           Status
	EventTimeMS      int64
	IngestTimeMS     int64
	ProcessingTimeMS int64
	WatermarkMS      int64
	AsOfMS           int64
}

func NewPolicy(allowedLatenessMS, maxFutureSkewMS, maxClockRollbackMS int64) (Policy, error) {
	if allowedLatenessMS < 0 || maxFutureSkewMS < 0 || maxClockRollbackMS < 0 {
		return Policy{}, fmt.Errorf("event-time budgets must not be negative")
	}
	return Policy{
		AllowedLatenessMS:  allowedLatenessMS,
		MaxFutureSkewMS:    maxFutureSkewMS,
		MaxClockRollbackMS: maxClockRollbackMS,
	}, nil
}

func (p Policy) Classify(input Input) Decision {
	status := Accept
	switch {
	case input.EventTimeMS <= 0:
		status = InvalidEventTime
	case input.IngestTimeMS <= 0:
		status = InvalidIngestTime
	case input.ProcessingTimeMS <= 0:
		status = InvalidProcessingTime
	default:
		// 预算参数由 NewPolicy 保证非负,此处错误只可能来自调用方绕过构造器;
		// 按 fail-closed 处理为对应状态而非 panic。
		if future, err := IsFuture(input.EventTimeMS, input.IngestTimeMS, p.MaxFutureSkewMS); err != nil {
			status = InvalidEventTime
		} else if future {
			status = FutureEvent
		} else if input.PreviousMaxEventTimeMS != nil {
			if rollback, err := IsClockRollback(input.EventTimeMS, *input.PreviousMaxEventTimeMS, p.MaxClockRollbackMS); err != nil {
				status = InvalidEventTime
			} else if rollback {
				status = ClockRollback
			} else if late, err := IsLate(input.EventTimeMS, input.WatermarkMS, p.AllowedLatenessMS); err != nil {
				status = InvalidEventTime
			} else if late {
				status = LateEvent
			}
		} else if late, err := IsLate(input.EventTimeMS, input.WatermarkMS, p.AllowedLatenessMS); err != nil {
			status = InvalidEventTime
		} else if late {
			status = LateEvent
		}
	}
	asOf := int64(0)
	if input.ProcessingTimeMS > 0 {
		asOf, _ = EffectiveAsOf(input.WatermarkMS, input.ProcessingTimeMS)
	}
	return Decision{
		Status: status, EventTimeMS: input.EventTimeMS, IngestTimeMS: input.IngestTimeMS,
		ProcessingTimeMS: input.ProcessingTimeMS, WatermarkMS: input.WatermarkMS, AsOfMS: asOf,
	}
}

func IsFuture(eventTimeMS, ingestTimeMS, maxFutureSkewMS int64) (bool, error) {
	if maxFutureSkewMS < 0 {
		return false, fmt.Errorf("maxFutureSkewMS must not be negative (got %d)", maxFutureSkewMS)
	}
	return eventTimeMS > saturatingAdd(ingestTimeMS, maxFutureSkewMS), nil
}

func IsClockRollback(eventTimeMS, previousMaximumMS, toleranceMS int64) (bool, error) {
	if toleranceMS < 0 {
		return false, fmt.Errorf("toleranceMS must not be negative (got %d)", toleranceMS)
	}
	return eventTimeMS < saturatingSubtract(previousMaximumMS, toleranceMS), nil
}

// IsLate uses the strict shared boundary. An event exactly on the cutoff is accepted.
func IsLate(eventTimeMS, watermarkMS, allowedLatenessMS int64) (bool, error) {
	if allowedLatenessMS < 0 {
		return false, fmt.Errorf("allowedLatenessMS must not be negative (got %d)", allowedLatenessMS)
	}
	return watermarkMS != UninitializedWatermark &&
		eventTimeMS < saturatingSubtract(watermarkMS, allowedLatenessMS), nil
}

func EffectiveAsOf(watermarkMS, processingTimeMS int64) (int64, error) {
	if processingTimeMS <= 0 {
		return 0, fmt.Errorf("processingTimeMS must be positive (got %d)", processingTimeMS)
	}
	if watermarkMS == UninitializedWatermark || watermarkMS > processingTimeMS {
		return processingTimeMS, nil
	}
	return watermarkMS, nil
}

func ObservedWithinAsOf(observedAtMS, asOfMS int64) bool {
	return observedAtMS > 0 && asOfMS > 0 && observedAtMS <= asOfMS
}

func saturatingAdd(left, right int64) int64 {
	if right > 0 && left > int64(^uint64(0)>>1)-right {
		return int64(^uint64(0) >> 1)
	}
	return left + right
}

func saturatingSubtract(left, right int64) int64 {
	const minInt64 = -1 << 63
	if right > 0 && left < minInt64+right {
		return minInt64
	}
	return left - right
}
