package main

import (
	"sync"
	"time"
)

type window struct {
	start      time.Time
	count      int
	suppressed int
}

type limiter struct {
	mu     sync.Mutex
	limit  int
	period time.Duration
	byIP   map[string]*window
	swept  time.Time
}

func newLimiter(limit int, period time.Duration) *limiter {
	return &limiter{limit: limit, period: period, byIP: map[string]*window{}}
}

func (l *limiter) allow(ip string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.swept) > l.period {
		for k, w := range l.byIP {
			if now.Sub(w.start) > l.period && w.suppressed == 0 {
				delete(l.byIP, k)
			}
		}
		l.swept = now
	}
	w := l.byIP[ip]
	if w == nil {
		w = &window{start: now}
		l.byIP[ip] = w
	}
	flushed := 0
	if now.Sub(w.start) >= l.period {
		flushed = w.suppressed
		*w = window{start: now}
	}
	w.count++
	if w.count > l.limit {
		w.suppressed++
		return false, flushed
	}
	return true, flushed
}
