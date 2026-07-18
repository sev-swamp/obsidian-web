package api

import (
	"sync"
	"time"
)

// loginLimiter throttles password brute force: it tracks recent failed
// attempts per key (username + client IP) and blocks further tries once
// the budget is exhausted. A successful login clears the key.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	max      int
	window   time.Duration
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{failures: map[string][]time.Time{}, max: max, window: window}
}

// Allow reports whether another attempt is permitted for the key.
func (l *loginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key)) < l.max
}

// Fail records a failed attempt.
func (l *loginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[key] = append(l.prune(key), time.Now())
}

// Reset clears the key after a successful login.
func (l *loginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

// prune drops attempts outside the window; caller holds the lock.
func (l *loginLimiter) prune(key string) []time.Time {
	cutoff := time.Now().Add(-l.window)
	kept := l.failures[key][:0]
	for _, t := range l.failures[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = kept
	return kept
}
