package dht

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockRepProvider struct {
	reps map[string]int
}

func (m *mockRepProvider) GetReputation(pubKey []byte) int {
	return m.reps[string(pubKey)]
}

func TestStorageGC(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create storage with small TTL
	storage, _ := NewDiskStorage(tempDir, 0, nil)
	
	key := NewNodeID("expired-item")
	// Manual store with expired timestamp
	expiresAt := time.Now().Add(-1 * time.Hour).Unix()
	
	meta := &StorageMetadata{
		Key:       key,
		Size:      10,
		ExpiresAt: expiresAt,
	}
	storage.metadata[key] = meta
	
	// Create dummy file with correct hex name
	filename := filepath.Join(tempDir, hex.EncodeToString(key[:])+".gz")
	os.WriteFile(filename, []byte("data"), 0644)
	
	deleted := storage.CleanExpired()
	if deleted != 1 {
		t.Errorf("Expected 1 deleted item, got %d", deleted)
	}
	
	if _, found := storage.Retrieve(key); found {
		t.Error("Item should have been deleted")
	}
}

func TestStorageEviction(t *testing.T) {
	tempDir := t.TempDir()
	rep := &mockRepProvider{reps: make(map[string]int)}
	
	// Quota: 100 bytes
	storage, _ := NewDiskStorage(tempDir, 100, rep)
	
	authorGood := []byte("good")
	authorBad := []byte("bad")
	rep.reps[string(authorGood)] = 10
	rep.reps[string(authorBad)] = -5
	
	// 1. Store data from good author (60 bytes)
	key1 := NewNodeID("good-data")
	data1 := make([]byte, 60)
	// Add dummy file to disk so Store/Retrieve works
	storage.Store(key1, data1) 
	// Override metadata to match our test case exactly
	storage.mu.Lock()
	storage.metadata[key1].AuthorPub = authorGood
	storage.metadata[key1].Size = 60
	storage.currentSize = 60
	storage.mu.Unlock()
	
	// 2. Store data from bad author (30 bytes)
	key2 := NewNodeID("bad-data")
	data2 := make([]byte, 30)
	storage.Store(key2, data2)
	storage.mu.Lock()
	storage.metadata[key2].AuthorPub = authorBad
	storage.metadata[key2].Size = 30
	storage.currentSize = 90
	storage.mu.Unlock()
	
	// 3. Store new data (20 bytes) -> Total 110 > 100 -> Must evict
	// The bad author should be evicted first.
	newData := make([]byte, 20)
	storage.Store(NewNodeID("new"), newData)
	
	if _, found := storage.metadata[key2]; found {
		t.Error("Bad author's data should have been evicted first")
	}
	if _, found := storage.metadata[key1]; !found {
		t.Error("Good author's data should have been kept")
	}
}
