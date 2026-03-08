package dht

import (
	"net"
	"testing"
)

func TestShortlist(t *testing.T) {
	target := NewNodeID("target")
	
	// Create some contacts with known distances
	c1 := Contact{ID: NewNodeID("c1"), Address: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1}}
	c2 := Contact{ID: NewNodeID("c2"), Address: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2}}
	c3 := Contact{ID: NewNodeID("c3"), Address: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 3}}
	c4 := Contact{ID: NewNodeID("c4"), Address: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4}}

	sl := NewShortlist(target, []Contact{c1, c2, c3})

	// 1. Get next to query (Alpha = 2)
	next := sl.GetNextToQuery(2)
	if len(next) != 2 {
		t.Errorf("Expected 2 nodes to query, got %d", len(next))
	}

	// 2. Mark one as responded with new contacts
	sl.MarkResponded(next[0].ID, []Contact{c4})

	// 3. Mark one as failed
	sl.MarkFailed(next[1].ID)

	// 4. Verify status
	if sl.IsFinished() {
		t.Errorf("Shortlist should not be finished yet")
	}

	// 5. Get remaining nodes
	next2 := sl.GetNextToQuery(2)
	if len(next2) != 2 { // One from initial (c3) and one new (c4)
		t.Errorf("Expected 2 nodes to query in second round, got %d", len(next2))
	}

	sl.MarkResponded(next2[0].ID, nil)
	sl.MarkResponded(next2[1].ID, nil)

	if !sl.IsFinished() {
		t.Errorf("Shortlist should be finished")
	}

	closest := sl.GetClosest()
	if len(closest) != 3 {
		t.Errorf("Expected 3 successful contacts, got %d", len(closest))
	}
}

func TestShortlistOrdering(t *testing.T) {
	target := NodeID{0}
	
	// id[0] determines distance to target{0}
	c1 := Contact{ID: NodeID{10}}
	c2 := Contact{ID: NodeID{5}}
	c3 := Contact{ID: NodeID{20}}

	sl := NewShortlist(target, []Contact{c1, c2, c3})
	
	next := sl.GetNextToQuery(1)
	if next[0].ID != c2.ID {
		t.Errorf("Expected closest node (5) first, got %v", next[0].ID[0])
	}
}
