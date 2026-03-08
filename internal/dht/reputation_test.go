package dht

import (
	"path/filepath"
	"testing"
)

func TestReputationStore(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "reputation.json")

	rs, err := NewReputationStore(path)
	if err != nil {
		t.Fatalf("Failed to create ReputationStore: %v", err)
	}

	pubKey := []byte("test-public-key")
	
	// Default score is 0
	if s := rs.GetScore(pubKey); s != 0 {
		t.Errorf("Expected score 0, got %d", s)
	}

	// Add score
	rs.AddScore(pubKey, 1)
	if s := rs.GetScore(pubKey); s != 1 {
		t.Errorf("Expected score 1, got %d", s)
	}

	// Persistent check
	rs2, err := NewReputationStore(path)
	if err != nil {
		t.Fatalf("Failed to reload ReputationStore: %v", err)
	}
	if s := rs2.GetScore(pubKey); s != 1 {
		t.Errorf("Expected persistent score 1, got %d", s)
	}

	// Penalize
	rs2.AddScore(pubKey, -5)
	if s := rs2.GetScore(pubKey); s != -4 {
		t.Errorf("Expected score -4, got %d", s)
	}
}
