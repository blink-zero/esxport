package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestGracefulShutdown(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:    ":0",
		Handler: mux,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runServer(ctx, server, func() {})
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Trigger shutdown via context cancellation (simulates signal)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected nil error on graceful shutdown, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5 seconds")
	}
}

func TestGracefulShutdownCallsCleanup(t *testing.T) {
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    ":0",
		Handler: mux,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanupCalled := false
	errCh := make(chan error, 1)
	go func() {
		errCh <- runServer(ctx, server, func() {
			cleanupCalled = true
		})
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5 seconds")
	}

	if !cleanupCalled {
		t.Error("expected cleanup function to be called on shutdown")
	}
}
