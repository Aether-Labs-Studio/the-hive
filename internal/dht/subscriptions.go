package dht

import (
	"sync"
)

// SubscriptionManager handles local interests in specific keywords.
type SubscriptionManager struct {
	interests map[string]bool
	mu        sync.RWMutex
}

// NewSubscriptionManager creates a new SubscriptionManager.
func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{
		interests: make(map[string]bool),
	}
}

// Subscribe registers interest in a keyword.
func (s *SubscriptionManager) Subscribe(keyword string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interests[keyword] = true
}

// Unsubscribe removes interest in a keyword.
func (s *SubscriptionManager) Unsubscribe(keyword string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.interests, keyword)
}

// IsSubscribed returns true if the node is interested in the given keyword.
func (s *SubscriptionManager) IsSubscribed(keyword string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.interests[keyword]
}

// SubscriptionStore tracks who is interested in topics this node is responsible for.
type SubscriptionStore struct {
	// topicID -> list of subscriber contacts
	subscribers map[NodeID][]Contact
	mu          sync.RWMutex
}

// NewSubscriptionStore creates a new SubscriptionStore.
func NewSubscriptionStore() *SubscriptionStore {
	return &SubscriptionStore{
		subscribers: make(map[NodeID][]Contact),
	}
}

func (s *SubscriptionStore) AddSubscriber(topicID NodeID, c Contact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	list := s.subscribers[topicID]
	for _, existing := range list {
		if existing.ID == c.ID { return }
	}
	s.subscribers[topicID] = append(list, c)
}

// GetAllSubscriptions returns all currently active interests.
func (s *SubscriptionManager) GetAllSubscriptions() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var res []string
	for k := range s.interests {
		res = append(res, k)
	}
	return res
}
