package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type fakeLoki struct {
	mu      sync.Mutex
	streams []lokiStream
}

func (f *fakeLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	var body struct {
		Streams []lokiStream `json:"streams"`
	}
	if r.URL.Path != "/loki/api/v1/push" || json.Unmarshal(data, &body) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.streams = append(f.streams, body.Streams...)
	f.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeLoki) lines() map[string][]event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string][]event{}
	for _, st := range f.streams {
		for _, v := range st.Values {
			var e event
			_ = json.Unmarshal([]byte(v[1]), &e)
			out[st.Stream["event"]+"/"+st.Stream["waf_flagged"]] = append(out[st.Stream["event"]+"/"+st.Stream["waf_flagged"]], e)
		}
	}
	return out
}

func TestEventsReachLokiWithWAFVerdict(t *testing.T) {
	fl := &fakeLoki{}
	ts := httptest.NewServer(fl)
	defer ts.Close()
	sinkLoki := newLokiSink(ts.URL+"/loki/api/v1/push", 100, 10, 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	go sinkLoki.run(ctx)

	s := newServer(config{maxBody: 1024, logBody: 64, perIPMinute: 100}, sinkLoki, newFakes())
	flagged := httptest.NewRequest("GET", "/.env", nil)
	flagged.Header.Set("X-Waf-Flagged", "true")
	flagged.Header.Set("X-Waf-Rules", "930120,913200")
	flagged.Header.Set("X-Waf-Score", "13")
	flagged.Header.Set("X-Waf-Request-Id", "DEA27657-4C38063B")
	s.ServeHTTP(httptest.NewRecorder(), flagged)
	missed := httptest.NewRequest("GET", "/actuator/env", nil)
	missed.Header.Set("X-Waf-Flagged", "false")
	missed.Header.Set("X-Waf-Rules", "none")
	s.ServeHTTP(httptest.NewRecorder(), missed)

	cancel()
	<-sinkLoki.done
	got := fl.lines()
	f := got["request/true"]
	if len(f) != 1 || f[0].WAFRules != "930120,913200" || f[0].WAFRequestID != "DEA27657-4C38063B" || f[0].Trap != "dotenv" {
		t.Fatalf("flagged stream %+v", got)
	}
	if _, leaked := f[0].Headers["X-Waf-Rules"]; leaked {
		t.Fatalf("x-waf headers duplicated in headers: %v", f[0].Headers)
	}
	m := got["request/false"]
	if len(m) != 1 || m[0].Path != "/actuator/env" || m[0].WAFRules != "none" {
		t.Fatalf("missed stream %+v", got)
	}
}

func TestLokiSinkCountsDropsWhenFull(t *testing.T) {
	s := newLokiSink("http://127.0.0.1:1/push", 1, 10, time.Hour)
	s.write(event{Event: "request"})
	s.write(event{Event: "request"})
	s.write(event{Event: "request"})
	if n := s.dropped.Load(); n != 2 {
		t.Fatalf("dropped %d, want 2", n)
	}
}

func TestUnknownVerdictWithoutWAFHeaders(t *testing.T) {
	fl := &fakeLoki{}
	ts := httptest.NewServer(fl)
	defer ts.Close()
	s := newLokiSink(ts.URL+"/loki/api/v1/push", 10, 10, time.Hour)
	if err := s.push(context.Background(), []event{{Time: time.Now(), Event: "request", Path: "/"}}); err != nil {
		t.Fatal(err)
	}
	if got := fl.lines(); len(got["request/unknown"]) != 1 {
		t.Fatalf("got %v", got)
	}
}
