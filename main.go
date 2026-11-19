package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	cfg := config{
		addr:        envOr("ADDR", ":8080"),
		maxBody:     envInt("MAX_BODY", 64<<10),
		logBody:     envInt("LOG_BODY", 4<<10),
		perIPMinute: envInt("LOG_PER_IP_MINUTE", 120),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var out sink = newLogger(os.Stdout)
	var loki *lokiSink
	if url := os.Getenv("LOKI_URL"); url != "" {
		loki = newLokiSink(url, envInt("LOKI_BUFFER", 10000), envInt("LOKI_BATCH", 500), 2*time.Second)
		go loki.run(ctx)
		out = loki
		log.Printf("sending events to %s", url)
	}

	srv := &http.Server{
		Addr:              cfg.addr,
		Handler:           newServer(cfg, out, newFakes()),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("listening on %s", cfg.addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	if loki != nil {
		select {
		case <-loki.done:
		case <-time.After(10 * time.Second):
		}
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}
