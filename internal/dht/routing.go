package dht

import (
	"math/bits"
	"net"
	"sort"
	"sync"
)

const K = 20
const Alpha = 3

// Contact represents a known node in the network.
type Contact struct {
	ID      NodeID
	Address net.Addr
}

// Bucket contains up to K contacts.
type Bucket struct {
	contacts []Contact
}

// RoutingTable manages K-buckets relative to a local NodeID.
type RoutingTable struct {
	localID NodeID
	buckets [160]*Bucket
	mu      sync.RWMutex
}

// NewRoutingTable creates a new routing table for the given local NodeID.
func NewRoutingTable(localID NodeID) *RoutingTable {
	rt := &RoutingTable{
		localID: localID,
	}
	for i := 0; i < 160; i++ {
		rt.buckets[i] = &Bucket{
			contacts: make([]Contact, 0, K),
		}
	}
	return rt
}

// getBucketIndex returns the index of the bucket (0-159) that would hold otherID.
func (rt *RoutingTable) getBucketIndex(otherID NodeID) int {
	dist := rt.localID.XOR(otherID)
	for i := 0; i < 20; i++ {
		if dist[i] != 0 {
			// Found the first byte that differs.
			// index = (byteIndexFromRight * 8) + bitIndexInByte
			return (19-i)*8 + bits.Len8(dist[i]) - 1
		}
	}
	return -1 // Same ID
}

// AddContact adds or updates a contact in the routing table.
// It returns true if the contact was newly added, false if it was an update.
func (rt *RoutingTable) AddContact(id NodeID, addr net.Addr) bool {
	if id == rt.localID {
		return false
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	idx := rt.getBucketIndex(id)
	if idx == -1 {
		return false
	}

	bucket := rt.buckets[idx]

	// If already in bucket, move to the end (most recently seen)
	for i, c := range bucket.contacts {
		if c.ID == id {
			bucket.contacts = append(bucket.contacts[:i], bucket.contacts[i+1:]...)
			bucket.contacts = append(bucket.contacts, Contact{ID: id, Address: addr})
			return false
		}
	}

	// If bucket is not full, add to the end
	if len(bucket.contacts) < K {
		bucket.contacts = append(bucket.contacts, Contact{ID: id, Address: addr})
		return true
	}
	return false
}

// FindClosestContacts returns up to 'count' contacts closest to the target NodeID.
func (rt *RoutingTable) FindClosestContacts(target NodeID, count int) []Contact {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	type distanceResult struct {
		contact Contact
		dist    NodeID
	}

	var results []distanceResult

	// Simple implementation: scan all buckets.
	for _, bucket := range rt.buckets {
		for _, c := range bucket.contacts {
			results = append(results, distanceResult{
				contact: c,
				dist:    target.XOR(c.ID),
			})
		}
	}

	// Sort by XOR distance using our NodeID.Less method.
	sort.Slice(results, func(i, j int) bool {
		return results[i].dist.Less(results[j].dist)
	})

	if len(results) > count {
		results = results[:count]
	}

	out := make([]Contact, len(results))
	for i, res := range results {
		out[i] = res.contact
	}
	return out
}

// GetAllContacts returns a list of all unique contacts currently in the routing table.
func (rt *RoutingTable) GetAllContacts() []Contact {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var contacts []Contact
	for _, bucket := range rt.buckets {
		contacts = append(contacts, bucket.contacts...)
	}
	return contacts
}
