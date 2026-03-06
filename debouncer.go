package main

import (
	"time"
)

// StateDebouncer handles the logic of debouncing state changes based on delays.
type StateDebouncer struct {
	onDelay      time.Duration
	offDelay     time.Duration
	lastActive   time.Time
	lastInactive time.Time
	currentState bool // true = active, false = inactive
}

// NewStateDebouncer creates a new debouncer with the specified delays.
func NewStateDebouncer(onDelay, offDelay time.Duration) *StateDebouncer {
	return &StateDebouncer{
		onDelay:  onDelay,
		offDelay: offDelay,
	}
}

// Update evaluates the current activity and returns whether the state changed and the new state.
func (d *StateDebouncer) Update(active bool) (stateChanged bool, newState bool) {
	now := time.Now()

	if active {
		d.lastInactive = time.Time{} // Reset inactive timer
		if d.lastActive.IsZero() {
			d.lastActive = now
		}

		if !d.currentState {
			if now.Sub(d.lastActive) >= d.onDelay {
				d.currentState = true
				return true, true
			}
		}
	} else {
		d.lastActive = time.Time{} // Reset active timer
		if d.lastInactive.IsZero() {
			d.lastInactive = now
		}

		if d.currentState {
			if now.Sub(d.lastInactive) >= d.offDelay {
				d.currentState = false
				return true, false
			}
		}
	}

	return false, d.currentState
}

// CurrentState returns the current state of the debouncer.
func (d *StateDebouncer) CurrentState() bool {
	return d.currentState
}
