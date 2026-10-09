package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Syt100/notify-relay/internal/bridge"
	"github.com/Syt100/notify-relay/internal/config"
	"github.com/Syt100/notify-relay/internal/provider"
	"github.com/Syt100/notify-relay/internal/queue"
)

var version = "dev"

func probe(endpoint string) error {
	port := os.Getenv("HEALTH_PORT")
	if port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, e := client.Get("http://127.0.0.1:" + port + endpoint)
	if e != nil {
		return fmt.Errorf("health endpoint unreachable")
	}
	defer r.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 65536))
	if r.StatusCode != 200 {
		return fmt.Errorf("health endpoint HTTP %d", r.StatusCode)
	}
	return nil
}

// Queue maintenance is offline: stop the bridge before opening its state file.
func failed(args []string) error {
	if len(args) != 1 && len(args) != 3 {
		return fmt.Errorf("usage: failed list | failed retry TOPIC ID")
	}
	if args[0] != "list" && args[0] != "retry" {
		return fmt.Errorf("usage: failed list | failed retry TOPIC ID")
	}
	if (args[0] == "list" && len(args) != 1) || (args[0] == "retry" && len(args) != 3) {
		return fmt.Errorf("usage: failed list | failed retry TOPIC ID")
	}
	c, err := config.Load()
	if err != nil {
		return err
	}
	q, err := queue.Open(c.DB, c.MaxPending)
	if err != nil {
		return fmt.Errorf("cannot open state database; stop the bridge before maintenance")
	}
	defer q.Close()
	if args[0] == "retry" {
		if err = q.Requeue(args[1], args[2]); err != nil {
			return err
		}
		fmt.Println("requeued")
		return nil
	}
	items, err := q.ListFailures(100)
	if err != nil {
		return err
	}
	n, err := q.FailedCount()
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"failed": n, "items": items, "limit": 100})
}

func run() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version":
			fmt.Println(version)
			return nil
		case "healthcheck":
			return probe("/healthz")
		case "readycheck":
			return probe("/readyz")
		case "failed":
			return failed(os.Args[2:])
		case "help", "--help", "-h":
			fmt.Println("notify-relay [version|healthcheck|readycheck|failed list|failed retry TOPIC ID]\nStop the bridge before failed queue maintenance.\nConfiguration: environment variables; see README.md.")
			return nil
		default:
			return fmt.Errorf("unknown command")
		}
	}
	c, e := config.Load()
	if e != nil {
		return e
	}
	level := slog.LevelInfo
	_ = level.UnmarshalText([]byte(c.LogLevel))
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	q, e := queue.Open(c.DB, c.MaxPending)
	if e != nil {
		return fmt.Errorf("open state database: %w", e)
	}
	defer q.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	s := bridge.New(c, q, provider.New(c.API, c.SPT, c.SendTimeout), log)
	server := &http.Server{Addr: c.HealthAddr, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	serverDone := make(chan error, 1)
	go func() {
		e := server.ListenAndServe()
		if e == http.ErrServerClosed {
			e = nil
		}
		serverDone <- e
		if e != nil {
			cancel()
		}
	}()
	log.Info("bridge started", "version", version, "topics", c.Topics, "max_pending", c.MaxPending)
	e = s.Run(ctx)
	shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		_ = server.Close()
	}
	serverErr := <-serverDone
	if e != nil {
		return e
	}
	if serverErr != nil {
		return serverErr
	}
	return shutdownErr
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
