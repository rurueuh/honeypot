package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

type lokiSink struct {
	url      string
	client   *http.Client
	events   chan event
	batch    int
	interval time.Duration
	dropped  atomic.Int64
	done     chan struct{}
}

func newLokiSink(url string, buffer, batch int, interval time.Duration) *lokiSink {
	return &lokiSink{
		url:      url,
		client:   &http.Client{Timeout: 10 * time.Second},
		events:   make(chan event, buffer),
		batch:    batch,
		interval: interval,
		done:     make(chan struct{}),
	}
}

func (s *lokiSink) write(e event) {
	select {
	case s.events <- e:
	default:
		s.dropped.Add(1)
	}
}

func (s *lokiSink) run(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	pending := make([]event, 0, s.batch)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		if err := s.push(ctx, pending); err != nil {
			log.Printf("loki push of %d events failed: %v", len(pending), err)
		}
		pending = pending[:0]
	}
	for {
		select {
		case e := <-s.events:
			pending = append(pending, e)
			if len(pending) >= s.batch {
				flush()
			}
		case <-ticker.C:
			if n := s.dropped.Swap(0); n > 0 {
				pending = append(pending, event{Time: time.Now(), Event: "dropped", Suppressed: int(n)})
			}
			flush()
		case <-ctx.Done():
			for {
				select {
				case e := <-s.events:
					pending = append(pending, e)
				default:
					flush()
					return
				}
			}
		}
	}
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

func (s *lokiSink) push(ctx context.Context, events []event) error {
	streams := map[string]*lokiStream{}
	var order []string
	for _, e := range events {
		flagged := e.WAFFlagged
		if flagged == "" {
			flagged = "unknown"
		}
		key := e.Event + "|" + flagged
		st := streams[key]
		if st == nil {
			st = &lokiStream{Stream: map[string]string{"job": "honeypot", "event": e.Event, "waf_flagged": flagged}}
			streams[key] = st
			order = append(order, key)
		}
		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		st.Values = append(st.Values, [2]string{strconv.FormatInt(e.Time.UnixNano(), 10), string(line)})
	}
	body := struct {
		Streams []*lokiStream `json:"streams"`
	}{}
	for _, k := range order {
		body.Streams = append(body.Streams, streams[k])
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var last error
	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * time.Second):
			case <-ctx.Done():
			}
		}
		req, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodPost, s.url, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.client.Do(req)
		if err != nil {
			last = err
			continue
		}
		resp.Body.Close()
		if resp.StatusCode/100 == 2 {
			return nil
		}
		last = fmt.Errorf("status %d", resp.StatusCode)
		if resp.StatusCode/100 == 4 && resp.StatusCode != http.StatusTooManyRequests {
			return last
		}
	}
	return last
}
