package main

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type config struct {
	addr        string
	maxBody     int
	logBody     int
	perIPMinute int
}

type server struct {
	cfg     config
	logger  *logger
	fakes   fakes
	limiter *limiter
}

func newServer(cfg config, l *logger, f fakes) *server {
	return &server{cfg: cfg, logger: l, fakes: f, limiter: newLimiter(cfg.perIPMinute, time.Minute)}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	body, bodyBytes := readBody(r, s.cfg.maxBody)
	t := route(r)
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	w.Header().Set("Server", "Apache/2.4.58 (Ubuntu)")
	t.respond(rec, r, s.fakes)

	client := clientIP(r)
	allowed, suppressed := s.limiter.allow(client, start)
	if suppressed > 0 {
		s.logger.write(event{Time: start, Event: "suppressed", Client: client, Suppressed: suppressed})
	}
	if !allowed {
		return
	}
	s.logger.write(event{
		Time:       start,
		Event:      "request",
		Client:     client,
		Method:     r.Method,
		Host:       r.Host,
		URI:        truncate(r.RequestURI, 2048),
		Path:       r.URL.Path,
		Query:      truncate(r.URL.RawQuery, 2048),
		Proto:      r.Proto,
		UserAgent:  truncate(r.UserAgent(), 512),
		Headers:    headers(r.Header),
		Body:       truncate(body, s.cfg.logBody),
		BodyBytes:  bodyBytes,
		Trap:       t.name,
		Status:     rec.status,
		DurationMS: time.Since(start).Milliseconds(),
	})
}

func readBody(r *http.Request, limit int) (string, int64) {
	if r.Body == nil {
		return "", 0
	}
	data, _ := io.ReadAll(io.LimitReader(r.Body, int64(limit)))
	rest, _ := io.Copy(io.Discard, io.LimitReader(r.Body, 8<<20))
	return string(data), int64(len(data)) + rest
}

func clientIP(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Real-Ip")); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

var skippedHeaders = map[string]bool{
	"X-Forwarded-For":    true,
	"X-Forwarded-Host":   true,
	"X-Forwarded-Port":   true,
	"X-Forwarded-Proto":  true,
	"X-Forwarded-Server": true,
	"X-Real-Ip":          true,
	"User-Agent":         true,
}

func headers(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if skippedHeaders[k] {
			continue
		}
		out[k] = truncate(strings.Join(v, ", "), 1024)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
