package dht

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestWorkersLifecycle(t *testing.T) {
	id := NewNodeID("node-test")
	rt := NewRoutingTable(id)
	st := NewInMemoryStorage()
	tr := NewTransport(id, nil)
	ro := NewRouter(tr, rt, st)
	eng := NewEngine(ro, nil)

	ctx, cancel := context.WithCancel(context.Background())
	
	opts := WorkerOptions{
		RefreshInterval:     10 * time.Millisecond,
		ReplicationInterval: 10 * time.Millisecond,
		TopologyInterval:    10 * time.Millisecond,
		GCInterval:          10 * time.Millisecond,
	}

	// Start workers
	eng.StartWorkers(ctx, opts)
	
	// Let them run for a bit
	time.Sleep(50 * time.Millisecond)

	// Cancel context to stop workers
	cancel()

	// Wait for cleanup
	time.Sleep(20*time.Millisecond)
}

func TestReplicationWorker(t *testing.T) {
	// Node A (Storage source)
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

	// Node B (Storage target)
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

	// Connect A and B
	rtA.AddContact(idB, trB.Addr())
	rtB.AddContact(idA, trA.Addr())

	// Add data to A
	key := NewNodeID("test-data")
	data := []byte("hello hive")
	stA.Store(key, data, StateCommitted)

	engA := NewEngine(roA, nil)

	// Run replication manually
	engA.replicateData()

	// Wait for async STORE requests
	time.Sleep(100 * time.Millisecond)

	// Verify Node B has the data
	retrieved, ok := stB.Retrieve(key)
	if !ok {
		t.Errorf("Node B should have received replicated data")
	}
	if string(retrieved) != string(data) {
		t.Errorf("Content mismatch: got %s, want %s", string(retrieved), string(data))
	}
}

func TestHandoffWorker(t *testing.T) {
	// Node A (Storage source)
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

	// Node B (Storage target, initially unknown)
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

	// Add data to A
	key := NewNodeID("data-to-handoff")
	data := []byte("handoff content")
	stA.Store(key, data, StateCommitted)

	engA := NewEngine(roA, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start workers (this starts handoffWorker)
	engA.StartWorkers(ctx, WorkerOptions{
		RefreshInterval:     1 * time.Hour,
		ReplicationInterval: 1 * time.Hour,
		TopologyInterval:    1 * time.Hour,
		GCInterval:          1 * time.Hour,
	})

	// Simulate Node B joining by sending a PING to Node A
	pingMsg := Message{
		Type:          Ping,
		Version:       ProtocolVersion,
		TransactionID: "test-ping",
		SenderID:      idB,
	}
	roA.HandleMessage(trB.Addr(), pingMsg)

	// Node A should have detected idB and triggered proactiveReplication
	
	// Wait for async STORE requests from handoff
	deadline := time.Now().Add(500 * time.Millisecond)
	var retrieved []byte
	var ok bool
	for time.Now().Before(deadline) {
		retrieved, ok = stB.Retrieve(key)
		if ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !ok {
		t.Errorf("Node B should have received proactive handoff data")
	}
	if string(retrieved) != string(data) {
		t.Errorf("Handoff content mismatch: got %s, want %s", string(retrieved), string(data))
	}
}

// mockAddr implements net.Addr for testing
type mockAddr struct {
	network string
	address string
}

func (a mockAddr) Network() string { return a.network }
func (a mockAddr) String() string  { return a.address }

func TestHandoffTriggerLimit(t *testing.T) {
	idA := NewNodeID("node-a")
	rtA := NewRoutingTable(idA)
	stA := NewInMemoryStorage()
	trA := NewTransport(idA, nil)
	roA := NewRouter(trA, rtA, stA)

	// Fill the handoff channel
	for i := 0; i < 70; i++ {
		addr := mockAddr{"udp", fmt.Sprintf("127.0.0.1:%d", 1000+i)}
		id := NewNodeID(fmt.Sprintf("node-%d", i))
		roA.HandleMessage(addr, Message{Type: Ping, Version: ProtocolVersion, SenderID: id})
	}
	
	// No panic expected, channel full means some drops but that's what we defined.
}
