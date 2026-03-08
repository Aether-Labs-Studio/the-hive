package dht

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPubSubBroker(t *testing.T) {
	// Node A (Broker for keyword "test-kw")
	idA := NewNodeID("broker-node")
	rtA := NewRoutingTable(idA)
	stA := NewInMemoryStorage()
	trA := NewTransport(idA, nil)
	roA := NewRouter(trA, rtA, stA)
	trA.SetHandler(roA)
	if err := trA.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("Node A failed to listen: %v", err)
	}
	defer trA.Stop()

	// Node B (Subscriber)
	idB := NewNodeID("subscriber-node")
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
	
	// Node C (Publisher)
	idC := NewNodeID("publisher-node")
	rtC := NewRoutingTable(idC)
	stC := NewInMemoryStorage()
	trC := NewTransport(idC, nil)
	roC := NewRouter(trC, rtC, stC)
	trC.SetHandler(roC)
	if err := trC.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("Node C failed to listen: %v", err)
	}
	defer trC.Stop()

	engineC := NewEngine(roC, nil)
	// We need a dummy sanitizer for engine.Share to work
	engineC.SetSanitizer(&dummySanitizer{})

	// Build network: B and C know A
	rtB.AddContact(idA, trA.Addr())
	rtC.AddContact(idA, trA.Addr())
	rtA.AddContact(idB, trB.Addr())
	rtA.AddContact(idC, trC.Addr())

	// 1. Node B Subscribes to "test-kw"
	engineB.Subscribe("test-kw")
	
	// Wait for subscription message to arrive at Broker (Node A)
	time.Sleep(100 * time.Millisecond)

	// Verify Node A (Broker) has the subscriber
	kwID := NewNodeID("test-kw")
	roA.subStore.mu.RLock()
	subs := roA.subStore.subscribers[kwID]
	roA.subStore.mu.RUnlock()
	
	found := false
	for _, sub := range subs {
		if sub.ID == idB {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Broker (Node A) did not record Node B as subscriber")
	}

	// 2. Node C shares content with "test-kw"
	content := "This is a pubsub message"
	topic := "test-kw"
	
	_, err := engineC.Share(topic, content, "")
	if err != nil {
		t.Fatalf("Share failed: %v", err)
	}
}

type dummySanitizer struct{}
func (d *dummySanitizer) ExtractAndInspectSecure(chunk []byte, expectedID NodeID) ([]byte, ed25519.PublicKey, string, error) {
	return chunk, nil, "", nil
}
func (d *dummySanitizer) Sanitize(raw []byte) []byte { return raw }
func (d *dummySanitizer) Sign(data []byte, parentID string) ([]byte, error) { return data, nil }
func (d *dummySanitizer) PackageChunk(sanitized []byte, parentID string) ([]byte, NodeID, error) {
	return sanitized, NewNodeID(string(sanitized)), nil
}
func (d *dummySanitizer) IsTopicBlocked(topic string) bool { return false }

func TestIndexMergingConcurrency(t *testing.T) {
	// Stress test for Set Merging with race detector
	id := NewNodeID("node-1")
	rt := NewRoutingTable(id)
	st := NewInMemoryStorage()
	tr := NewTransport(id, nil)
	ro := NewRouter(tr, rt, st)
	ro.SetSigner(&dummySanitizer{})

	keyword := "concurrency"
	kwID := NewNodeID(keyword)

	// Concurrent merges
	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			idx := IndexChunk{
				Kind:     IndexKind,
				Pointers: []string{fmt.Sprintf("chunk-%d", n)},
			}
			data, _ := json.Marshal(idx)
			
			// We need to wrap it in StorePayload
			sp := StorePayload{
				Key:  kwID,
				Data: data,
			}
			payload, _ := json.Marshal(sp)
			
			msg := Message{
				Type:     Store,
				Payload:  payload,
				SenderID: NewNodeID(fmt.Sprintf("sender-%d", n)),
				Version:  ProtocolVersion,
			}
			ro.handleStore(nil, msg)
		}(i)
	}

	wg.Wait()

	// Verify all chunks are present
	val, exists := st.Retrieve(kwID)
	if !exists {
		t.Fatalf("Index not found in storage")
	}
	
	var finalIdx IndexChunk
	json.Unmarshal(val, &finalIdx)
	if len(finalIdx.Pointers) != workers {
		t.Errorf("Expected %d pointers, got %d. Index: %+v", workers, len(finalIdx.Pointers), finalIdx)
	}
}
