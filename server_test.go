package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testServer(perIP int) (*server, *bytes.Buffer) {
	var buf bytes.Buffer
	cfg := config{maxBody: 1024, logBody: 16, perIPMinute: perIP}
	return newServer(cfg, newLogger(&buf), newFakes()), &buf
}

func events(t *testing.T, buf *bytes.Buffer) []event {
	t.Helper()
	var out []event
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var e event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad log line %q: %v", line, err)
		}
		out = append(out, e)
	}
	return out
}

func TestRoutes(t *testing.T) {
	cases := map[string]struct {
		method string
		trap   string
		status int
	}{
		"/":                       {"GET", "portal-login", 200},
		"/login":                  {"POST", "portal-login", 401},
		"/.env":                   {"GET", "dotenv", 200},
		"/app/.env.production":    {"GET", "dotenv", 200},
		"/.git/config":            {"GET", "git", 200},
		"/.aws/credentials":       {"GET", "aws-credentials", 200},
		"/wp-login.php":           {"GET", "wp-login", 200},
		"/wp-admin/":              {"GET", "wp-admin", 302},
		"/xmlrpc.php":             {"POST", "xmlrpc", 200},
		"/phpMyAdmin/index.php":   {"GET", "phpmyadmin", 200},
		"/actuator/env":           {"GET", "actuator", 200},
		"/vendor/phpunit/phpunit": {"POST", "phpunit", 200},
		"/cgi-bin/luci":           {"GET", "cgi-bin", 403},
		"/backup/site.zip":        {"GET", "backup", 403},
		"/api/v1/users":           {"GET", "api", 401},
		"/static/app.env.js":      {"GET", "not-found", 404},
		"/nothing-here":           {"GET", "not-found", 404},
	}
	for path, c := range cases {
		s, buf := testServer(100)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(c.method, path, nil))
		if rec.Code != c.status {
			t.Errorf("%s %s: status %d, want %d", c.method, path, rec.Code, c.status)
		}
		ev := events(t, buf)
		if len(ev) != 1 || ev[0].Trap != c.trap || ev[0].Status != c.status {
			t.Errorf("%s %s: logged %+v, want trap %s status %d", c.method, path, ev, c.trap, c.status)
		}
	}
}

func TestLogsClientAndTruncatedBody(t *testing.T) {
	s, buf := testServer(100)
	req := httptest.NewRequest("POST", "/login?next=%2Fadmin", strings.NewReader("username=admin&password=hunter2hunter2"))
	req.Header.Set("X-Real-Ip", "203.0.113.7")
	req.Header.Set("User-Agent", "curl/8")
	req.Header.Set("X-Custom", "probe")
	s.ServeHTTP(httptest.NewRecorder(), req)
	ev := events(t, buf)[0]
	if ev.Client != "203.0.113.7" || ev.Method != "POST" || ev.Query != "next=%2Fadmin" || ev.UserAgent != "curl/8" {
		t.Fatalf("unexpected event %+v", ev)
	}
	if ev.Body != "username=admin&p" || ev.BodyBytes != 38 {
		t.Fatalf("body %q (%d bytes), want the first 16 bytes of 38", ev.Body, ev.BodyBytes)
	}
	if ev.Headers["X-Custom"] != "probe" || ev.Headers["X-Real-Ip"] != "" {
		t.Fatalf("headers %v", ev.Headers)
	}
}

func TestPerIPLogLimit(t *testing.T) {
	s, buf := testServer(2)
	for range 5 {
		req := httptest.NewRequest("GET", "/.env", nil)
		req.Header.Set("X-Real-Ip", "198.51.100.1")
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
	}
	if n := len(events(t, buf)); n != 2 {
		t.Fatalf("logged %d requests, want 2", n)
	}
	allowed, suppressed := s.limiter.allow("198.51.100.1", time.Now().Add(2*time.Minute))
	if !allowed || suppressed != 3 {
		t.Fatalf("after the window: allowed %v suppressed %d, want true 3", allowed, suppressed)
	}
}

func TestFakeSecretsDifferPerInstance(t *testing.T) {
	a, b := newFakes(), newFakes()
	if a.dbPassword == b.dbPassword || len(a.accessKey) != 20 || !strings.HasPrefix(a.accessKey, "AKIA") {
		t.Fatalf("fakes %+v %+v", a, b)
	}
}
