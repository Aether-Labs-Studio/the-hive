package dht

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
	"the-hive/internal/logger"
)

// EventType identifies the kind of P2P event occurring in the network.
type EventType string

const (
	PeerJoined     EventType = "PeerJoined"
	BucketRefresh  EventType = "BucketRefresh"
	DataReplicated EventType = "DataReplicated"
	RPCError       EventType = "RPCError"
	TopologySync   EventType = "TopologySync"
	TopicMatch     EventType = "TopicMatch"
)

// Event represents a structured telemetry record for P2P activity.
type Event struct {
	Type      EventType       `json:"type"`
	NodeID    string          `json:"node_id,omitempty"`
	Details   string          `json:"details,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// Telemetry provides a mechanism to emit structured events without blocking.
type Telemetry struct {
	writer  io.Writer
	mu      sync.Mutex
	clients map[chan Event]struct{}
	engine  *Engine // Reference to control network state
}

// NewTelemetry creates a new Telemetry instance with the specified writer.
func NewTelemetry(w io.Writer) *Telemetry {
	if w == nil {
		w = os.Stderr
	}
	return &Telemetry{
		writer:  w,
		clients: make(map[chan Event]struct{}),
	}
}

// SetEngine provides the engine reference for API control.
func (t *Telemetry) SetEngine(e *Engine) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.engine = e
}

// GlobalTelemetry is the default instance that writes to Stderr and manages SSE clients.
var GlobalTelemetry = NewTelemetry(os.Stderr)

// Emit serializes and writes an event to the telemetry sink and broadcasts to SSE clients.
func (t *Telemetry) Emit(eventType EventType, nodeID string, details string) {
	t.EmitWithPayload(eventType, nodeID, details, nil)
}

// EmitWithPayload allows emitting an event with a structured payload.
func (t *Telemetry) EmitWithPayload(eventType EventType, nodeID string, details string, payload any) {
	var rawPayload json.RawMessage
	if payload != nil {
		if p, ok := payload.([]byte); ok {
			rawPayload = p
		} else {
			rawData, _ := json.Marshal(payload)
			rawPayload = rawData
		}
	}

	event := Event{
		Type:      eventType,
		NodeID:    nodeID,
		Details:   details,
		Payload:   rawPayload,
		Timestamp: time.Now().UTC(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	// 1. Write to Stderr
	go func() {
		_, _ = t.writer.Write(append(data, '\n'))
	}()

	// 2. Broadcast
	t.mu.Lock()
	defer t.mu.Unlock()

	if eventType == EventType("KeywordSeen") && t.engine != nil {
		t.engine.TrackKeyword(details)
	}

	for ch := range t.clients {
		select {
		case ch <- event:
		default:
		}
	}
}

// ServeHTTP implements http.Handler for the SSE endpoint and API.
func (t *Telemetry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Enable CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Routes
	switch {
	case r.URL.Path == "/events":
		t.handleSSE(w, r)
	case r.URL.Path == "/api/state" && r.Method == http.MethodPost:
		t.handleAPIState(w, r)
	case r.URL.Path == "/api/search" && r.Method == http.MethodGet:
		t.handleAPISearch(w, r)
	case r.URL.Path == "/api/share" && r.Method == http.MethodPost:
		t.handleAPIShare(w, r)
	case r.URL.Path == "/api/rate" && r.Method == http.MethodPost:
		t.handleAPIRate(w, r)
	case r.URL.Path == "/api/subscribe" && r.Method == http.MethodPost:
		t.handleAPISubscribe(w, r)
	case r.URL.Path == "/api/subscriptions" && r.Method == http.MethodGet:
		t.handleAPIGetSubscriptions(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (t *Telemetry) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan Event, 16)
	t.mu.Lock()
	t.clients[ch] = struct{}{}
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.clients, ch)
		t.mu.Unlock()
	}()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	for {
		select {
		case event := <-ch:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (t *Telemetry) handleAPIState(w http.ResponseWriter, r *http.Request) {
	var req struct { State NetworkState `json:"state"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if t.engine == nil { http.Error(w, "Engine not ready", 503); return }
	if err := t.engine.SetState(req.State, ""); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(200)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "state": req.State})
}

func (t *Telemetry) handleAPISearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" { http.Error(w, "Missing q parameter", 400); return }
	if t.engine == nil { http.Error(w, "Engine not ready", 503); return }
	
	res, err := t.engine.Search(q)
	if err != nil { http.Error(w, err.Error(), 500); return }
	
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"query": q, "result": res})
}

func (t *Telemetry) handleAPIShare(w http.ResponseWriter, r *http.Request) {
	var req struct { 
		Topic    string `json:"topic"` 
		Content  string `json:"content"` 
		ParentID string `json:"parent_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", 400); return
	}
	if t.engine == nil { http.Error(w, "Engine not ready", 503); return }
	
	res, err := t.engine.Share(req.Topic, req.Content, req.ParentID)
	if err != nil { http.Error(w, err.Error(), 500); return }
	
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "message": res})
}

func (t *Telemetry) handleAPIRate(w http.ResponseWriter, r *http.Request) {
	var req struct { 
		ChunkID string `json:"chunk_id"` 
		Score   int    `json:"score"` 
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", 400); return
	}
	if t.engine == nil { http.Error(w, "Engine not ready", 503); return }
	
	res, err := t.engine.Rate(req.ChunkID, req.Score)
	if err != nil { http.Error(w, err.Error(), 500); return }
	
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "message": res})
}

func (t *Telemetry) handleAPISubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct { 
		Keyword string `json:"keyword"` 
		Action  string `json:"action"` 
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", 400); return
	}
	if t.engine == nil { http.Error(w, "Engine not ready", 503); return }
	
	if req.Action == "unsubscribe" {
		t.engine.Unsubscribe(req.Keyword)
	} else {
		t.engine.Subscribe(req.Keyword)
	}
	w.WriteHeader(200)
}

func (t *Telemetry) handleAPIGetSubscriptions(w http.ResponseWriter, r *http.Request) {
	if t.engine == nil { http.Error(w, "Engine not ready", 503); return }
	subs := t.engine.GetAllSubscriptions()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"subscriptions": subs})
}

// StartMonitor launches the HTTP server for the Live Monitor.
func StartMonitor(addr string) {
	mux := http.NewServeMux()
	
	// Root: Serve the embedded HTML with anti-cache headers
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("ETag", fmt.Sprintf(`"%d"`, len(monitorHTML)))
		_, _ = w.Write(monitorHTML)
	})

	// Unified handler for SSE and API
	mux.Handle("/events", GlobalTelemetry)
	mux.Handle("/api/", GlobalTelemetry)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	fmt.Fprintf(os.Stderr, "  → Telemetry & REST API at http://%s\n", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Error starting monitor: %v", err)
	}
}
