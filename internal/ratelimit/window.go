package ratelimit

import (
	"sync"
	"time"
)

// Window is a fixed-window limiter suitable for API keys. It reports the
// remaining requests and reset delay so callers can expose rate-limit headers.
type Window struct {
	limit  int
	period time.Duration
	mu     sync.Mutex
	items  map[string]*windowItem
	stop   chan struct{}
}

type windowItem struct {
	count int
	reset time.Time
}

func NewWindow(limit int, period time.Duration) *Window {
	if limit < 1 {
		limit = 1
	}
	w := &Window{
		limit:  limit,
		period: period,
		items:  make(map[string]*windowItem),
		stop:   make(chan struct{}),
	}
	go w.cleanup()
	return w
}

func (w *Window) Allow(key string) (allowed bool, remaining int, reset time.Duration) {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	item, ok := w.items[key]
	if !ok || !now.Before(item.reset) {
		item = &windowItem{count: 0, reset: now.Add(w.period)}
		w.items[key] = item
	}
	if item.count >= w.limit {
		return false, 0, time.Until(item.reset)
	}
	item.count++
	return true, w.limit - item.count, time.Until(item.reset)
}

func (w *Window) Stop() { close(w.stop) }

func (w *Window) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().Add(-2 * w.period)
			w.mu.Lock()
			for key, item := range w.items {
				if item.reset.Before(cutoff) {
					delete(w.items, key)
				}
			}
			w.mu.Unlock()
		case <-w.stop:
			return
		}
	}
}
