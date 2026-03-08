package dht

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDiscovery(t *testing.T) {
	// Node A
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

	// Node B
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Unique multicast address for testing to avoid interference
	mcastAddr := "239.0.0.1:7445"

	// Start Discovery on A
	portA := trA.Addr().(*net.UDPAddr).Port
	discA := NewDiscovery(roA, portA, mcastAddr, true)
	discA.SetInterval(100 * time.Millisecond)
	discA.Start(ctx)

	// Start Discovery on B
	portB := trB.Addr().(*net.UDPAddr).Port
	discB := NewDiscovery(roB, portB, mcastAddr, true)
	discB.SetInterval(100 * time.Millisecond)
	discB.Start(ctx)

	// Wait for discovery
	deadline := time.Now().Add(10 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		contacts := rtA.FindClosestContacts(idB, 1)
		if len(contacts) > 0 && contacts[0].ID == idB {
			found = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !found {
		t.Errorf("Node A failed to discover Node B")
	}

	// Verify Node B also discovered Node A
	contactsB := rtB.FindClosestContacts(idA, 1)
	if len(contactsB) == 0 || contactsB[0].ID != idA {
		t.Errorf("Node B failed to discover Node A")
	}
}
