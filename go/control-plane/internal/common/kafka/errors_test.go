package kafka

import (
	"errors"
	"fmt"
	"testing"
)

func TestPermanentErrorClassificationSurvivesWrapping(t *testing.T) {
	original := errors.New("invalid payload")
	marked := Permanent(original)
	if !IsPermanent(marked) || !IsPermanent(fmt.Errorf("decode: %w", marked)) {
		t.Fatal("permanent classification must survive wrapping")
	}
	if !errors.Is(marked, original) {
		t.Fatal("permanent error must preserve the original cause")
	}
	if Permanent(marked) != marked {
		t.Fatal("marking an already permanent error must be idempotent")
	}
	if Permanent(nil) != nil || IsPermanent(errors.New("database unavailable")) {
		t.Fatal("nil or unmarked transient errors must not be permanent")
	}
}

func TestDLQPermanentOnlyIsExplicitAndBackwardCompatible(t *testing.T) {
	transient := errors.New("postgres unavailable")
	legacy := &Consumer{}
	if !legacy.dlqEligible(transient) {
		t.Fatal("existing consumers must retain legacy DLQ eligibility until migrated")
	}
	strict := &Consumer{config: ConsumerConfig{DLQPermanentOnly: true}}
	if strict.dlqEligible(transient) {
		t.Fatal("strict consumer must not quarantine a transient failure")
	}
	if !strict.dlqEligible(Permanent(errors.New("invalid payload"))) {
		t.Fatal("strict consumer must quarantine explicitly permanent payload errors")
	}
}
