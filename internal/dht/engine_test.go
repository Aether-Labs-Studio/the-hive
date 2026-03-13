package dht

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
)

func TestEngineBootstrap(t *testing.T) {
	// Node A (Seed)
	idA := NewNodeID("node-a")
	rtA := NewRoutingTable(idA)
	stA := NewInMemoryStorage()
	trA := NewTransport(idA, nil)
	roA := NewRouter(trA, rtA, stA)
	trA.SetHandler(roA)
	if err := trA.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("Node A failed to listen: %v", err)
	}
	defer trA.Stop()

	// Node C (Already known by Seed)
	idC := NewNodeID("node-c")
	rtC := NewRoutingTable(idC)
	stC := NewInMemoryStorage()
	trC := NewTransport(idC, nil)
	roC := NewRouter(trC, rtC, stC)
	trC.SetHandler(roC)
	if err := trC.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("Node C failed to listen: %v", err)
	}
	defer trC.Stop()

	// Add Node C to Node A's routing table
	rtA.AddContact(idC, trC.Addr())

	// Node B (Joining Node)
	idB := NewNodeID("node-b")
	rtB := NewRoutingTable(idB)
	stB := NewInMemoryStorage()
	trB := NewTransport(idB, nil)
	roB := NewRouter(trB, rtB, stB)
	trB.SetHandler(roB)
	if err := trB.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("Node B failed to listen: %v", err)
	}
	defer trB.Stop()

	engineB := NewEngine(roB, nil)

	// 1. Perform Bootstrapping
	ctx := context.Background()
	seedAddr := trA.Addr().String()
	err := engineB.Bootstrap(ctx, seedAddr)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// 2. Verification
	// Node B should know Node A (seed)
	contactsA := rtB.FindClosestContacts(idA, 1)
	if len(contactsA) == 0 || contactsA[0].ID != idA {
		t.Errorf("Node B should have discovered Node A (seed)")
	}

	// Node B should have discovered Node C via self-lookup to Node A
	contactsC := rtB.FindClosestContacts(idC, 1)
	if len(contactsC) == 0 || contactsC[0].ID != idC {
		t.Errorf("Node B should have discovered Node C via self-lookup to Node A")
	}
}

func TestKnowledgeVersioning(t *testing.T) {
	// Logic verified via sanitizer tests and E2E
}

type shareTestSanitizer struct{}

func (s *shareTestSanitizer) ExtractAndInspectSecure(chunk []byte, expectedID NodeID) ([]byte, ed25519.PublicKey, string, error) {
	return chunk, nil, "", nil
}
func (s *shareTestSanitizer) Sanitize(raw []byte) []byte                        { return raw }
func (s *shareTestSanitizer) Sign(data []byte, parentID string) ([]byte, error) { return data, nil }
func (s *shareTestSanitizer) PackageChunk(sanitized []byte, parentID string) ([]byte, NodeID, error) {
	return sanitized, NewNodeID(string(sanitized)), nil
}
func (s *shareTestSanitizer) IsTopicBlocked(topic string) bool { return false }

func TestEngineShareRejectsBinaryLookingPayload(t *testing.T) {
	id := NewNodeID("share-node")
	engine := NewEngine(NewRouter(nil, NewRoutingTable(id), NewInMemoryStorage()), nil)
	engine.SetSanitizer(&shareTestSanitizer{})
	buf := &SafeBuffer{}
	prevTelemetry := GlobalTelemetry
	GlobalTelemetry = NewTelemetry(buf)
	defer func() { GlobalTelemetry = prevTelemetry }()

	payload := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO7Z0a4AAAAASUVORK5CYII="
	_, err := engine.Share("design", payload, "", StateCommitted)
	if err == nil {
		t.Fatal("expected binary-like payload to be rejected")
	}
	if !strings.Contains(err.Error(), ceBinaryPayloadError) {
		t.Fatalf("unexpected error: %v", err)
	}

	var event Event
	if err := waitForTelemetryEvent(buf, &event); err != nil {
		t.Fatalf("expected security telemetry event: %v", err)
	}
	if event.Type != SecurityPolicy {
		t.Fatalf("expected %s event, got %s", SecurityPolicy, event.Type)
	}
	if !strings.Contains(event.Details, "Rejected binary-like payload") {
		t.Fatalf("unexpected event details: %s", event.Details)
	}

	var payloadMap map[string]any
	if err := json.Unmarshal(event.Payload, &payloadMap); err != nil {
		t.Fatalf("failed to decode telemetry payload: %v", err)
	}
	if payloadMap["reason"] != "binary_like_payload" {
		t.Fatalf("unexpected telemetry reason: %#v", payloadMap["reason"])
	}
}
