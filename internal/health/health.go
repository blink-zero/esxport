package health

import (
	"encoding/json"
	"net/http"
	"sync"
)

// Handler tracks scrape health and serves health check endpoints.
type Handler struct {
	mu       sync.RWMutex
	successes map[string]bool
}

// NewHandler creates a new Handler.
func NewHandler() *Handler {
	return &Handler{
		successes: make(map[string]bool),
	}
}

// RecordSuccess marks a target's last scrape as successful.
func (h *Handler) RecordSuccess(target string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.successes[target] = true
}

// RecordFailure marks a target's last scrape as failed.
func (h *Handler) RecordFailure(target string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.successes[target] = false
}

// Healthz returns 200 OK unconditionally.
func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Readyz returns 200 if at least one target succeeded, 503 otherwise.
func (h *Handler) Readyz(w http.ResponseWriter, _ *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ready := false
	for _, ok := range h.successes {
		if ok {
			ready = true
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if ready {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
	}
}
