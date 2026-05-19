package ratelimit

import (
	"sync"
	"time"
)

const (
	maxRequests = 5
	windowSize  = 60 * time.Second
)

type userState struct {
	timestamps []time.Time
	accepted   int64
	rejected   int64
}

// Stats holds accepted and rejected counts for a user.
type Stats struct {
	Accepted int64
	Rejected int64
}

// Limiter is a concurrency-safe sliding window rate limiter.
type Limiter struct {
	mu    sync.Mutex
	users map[string]*userState
}

// New returns a ready-to-use Limiter.
func New() *Limiter {
	return &Limiter{users: make(map[string]*userState)}
}

// Allow returns true if the request is within the rate limit for userID.
func (l *Limiter) Allow(userID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-windowSize)

	s, ok := l.users[userID]
	if !ok {
		s = &userState{}
		l.users[userID] = s
	}

	// Evict timestamps outside the rolling window.
	valid := s.timestamps[:0]
	for _, t := range s.timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	s.timestamps = valid

	if len(s.timestamps) >= maxRequests {
		s.rejected++
		return false
	}

	s.timestamps = append(s.timestamps, now)
	s.accepted++
	return true
}

// Stats returns the accepted count in the current rolling window and the
// cumulative rejected count for userID.
func (l *Limiter) Stats(userID string) Stats {
	l.mu.Lock()
	defer l.mu.Unlock()

	s, ok := l.users[userID]
	if !ok {
		return Stats{}
	}

	// Evict stale timestamps so Accepted reflects only the current window.
	now := time.Now()
	cutoff := now.Add(-windowSize)
	valid := s.timestamps[:0]
	for _, t := range s.timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	s.timestamps = valid

	return Stats{Accepted: int64(len(s.timestamps)), Rejected: s.rejected}
}
