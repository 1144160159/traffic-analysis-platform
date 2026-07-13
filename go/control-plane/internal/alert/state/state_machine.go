package state

import (
	"fmt"
	"strings"
)

type AlertStatus string

const (
	StatusNew      AlertStatus = "new"
	StatusTriage   AlertStatus = "triage"
	StatusAssigned AlertStatus = "assigned"
	StatusClosed   AlertStatus = "closed"
)

var ValidTransitions = map[AlertStatus][]AlertStatus{
	StatusNew:      {StatusTriage, StatusAssigned, StatusClosed},
	StatusTriage:   {StatusAssigned, StatusClosed},
	StatusAssigned: {StatusTriage, StatusClosed},
	StatusClosed:   {StatusNew}, // Reopen
}

var statusAliases = map[string]AlertStatus{
	"new":              StatusNew,
	"open":             StatusNew,
	"unhandled":        StatusNew,
	"alert_status_new": StatusNew,
	"未处理":              StatusNew,

	"triage":                 StatusTriage,
	"investigating":          StatusTriage,
	"investigation":          StatusTriage,
	"review":                 StatusTriage,
	"reviewing":              StatusTriage,
	"in_progress":            StatusTriage,
	"processing":             StatusTriage,
	"alert_status_triage":    StatusTriage,
	"alert_status_reviewing": StatusTriage,
	"研判中":                    StatusTriage,

	"assigned":              StatusAssigned,
	"delegated":             StatusAssigned,
	"alert_status_assigned": StatusAssigned,
	"已指派":                   StatusAssigned,

	"closed":                StatusClosed,
	"resolved":              StatusClosed,
	"confirmed":             StatusClosed,
	"ignored":               StatusClosed,
	"false_positive":        StatusClosed,
	"alert_status_closed":   StatusClosed,
	"alert_status_resolved": StatusClosed,
	"已关闭":                   StatusClosed,
}

func CanTransition(from, to AlertStatus) bool {
	validStates, ok := ValidTransitions[from]
	if !ok {
		return false
	}

	for _, state := range validStates {
		if state == to {
			return true
		}
	}

	return false
}

func Transition(from, to AlertStatus) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("invalid transition from %s to %s", from, to)
	}
	return nil
}

func ParseStatus(s string) (AlertStatus, error) {
	normalized := strings.ToLower(strings.TrimSpace(s))
	if normalized == "" {
		return "", fmt.Errorf("unknown status: %s", s)
	}
	status, ok := statusAliases[normalized]
	if !ok {
		return "", fmt.Errorf("unknown status: %s", s)
	}
	return status, nil
}

func (s AlertStatus) String() string {
	return string(s)
}
