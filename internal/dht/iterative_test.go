package dht

import (
	"testing"
)

func TestIterativeLookup(t *testing.T) {
	// Setup a 3-hop network: A -> B -> C -> D
	
	// Node D (Target node)
	idD := NewNodeID("node-d")
	rtD := NewRoutingTable(idD)
	stD := NewInMemoryStorage()
	trD := NewTransport(idD, nil)
	roD := NewRouter(trD, rtD, stD)
	trD.SetHandler(roD)
	trD.Listen("127.0.0.1:0")
	defer trD.Stop()

	// Node C (knows D)
	idC := NewNodeID("node-c")
	rtC := NewRoutingTable(idC)
	stC := NewInMemoryStorage()
	trC := NewTransport(idC, nil)
	roC := NewRouter(trC, rtC, stC)
	trC.SetHandler(roC)
	trC.Listen("127.0.0.1:0")
	defer trC.Stop()
	rtC.AddContact(idD, trD.Addr())

	// Node B (knows C)
	idB := NewNodeID("node-b")
	rtB := NewRoutingTable(idB)
	stB := NewInMemoryStorage()
	trB := NewTransport(idB, nil)
	roB := NewRouter(trB, rtB, stB)
	trB.SetHandler(roB)
	trB.Listen("127.0.0.1:0")
	defer trB.Stop()
	rtB.AddContact(idC, trC.Addr())

	// Node A (knows B)
	idA := NewNodeID("node-a")
	rtA := NewRoutingTable(idA)
	stA := NewInMemoryStorage()
	trA := NewTransport(idA, nil)
	roA := NewRouter(trA, rtA, stA)
	trA.SetHandler(roA)
	trA.Listen("127.0.0.1:0")
	defer trA.Stop()
	rtA.AddContact(idB, trB.Addr())

	// Node A performs lookup for Target ID (idD)
	found, err := roA.LookupNode(idD)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	// Verification: Node A should have discovered and successfully contacted Node D
	discoveredD := false
	for _, c := range found {
		if c.ID == idD {
			discoveredD = true
			break
		}
	}

	if !discoveredD {
		t.Errorf("Node A failed to discover Node D via B and C")
	}
}

func TestIterativeFindValue(t *testing.T) {
	// A -> B -> C (holds Value)
	key := NewNodeID("secret-key")
	value := []byte("the-secret-value")

	idC := NewNodeID("node-c")
	rtC := NewRoutingTable(idC)
	stC := NewInMemoryStorage()
	stC.Store(key, value, StateCommitted)
	trC := NewTransport(idC, nil)
	roC := NewRouter(trC, rtC, stC)
	trC.SetHandler(roC)
	trC.Listen("127.0.0.1:0")
	defer trC.Stop()

	idB := NewNodeID("node-b")
	rtB := NewRoutingTable(idB)
	stB := NewInMemoryStorage()
	trB := NewTransport(idB, nil)
	roB := NewRouter(trB, rtB, stB)
	trB.SetHandler(roB)
	trB.Listen("127.0.0.1:0")
	defer trB.Stop()

	idA := NewNodeID("node-a")
	rtA := NewRoutingTable(idA)
	stA := NewInMemoryStorage()
	trA := NewTransport(idA, nil)
	roA := NewRouter(trA, rtA, stA)
	trA.SetHandler(roA)
	trA.Listen("127.0.0.1:0")
	defer trA.Stop()

	rtA.AddContact(idB, trB.Addr())
	rtB.AddContact(idC, trC.Addr())

	// Node A searches for Value
	foundValue, found := roA.FindValue(key)
	if !found {
		t.Fatalf("Value not found")
	}

	if string(foundValue) != string(value) {
		t.Errorf("Value mismatch: got %s, want %s", string(foundValue), string(value))
	}
}
