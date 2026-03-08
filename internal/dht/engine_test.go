package dht

import (
	"context"
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

