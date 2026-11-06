package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Limiter struct {
	limit   rate.Limit
	burst   int
	mu      sync.Mutex
	entries map[string]*entry
	stop    chan struct{}
}

type entry struct {
	limiter *rate.Limiter
	seen    time.Time
}

func New(limit rate.Limit, burst int) *Limiter {
	if burst <= 0 {
		burst = 1
	}
	l := &Limiter{
		limit:   limit,
		burst:   burst,
		entries: make(map[string]*entry),
		stop:    make(chan struct{}),
	}
	go l.cleanup()
	return l
}

func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	e, ok := l.entries[key]
	if !ok {
		e = &entry{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.entries[key] = e
	}
	e.seen = now
	allowed := e.limiter.Allow()
	l.mu.Unlock()
	return allowed
}

func (l *Limiter) Stop() { close(l.stop) }

func (l *Limiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().Add(-30 * time.Minute)
			l.mu.Lock()
			for k, e := range l.entries {
				if e.seen.Before(cutoff) {
					delete(l.entries, k)
				}
			}
			l.mu.Unlock()
		case <-l.stop:
			return
		}
	}
}
