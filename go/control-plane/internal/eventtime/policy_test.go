package eventtime

import (
	"math"
	"testing"
)

func TestStrictLateBoundaryAndOverflow(t *testing.T) {
	late, err := IsLate(900, UninitializedWatermark, 100)
	if err != nil {
		t.Fatal(err)
	}
	if late {
		t.Fatal("uninitialized watermark must not classify an event as late")
	}
	late, err = IsLate(900, 1000, 100)
	if err != nil {
		t.Fatal(err)
	}
	if late {
		t.Fatal("event exactly on the cutoff must be accepted")
	}
	late, err = IsLate(899, 1000, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !late {
		t.Fatal("event before the cutoff must be late")
	}
	future, err := IsFuture(math.MaxInt64, math.MaxInt64, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	if future {
		t.Fatal("future comparison overflowed")
	}
}

func TestNegativeBudgetsReturnErrors(t *testing.T) {
	if _, err := IsFuture(1, 1, -1); err == nil {
		t.Fatal("IsFuture must reject negative skew")
	}
	if _, err := IsClockRollback(1, 1, -1); err == nil {
		t.Fatal("IsClockRollback must reject negative tolerance")
	}
	if _, err := IsLate(1, 1, -1); err == nil {
		t.Fatal("IsLate must reject negative lateness")
	}
	if _, err := EffectiveAsOf(0, 0); err == nil {
		t.Fatal("EffectiveAsOf must reject non-positive processing time")
	}
}

func TestClassificationOrderMatchesSharedContract(t *testing.T) {
	policy, err := NewPolicy(100, 500, 50)
	if err != nil {
		t.Fatal(err)
	}
	previous := int64(1000)
	tests := []struct {
		name string
		in   Input
		want Status
	}{
		{"future", Input{1501, 1000, 2000, UninitializedWatermark, nil}, FutureEvent},
		{"rollback before late", Input{949, 1500, 2000, 1200, &previous}, ClockRollback},
		{"late", Input{1099, 1500, 2000, 1200, nil}, LateEvent},
		{"boundary", Input{1100, 1500, 2000, 1200, nil}, Accept},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := policy.Classify(test.in).Status; got != test.want {
				t.Fatalf("status=%s want=%s", got, test.want)
			}
		})
	}
}

func TestEffectiveAsOfAndObservationBoundary(t *testing.T) {
	asOf, err := EffectiveAsOf(UninitializedWatermark, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if asOf != 2000 {
		t.Fatalf("as_of=%d", asOf)
	}
	asOf, err = EffectiveAsOf(1900, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if asOf != 1900 {
		t.Fatalf("as_of=%d", asOf)
	}
	if !ObservedWithinAsOf(1900, 1900) || ObservedWithinAsOf(1901, 1900) {
		t.Fatal("observation as_of boundary is inconsistent")
	}
}
