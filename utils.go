package main

import (
	"math/rand/v2"
	"time"
)

const maxReconnectBackoff = 5 * time.Minute

func exponentialBackoff(attempt int, base, max time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	backoff := base * time.Duration(1<<uint(attempt))
	if backoff > max || backoff < 0 {
		return max
	}
	return backoff
}

func randInt16() uint16 {
	a := rand.Uint32()
	a %= (65535 - 1)
	a += 1
	return uint16(a)
}
