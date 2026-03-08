package dht

import (
	"fmt"
	"net"
	"sync"
	"testing"
)

func TestAddContactCapacity(t *testing.T) {
	localID := NewNodeID("local-node")
	rt := NewRoutingTable(localID)
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}

	for i := 0; i < 1000; i++ {
		id := NewNodeID(fmt.Sprintf("node-%d", i))
		if id == localID {
			continue
		}
		rt.AddContact(id, addr)
	}

	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for i, bucket := range rt.buckets {
		if len(bucket.contacts) > 20 {
			t.Errorf("Bucket %d exceeds capacity K=20: got %d", i, len(bucket.contacts))
		}
	}
}

func TestRoutingTableConcurrency(t *testing.T) {
	localID := NewNodeID("local-node")
	rt := NewRoutingTable(localID)
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
	var wg sync.WaitGroup

	// concurrent writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				id := NewNodeID(fmt.Sprintf("writer-%d-%d", n, j))
				rt.AddContact(id, addr)
			}
		}(i)
	}

	// concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				target := NewNodeID(fmt.Sprintf("target-%d-%d", n, j))
				rt.FindClosestContacts(target, 20)
			}
		}(i)
	}

	wg.Wait()
}

func TestFindClosestContacts(t *testing.T) {
	localID := NewNodeID("local")
	rt := NewRoutingTable(localID)
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}

	// Add some known contacts
	ids := make([]NodeID, 100)
	for i := 0; i < 100; i++ {
		ids[i] = NewNodeID(fmt.Sprintf("contact-%d", i))
		rt.AddContact(ids[i], addr)
	}

	target := NewNodeID("target-query")
	closest := rt.FindClosestContacts(target, 10)

	if len(closest) != 10 {
		t.Errorf("Expected 10 closest contacts, got %d", len(closest))
	}

	// Verify they are actually the closest
	furthestInClosestDist := target.XOR(closest[9].ID)
	
	for _, id := range ids {
		dist := target.XOR(id)
		found := false
		for _, c := range closest {
			if c.ID == id {
				found = true
				break
			}
		}
		
		if !found && dist.Less(furthestInClosestDist) {
			t.Errorf("Found an ID %x closer than the furthest in closest set %x", id, closest[9].ID)
		}
	}
}
