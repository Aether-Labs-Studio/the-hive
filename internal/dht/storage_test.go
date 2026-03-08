package dht

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestInMemoryStorage(t *testing.T) {
	storage := NewInMemoryStorage()
	key := NewNodeID("test-key")
	data := []byte("test-data")

	// Test store and retrieve
	err := storage.Store(key, data, StateCommitted)
	if err != nil {
		t.Fatalf("Failed to store data: %v", err)
	}

	retrieved, ok := storage.Retrieve(key)
	if !ok {
		t.Fatalf("Failed to retrieve data")
	}

	if !bytes.Equal(retrieved, data) {
		t.Errorf("Retrieved data does not match original: %s vs %s", retrieved, data)
	}

	// Test retrieve non-existent key
	_, ok = storage.Retrieve(NewNodeID("non-existent"))
	if ok {
		t.Errorf("Should not have retrieved data for non-existent key")
	}
}

func TestDiskStorage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hive-storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storage, err := NewDiskStorage(tempDir, 0, nil)
	if err != nil {
		t.Fatalf("Failed to create DiskStorage: %v", err)
	}

	key := NewNodeID("persistent-key")
	data := []byte("persistent-data")

	// 1. Test basic store and retrieve
	if err := storage.Store(key, data, StateCommitted); err != nil {
		t.Fatalf("Failed to store data: %v", err)
	}

	// Verify file existence (using hex name)
	hexKey := fmt.Sprintf("%x.gz", key)
	filePath := filepath.Join(tempDir, hexKey)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Gzipped file was not created: %s", filePath)
	}

	retrieved, ok := storage.Retrieve(key)
	if !ok {
		t.Fatalf("Failed to retrieve data")
	}

	if !bytes.Equal(retrieved, data) {
		t.Errorf("Retrieved data does not match original: %s vs %s", retrieved, data)
	}

	// 2. Test persistence after "restart"
	// Create a new storage instance pointing to the same directory
	storageRestart, err := NewDiskStorage(tempDir, 0, nil)
	if err != nil {
		t.Fatalf("Failed to recreate DiskStorage: %v", err)
	}

	retrievedAfter, ok := storageRestart.Retrieve(key)
	if !ok {
		t.Fatalf("Failed to retrieve data after restart")
	}

	if !bytes.Equal(retrievedAfter, data) {
		t.Errorf("Data after restart does not match original: %s vs %s", retrievedAfter, data)
	}
}

func TestDiskStorageConcurrency(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hive-storage-concurrency-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storage, err := NewDiskStorage(tempDir, 0, nil)
	if err != nil {
		t.Fatalf("Failed to create DiskStorage: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 20
	numOperations := 20

	// Concurrent writers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := NewNodeID(fmt.Sprintf("key-%d-%d", id, j))
				data := []byte(fmt.Sprintf("data-%d-%d", id, j))
				storage.Store(key, data, StateCommitted)
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := NewNodeID(fmt.Sprintf("key-%d-%d", id, j))
				storage.Retrieve(key)
			}
		}(i)
	}

	wg.Wait()
}
