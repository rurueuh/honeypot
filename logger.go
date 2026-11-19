package main

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

type event struct {
	Time       time.Time         `json:"ts"`
	Event      string            `json:"event"`
	Client     string            `json:"client"`
	Method     string            `json:"method,omitempty"`
	Host       string            `json:"host,omitempty"`
	URI        string            `json:"uri,omitempty"`
	Path       string            `json:"path,omitempty"`
	Query      string            `json:"query,omitempty"`
	Proto      string            `json:"proto,omitempty"`
	UserAgent  string            `json:"ua,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	BodyBytes  int64             `json:"body_bytes,omitempty"`
	Trap       string            `json:"trap,omitempty"`
	Status     int               `json:"status,omitempty"`
	DurationMS int64             `json:"duration_ms,omitempty"`
	Suppressed int               `json:"suppressed,omitempty"`
}

type logger struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func newLogger(w io.Writer) *logger {
	return &logger{enc: json.NewEncoder(w)}
}

func (l *logger) write(e event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.enc.Encode(e)
}
