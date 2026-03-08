package dht

import (
	"sort"
	"sync"
)

type contactStatus int

const (
	statusNew contactStatus = iota
	statusQuerying
	statusResponded
	statusFailed
)

type shortlistEntry struct {
	contact Contact
	status  contactStatus
	dist    NodeID
}

// Shortlist manages a list of nodes discovered during an iterative search.
// It tracks their status and keeps them sorted by XOR distance to the target.
type Shortlist struct {
	target  NodeID
	entries []shortlistEntry
	mu      sync.Mutex
}

// NewShortlist creates a new Shortlist initialized with the given contacts.
func NewShortlist(target NodeID, initial []Contact) *Shortlist {
	s := &Shortlist{
		target: target,
	}
	for _, c := range initial {
		s.addContact(c)
	}
	s.sort()
	return s
}

func (s *Shortlist) addContact(c Contact) {
	// Check if already present
	for _, e := range s.entries {
		if e.contact.ID == c.ID {
			return
		}
	}
	s.entries = append(s.entries, shortlistEntry{
		contact: c,
		status:  statusNew,
		dist:    s.target.XOR(c.ID),
	})
}

func (s *Shortlist) sort() {
	sort.Slice(s.entries, func(i, j int) bool {
		return s.entries[i].dist.Less(s.entries[j].dist)
	})
}

// GetNextToQuery returns up to 'alpha' contacts that are in 'statusNew' state.
func (s *Shortlist) GetNextToQuery(alpha int) []Contact {
	s.mu.Lock()
	defer s.mu.Unlock()

	var next []Contact
	for i := 0; i < len(s.entries) && len(next) < alpha; i++ {
		if s.entries[i].status == statusNew {
			s.entries[i].status = statusQuerying
			next = append(next, s.entries[i].contact)
		}
	}
	return next
}

// MarkResponded marks a contact as having responded and adds newly discovered contacts.
func (s *Shortlist) MarkResponded(id NodeID, newContacts []Contact) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.entries {
		if s.entries[i].contact.ID == id {
			s.entries[i].status = statusResponded
			break
		}
	}

	for _, c := range newContacts {
		s.addContact(c)
	}
	s.sort()

	// Keep only the top K closest nodes to prevent unbounded growth
	if len(s.entries) > K {
		s.entries = s.entries[:K]
	}
}

// MarkFailed marks a contact as failed (unreachable).
func (s *Shortlist) MarkFailed(id NodeID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.entries {
		if s.entries[i].contact.ID == id {
			s.entries[i].status = statusFailed
			break
		}
	}
}

// IsFinished returns true if there are no more nodes to query or if we reached convergence.
// convergence means the closest K nodes have all responded (or failed).
func (s *Shortlist) IsFinished() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range s.entries {
		if e.status == statusNew || e.status == statusQuerying {
			return false
		}
	}
	return true
}

// GetClosest returns all contacts that have successfully responded, sorted by distance.
func (s *Shortlist) GetClosest() []Contact {
	s.mu.Lock()
	defer s.mu.Unlock()

	var closest []Contact
	for _, e := range s.entries {
		if e.status == statusResponded {
			closest = append(closest, e.contact)
		}
	}
	return closest
}

// ClosestUnqueried returns the distance of the closest node that hasn't responded yet.
func (s *Shortlist) HasNewerCloserNode() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find distance of closest responded node
	var minResponded *NodeID
	for _, e := range s.entries {
		if e.status == statusResponded {
			minResponded = &e.dist
			break
		}
	}

	// Check if there's any new node closer than the minResponded
	for _, e := range s.entries {
		if e.status == statusNew {
			if minResponded == nil || e.dist.Less(*minResponded) {
				return true
			}
		}
	}
	return false
}
