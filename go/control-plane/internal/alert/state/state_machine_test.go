package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanTransition_Valid(t *testing.T) {
	tests := []struct {
		name string
		from AlertStatus
		to   AlertStatus
		want bool
	}{
		{"new_to_triage", StatusNew, StatusTriage, true},
		{"new_to_closed", StatusNew, StatusClosed, true},
		{"new_to_assigned", StatusNew, StatusAssigned, true},
		{"triage_to_assigned", StatusTriage, StatusAssigned, true},
		{"triage_to_closed", StatusTriage, StatusClosed, true},
		{"triage_to_new", StatusTriage, StatusNew, false},
		{"assigned_to_triage", StatusAssigned, StatusTriage, true},
		{"assigned_to_closed", StatusAssigned, StatusClosed, true},
		{"assigned_to_new", StatusAssigned, StatusNew, false},
		{"closed_to_new", StatusClosed, StatusNew, true}, // reopen
		{"closed_to_triage", StatusClosed, StatusTriage, false},
		{"unknown_from", AlertStatus("unknown"), StatusNew, false},
		{"unknown_to", StatusNew, AlertStatus("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanTransition(tt.from, tt.to)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    AlertStatus
		to      AlertStatus
		wantErr bool
	}{
		{"valid", StatusNew, StatusTriage, false},
		{"invalid", StatusTriage, StatusNew, true},
		{"reopen", StatusClosed, StatusNew, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Transition(tt.from, tt.to)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    AlertStatus
		wantErr bool
	}{
		{"new", "new", StatusNew, false},
		{"open_alias", "open", StatusNew, false},
		{"triage", "triage", StatusTriage, false},
		{"in_progress_alias", "in_progress", StatusTriage, false},
		{"assigned", "assigned", StatusAssigned, false},
		{"assigned_display_alias", "已指派", StatusAssigned, false},
		{"closed", "closed", StatusClosed, false},
		{"resolved_alias", "resolved", StatusClosed, false},
		{"false_positive_alias", "false_positive", StatusClosed, false},
		{"proto_alias", "ALERT_STATUS_CLOSED", StatusClosed, false},
		{"unknown", "unknown", "", true},
		{"empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStatus(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestAlertStatus_String(t *testing.T) {
	assert.Equal(t, "new", StatusNew.String())
	assert.Equal(t, "triage", StatusTriage.String())
	assert.Equal(t, "assigned", StatusAssigned.String())
	assert.Equal(t, "closed", StatusClosed.String())
}
