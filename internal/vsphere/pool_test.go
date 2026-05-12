package vsphere

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeConnector tracks connect calls for testing.
type fakeConnector struct {
	mu       sync.Mutex
	calls    int
	failNext bool
}

func newFakeConnector() *fakeConnector {
	return &fakeConnector{}
}

func (f *fakeConnector) connect(_ context.Context, cfg ConnectConfig) (*Client, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.failNext {
		f.failNext = false
		return nil, errors.New("connection failed")
	}
	return &Client{}, nil
}

func TestPoolGetReturnsNewClient(t *testing.T) {
	fc := newFakeConnector()
	pool := NewPool(fc.connect)

	cfg := ConnectConfig{Host: "host1", Username: "user", Password: "pass"}
	client, err := pool.Get(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if fc.calls != 1 {
		t.Errorf("expected 1 connect call, got %d", fc.calls)
	}
}

func TestPoolGetReturnsCachedClient(t *testing.T) {
	fc := newFakeConnector()
	pool := NewPool(fc.connect)

	cfg := ConnectConfig{Host: "host1", Username: "user", Password: "pass"}
	client1, err := pool.Get(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	client2, err := pool.Get(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client1 != client2 {
		t.Error("expected same client instance on second call")
	}
	if fc.calls != 1 {
		t.Errorf("expected 1 connect call (cached), got %d", fc.calls)
	}
}

func TestPoolGetReconnectsOnStaleSession(t *testing.T) {
	fc := newFakeConnector()
	pool := NewPool(fc.connect)

	cfg := ConnectConfig{Host: "host1", Username: "user", Password: "pass"}
	_, err := pool.Get(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mark the cached entry as stale
	pool.mu.Lock()
	pool.entries[cfg.Host].stale = true
	pool.mu.Unlock()

	_, err = pool.Get(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error on reconnect: %v", err)
	}

	if fc.calls != 2 {
		t.Errorf("expected 2 connect calls (reconnect), got %d", fc.calls)
	}
}

func TestPoolGetClosesStaleClient(t *testing.T) {
	staleClientClosed := false
	staleClient := &Client{
		onClose: func() { staleClientClosed = true },
	}

	callCount := 0
	pool := NewPool(func(_ context.Context, cfg ConnectConfig) (*Client, error) {
		callCount++
		if callCount == 1 {
			return staleClient, nil
		}
		return &Client{}, nil
	})

	cfg := ConnectConfig{Host: "host1"}
	if _, err := pool.Get(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mark as stale
	pool.MarkStale(cfg.Host)

	// Get should close the stale client before creating a new one
	newClient, err := pool.Get(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error on reconnect: %v", err)
	}

	if !staleClientClosed {
		t.Error("expected stale client to be closed before replacement")
	}
	if newClient == staleClient {
		t.Error("expected a new client, got the stale one")
	}
}

func TestPoolCloseAll(t *testing.T) {
	connectCalls := 0
	pool := NewPool(func(_ context.Context, cfg ConnectConfig) (*Client, error) {
		connectCalls++
		return &Client{}, nil
	})

	configs := []ConnectConfig{
		{Host: "host1"},
		{Host: "host2"},
	}
	for _, cfg := range configs {
		if _, err := pool.Get(context.Background(), cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	pool.CloseAll()

	pool.mu.Lock()
	remaining := len(pool.entries)
	pool.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected 0 entries after CloseAll, got %d", remaining)
	}
}

func TestPoolConcurrentAccess(t *testing.T) {
	var connectCount atomic.Int64
	pool := NewPool(func(_ context.Context, cfg ConnectConfig) (*Client, error) {
		connectCount.Add(1)
		return &Client{}, nil
	})

	cfg := ConnectConfig{Host: "host1", Username: "user", Password: "pass"}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := pool.Get(context.Background(), cfg)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	// Should have only connected once for the same host
	if connectCount.Load() != 1 {
		t.Errorf("expected 1 connect call for concurrent access, got %d", connectCount.Load())
	}
}
