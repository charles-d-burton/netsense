package main

import (
	"testing"
	"time"
)

func TestExponentialBackoff(t *testing.T) {
	base := 1 * time.Second
	max := 10 * time.Second

	tests := []struct {
		attempt int
		expect  time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 10 * time.Second}, // Capped at max
		{-1, 1 * time.Second}, // Negative attempt
	}

	for _, tt := range tests {
		got := exponentialBackoff(tt.attempt, base, max)
		if got != tt.expect {
			t.Errorf("exponentialBackoff(%d) = %v; want %v", tt.attempt, got, tt.expect)
		}
	}
}

func TestRandInt16_Distribution(t *testing.T) {
	seen := make(map[uint16]bool)
	for i := 0; i < 1000; i++ {
		val := randInt16()
		if val == 0 {
			t.Errorf("randInt16() returned 0")
		}
		seen[val] = true
	}
	if len(seen) < 900 {
		t.Errorf("randInt16() doesn't seem very random, only saw %d unique values in 1000 trials", len(seen))
	}
}
