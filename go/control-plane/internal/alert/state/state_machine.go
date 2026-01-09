package state

import (
	"fmt"
)

type AlertStatus string

const (
	StatusNew      AlertStatus = "new"
	StatusTriage   AlertStatus = "triage"
	StatusAssigned AlertStatus = "assigned"
	StatusClosed   AlertStatus = "closed"
)

var ValidTransitions = map[AlertStatus][]AlertStatus{
	StatusNew:      {StatusTriage, StatusClosed},
	StatusTriage:   {StatusAssigned, StatusClosed},
	StatusAssigned: {StatusTriage, StatusClosed},
	StatusClosed:   {StatusNew}, // Reopen
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
	switch s {
	case "new":
		return StatusNew, nil
	case "triage":
		return StatusTriage, nil
	case "assigned":
		return StatusAssigned, nil
	case "closed":
		return StatusClosed, nil
	default:
		return "", fmt.Errorf("unknown status: %s", s)
	}
}

func (s AlertStatus) String() string {
	return string(s)
}
