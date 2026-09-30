package auction

import (
	"errors"
	"fmt"
)

type Status string

const (
	StatusNotActive Status = "NOT_ACTIVE"
	StatusActive    Status = "ACTIVE"
	StatusCompleted Status = "COMPLETED"
	StatusCancelled Status = "CANCELLED"
)

var (
	ErrInvalidTransition = errors.New("invalid auction status transition")
	ErrUnknownStatus     = errors.New("unknown auction status")
)

// transitions lists every allowed move. Anything not listed is rejected.
//
//	NOT_ACTIVE ──► ACTIVE ──► COMPLETED
//	    │
//	    └────────► CANCELLED
var transitions = map[Status][]Status{
	StatusNotActive: {StatusActive, StatusCancelled},
	StatusActive:    {StatusCompleted},
	StatusCompleted: {},
	StatusCancelled: {},
}

// ParseStatus converts user input such as a query parameter into a Status.
func ParseStatus(s string) (Status, error) {
	status := Status(s)
	if !status.Valid() {
		return "", fmt.Errorf("%w: %q", ErrUnknownStatus, s)
	}
	return status, nil
}

func (s Status) Valid() bool {
	_, ok := transitions[s]
	return ok
}

// Terminal reports whether no further transitions are possible.
func (s Status) Terminal() bool {
	return s.Valid() && len(transitions[s]) == 0
}

// CanTransition reports whether an auction may move from s to next.
func (s Status) CanTransition(next Status) bool {
	for _, allowed := range transitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// Transition returns next if the move is allowed, or an error wrapping
// ErrInvalidTransition that names both states.
func (s Status) Transition(next Status) (Status, error) {
	if !s.Valid() {
		return s, fmt.Errorf("%w: %q", ErrUnknownStatus, s)
	}
	if !next.Valid() {
		return s, fmt.Errorf("%w: %q", ErrUnknownStatus, next)
	}
	if !s.CanTransition(next) {
		return s, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, s, next)
	}
	return next, nil
}
