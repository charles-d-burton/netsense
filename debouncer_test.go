package main

import (
	"testing"
	"time"
)

func TestStateDebouncer(t *testing.T) {
	onDelay := 100 * time.Millisecond
	offDelay := 200 * time.Millisecond
	d := NewStateDebouncer(onDelay, offDelay)

	// Initial state
	if d.CurrentState() != false {
		t.Errorf("Initial state should be false")
	}

	// Active, but not for long enough
	changed, state := d.Update(true)
	if changed || state != false {
		t.Errorf("Should not have changed state yet")
	}

	time.Sleep(150 * time.Millisecond)

	// Active for long enough
	changed, state = d.Update(true)
	if !changed || state != true {
		t.Errorf("Should have changed to active")
	}

	// Inactive, but not for long enough
	changed, state = d.Update(false)
	if changed || state != true {
		t.Errorf("Should not have changed state back yet")
	}

	time.Sleep(150 * time.Millisecond)
	
	// Still not long enough (offDelay is 200ms)
	changed, state = d.Update(false)
	if changed || state != true {
		t.Errorf("Should still be active after 150ms")
	}

	time.Sleep(100 * time.Millisecond)

	// Inactive for long enough
	changed, state = d.Update(false)
	if !changed || state != false {
		t.Errorf("Should have changed to inactive")
	}
}
