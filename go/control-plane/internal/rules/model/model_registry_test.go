package model

import "testing"

func TestDeprecatedVersionCannotUseOrdinaryActivationTransition(t *testing.T) {
	if CanTransitionModelStatus(ModelStatusDeprecated, ModelStatusActive) {
		t.Fatal("deprecated -> active must remain rollback-only, not a generic state transition")
	}
	if !CanTransitionModelStatus(ModelStatusDeprecated, ModelStatusArchived) {
		t.Fatal("deprecated -> archived must remain allowed")
	}
}
