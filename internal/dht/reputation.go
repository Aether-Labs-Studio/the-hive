package dht

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ReputationStore manages local scores for authors based on their public keys.
type ReputationStore struct {
	path   string
	scores map[string]int
	mu     sync.RWMutex
}

// NewReputationStore creates or loads a ReputationStore from the given path.
func NewReputationStore(path string) (*ReputationStore, error) {
	rs := &ReputationStore{
		path:   path,
		scores: make(map[string]int),
	}

	if err := rs.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load reputation store: %w", err)
	}

	return rs, nil
}

// AddScore updates the author's reputation score by adding the delta.
func (rs *ReputationStore) AddScore(pubKey []byte, delta int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	key := hex.EncodeToString(pubKey)
	rs.scores[key] += delta
	_ = rs.save()
}

// GetReputation returns the current reputation score for the given author.
func (rs *ReputationStore) GetReputation(pubKey []byte) int {
	return rs.GetScore(pubKey)
}

// GetScore returns the current reputation score for the given author.
func (rs *ReputationStore) GetScore(pubKey []byte) int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	key := hex.EncodeToString(pubKey)
	return rs.scores[key]
}

func (rs *ReputationStore) save() error {
	data, err := json.MarshalIndent(rs.scores, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(rs.path, data, 0600)
}

func (rs *ReputationStore) load() error {
	data, err := os.ReadFile(rs.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &rs.scores)
}
