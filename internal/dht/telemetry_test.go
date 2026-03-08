package dht

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	// Wait for goroutine with a retry loop or sufficient sleep
	var line string
	for i := 0; i < 10; i++ {
		line = buf.String()
		if line != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !strings.HasSuffix(line, "\n") {
		t.Fatalf("Telemetry should emit a newline-terminated string, got: %q", line)
	}

	var event Event
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("Failed to unmarshal telemetry event: %v", err)
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
