package vsphere

import (
	"context"
	"sync"
)

// ConnectFunc creates a new Client for the given config.
type ConnectFunc func(ctx context.Context, cfg ConnectConfig) (*Client, error)

type poolEntry struct {
	client *Client
	stale  bool
}

// Pool caches vSphere client connections keyed by host.
type Pool struct {
	mu      sync.Mutex
	entries map[string]*poolEntry
	connect ConnectFunc
}

// NewPool creates a connection pool with the given connect function.
func NewPool(connect ConnectFunc) *Pool {
	return &Pool{
		entries: make(map[string]*poolEntry),
		connect: connect,
	}
}

// Get returns a cached client or creates a new one. Reconnects stale sessions.
func (p *Pool) Get(ctx context.Context, cfg ConnectConfig) (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.entries[cfg.Host]
	if ok && !entry.stale {
		return entry.client, nil
	}

	// Close the stale client before replacing it to prevent connection leaks.
	if ok && entry.client != nil {
		_ = entry.client.Close(ctx)
	}

	client, err := p.connect(ctx, cfg)
	if err != nil {
		return nil, err
	}

	p.entries[cfg.Host] = &poolEntry{client: client}
	return client, nil
}

// MarkStale marks a host's connection as stale so it reconnects on next Get.
func (p *Pool) MarkStale(host string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[host]; ok {
		entry.stale = true
	}
}

// CloseAll closes all cached connections and clears the pool.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for host, entry := range p.entries {
		if entry.client != nil {
			_ = entry.client.Close(context.Background())
		}
		delete(p.entries, host)
	}
}
