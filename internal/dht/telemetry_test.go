package dht

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// SafeBuffer is a thread-safe wrapper around bytes.Buffer.
type SafeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *SafeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *SafeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestTelemetryEmit(t *testing.T) {
	buf := &SafeBuffer{}
	telemetry := NewTelemetry(buf)

	id := NewNodeID("test-node")
	details := "joined from 127.0.0.1"
	telemetry.Emit(PeerJoined, fmt.Sprintf("%x", id), details)

	var event Event
	if err := waitForTelemetryEvent(buf, &event); err != nil {
		t.Fatalf("Failed to read telemetry event: %v", err)
	}

	if event.Type != PeerJoined {
		t.Errorf("Expected event type %s, got %s", PeerJoined, event.Type)
	}

	if event.NodeID != fmt.Sprintf("%x", id) {
		t.Errorf("Expected node ID %x, got %s", id, event.NodeID)
	}

	if event.Details != details {
		t.Errorf("Expected details %s, got %s", details, event.Details)
	}

	if time.Since(event.Timestamp) > time.Second {
		t.Errorf("Event timestamp too old: %v", event.Timestamp)
	}
}

func TestTelemetryHandleAPIShareRejectsMultipart(t *testing.T) {
	buf := &SafeBuffer{}
	telemetry := NewTelemetry(buf)
	req := httptest.NewRequest(http.MethodPost, "/api/share", strings.NewReader("payload"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=abc123")
	rec := httptest.NewRecorder()

	telemetry.handleAPIShare(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Community Edition only accepts application/json") {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}

	var event Event
	if err := waitForTelemetryEvent(buf, &event); err != nil {
		t.Fatalf("expected security telemetry event: %v", err)
	}
	if event.Type != SecurityPolicy {
		t.Fatalf("expected %s event, got %s", SecurityPolicy, event.Type)
	}
}

func waitForTelemetryEvent(buf *SafeBuffer, event *Event) error {
	for i := 0; i < 20; i++ {
		line := buf.String()
		if line != "" {
			if !strings.HasSuffix(line, "\n") {
				return fmt.Errorf("telemetry should emit a newline-terminated string, got: %q", line)
			}
			if err := json.Unmarshal([]byte(line), event); err != nil {
				return err
			}
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("timeout waiting for telemetry")
}
