package dht

import (
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
	"the-hive/internal/logger"
)

// Storage defines the interface for storing and retrieving data in the DHT.
type Storage interface {
	Store(key NodeID, data []byte, state ChunkState) error
	Retrieve(key NodeID) ([]byte, bool)
	GetMetadata(key NodeID) (*StorageMetadata, bool)
	GetAllKeys() []NodeID
	CleanExpired() int
}

// ChunkState represents the GitOps-inspired lifecycle of a knowledge chunk.
type ChunkState string

const (
	StateModified  ChunkState = "modified"  // Work-in-progress, local only, short TTL
	StateStaged    ChunkState = "staged"    // Validated locally, ready to be committed
	StateCommitted ChunkState = "committed" // Signed, immutable, searchable, and broadcastable
	StateIndex     ChunkState = "index"     // Mutable, broadcastable, searchable (used for keyword indices)
)

// StorageMetadata tracks TTL and LRU info for a stored chunk.
type StorageMetadata struct {
	Key        NodeID     `json:"key"`
	Size       int64      `json:"size"`
	ExpiresAt  int64      `json:"expires_at"`
	AccessedAt int64      `json:"accessed_at"`
	AuthorPub  []byte     `json:"author_pub,omitempty"`
	State      ChunkState `json:"state"`                // Added for Phase 1.1.0
	ParentID   string     `json:"parent_id,omitempty"`   // Added for Phase 1.1.0 DAG support
}

// ReputationProvider allows storage to query author trust during eviction.
type ReputationProvider interface {
	GetReputation(pubKey []byte) int
}

// DiskStorage implements the Storage interface by saving chunks as gzipped files.
type DiskStorage struct {
	BasePath   string
	MaxStorage int64
	metaPath   string
	metadata   map[NodeID]*StorageMetadata
	currentSize int64
	rep        ReputationProvider
	mu         sync.RWMutex
}

// NewDiskStorage creates a new DiskStorage instance with a quota and reputation source.
func NewDiskStorage(basePath string, maxStorage int64, rep ReputationProvider) (*DiskStorage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	s := &DiskStorage{
		BasePath:   basePath,
		MaxStorage: maxStorage,
		metaPath:   filepath.Join(basePath, "metadata.json"),
		metadata:   make(map[NodeID]*StorageMetadata),
		rep:        rep,
	}

	if err := s.loadMetadata(); err != nil && !os.IsNotExist(err) {
		logger.Error("Storage: Error al cargar metadatos: %v", err)
	}

	return s, nil
}

// Store saves data and updates metadata, enforcing quotas and immutability.
func (s *DiskStorage) Store(key NodeID, data []byte, state ChunkState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Enforce Immutability for Committed chunks
	if old, exists := s.metadata[key]; exists {
		if old.State == StateCommitted {
			return fmt.Errorf("immutable error: cannot modify committed chunk %x", key)
		}
		s.currentSize -= old.Size
	}

	// 2. Extract basic envelope info if possible
	if state == "" {
		state = StateModified // Default
	}
	
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	if state == StateModified {
		expiresAt = time.Now().Add(1 * time.Hour).Unix() // Modified chunks expire fast
	}
	
	var authorPub []byte
	var parentID string
	
	var envelope struct {
		PublicKey []byte `json:"pub_key"`
		ExpiresAt int64  `json:"expires_at"`
		ParentID  string `json:"parent_id"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil {
		if envelope.ExpiresAt > 0 {
			expiresAt = envelope.ExpiresAt
		}
		authorPub = envelope.PublicKey
		parentID = envelope.ParentID
	}

	itemSize := int64(len(data))
	
	// Enforce quota
	if s.MaxStorage > 0 && s.currentSize+itemSize > s.MaxStorage {
		s.evict(itemSize)
	}

	// Save to disk
	filename := filepath.Join(s.BasePath, hex.EncodeToString(key[:])+".gz")
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	if _, err := gzWriter.Write(data); err != nil {
		gzWriter.Close()
		return err
	}
	gzWriter.Close()

	// Update metadata
	s.metadata[key] = &StorageMetadata{
		Key:        key,
		Size:       itemSize,
		ExpiresAt:  expiresAt,
		AccessedAt: time.Now().Unix(),
		AuthorPub:  authorPub,
		State:      state,
		ParentID:   parentID,
	}
	s.currentSize += itemSize
	_ = s.saveMetadata()

	return nil
}

// Retrieve returns data and updates LRU timestamp.
func (s *DiskStorage) Retrieve(key NodeID) ([]byte, bool) {
	s.mu.Lock()
	meta, exists := s.metadata[key]
	if exists {
		meta.AccessedAt = time.Now().Unix()
		// Non-blocking save
		go func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			_ = s.saveMetadata()
		}()
	}
	s.mu.Unlock()

	s.mu.RLock()
	defer s.mu.RUnlock()

	filename := filepath.Join(s.BasePath, hex.EncodeToString(key[:])+".gz")
	file, err := os.Open(filename)
	if err != nil {
		return nil, false
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, false
	}
	defer gzReader.Close()

	data, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, false
	}

	return data, true
}

func (s *DiskStorage) GetMetadata(key NodeID) (*StorageMetadata, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	meta, ok := s.metadata[key]
	return meta, ok
}

func (s *DiskStorage) GetAllKeys() []NodeID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]NodeID, 0, len(s.metadata))
	for k := range s.metadata {
		keys = append(keys, k)
	}
	return keys
}

// CleanExpired removes items that have passed their TTL.
func (s *DiskStorage) CleanExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	count := 0
	for key, meta := range s.metadata {
		if meta.ExpiresAt > 0 && meta.ExpiresAt < now {
			s.deleteKey(key)
			count++
		}
	}
	if count > 0 {
		_ = s.saveMetadata()
	}
	return count
}

func (s *DiskStorage) evict(needed int64) {
	// 1. Prepare candidates
	type evictCandidate struct {
		key        NodeID
		reputation int
		accessed   int64
		size       int64
	}
	
	var candidates []evictCandidate
	for k, m := range s.metadata {
		rep := 0
		if s.rep != nil && m.AuthorPub != nil {
			rep = s.rep.GetReputation(m.AuthorPub)
		}
		candidates = append(candidates, evictCandidate{
			key:        k,
			reputation: rep,
			accessed:   m.AccessedAt,
			size:       m.Size,
		})
	}

	// 2. Sort: Negative reputation first, then LRU
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].reputation != candidates[j].reputation {
			return candidates[i].reputation < candidates[j].reputation
		}
		return candidates[i].accessed < candidates[j].accessed
	})

	// 3. Delete until space is enough
	for _, c := range candidates {
		s.deleteKey(c.key)
		if s.currentSize+needed <= s.MaxStorage {
			break
		}
	}
}

func (s *DiskStorage) deleteKey(key NodeID) {
	meta, exists := s.metadata[key]
	if !exists {
		return
	}

	filename := filepath.Join(s.BasePath, hex.EncodeToString(key[:])+".gz")
	_ = os.Remove(filename)
	
	s.currentSize -= meta.Size
	delete(s.metadata, key)
	
	// Manifest Cascade Logic
	// In a real implementation we would peek into the data to see if it's a manifest
	// and trigger segment deletion. For now, we rely on segment TTL or future sweeps.
}

func (s *DiskStorage) loadMetadata() error {
	data, err := os.ReadFile(s.metaPath)
	if err != nil {
		return err
	}
	var list []*StorageMetadata
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	
	s.currentSize = 0
	for _, m := range list {
		s.metadata[m.Key] = m
		s.currentSize += m.Size
	}
	return nil
}

func (s *DiskStorage) saveMetadata() error {
	list := make([]*StorageMetadata, 0, len(s.metadata))
	for _, m := range s.metadata {
		list = append(list, m)
	}
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return os.WriteFile(s.metaPath, data, 0600)
}

// InMemoryStorage remains simple for testing
type InMemoryStorage struct {
	data map[NodeID][]byte
	meta map[NodeID]*StorageMetadata
	mu   sync.RWMutex
}

func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		data: make(map[NodeID][]byte),
		meta: make(map[NodeID]*StorageMetadata),
	}
}

func (s *InMemoryStorage) Store(key NodeID, data []byte, state ChunkState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if old, exists := s.meta[key]; exists && old.State == StateCommitted {
		return fmt.Errorf("immutable error")
	}

	s.data[key] = data
	s.meta[key] = &StorageMetadata{Key: key, State: state}
	return nil
}

func (s *InMemoryStorage) Retrieve(key NodeID) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.data[key]
	return d, ok
}

func (s *InMemoryStorage) GetMetadata(key NodeID) (*StorageMetadata, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.meta[key]
	return m, ok
}

func (s *InMemoryStorage) GetAllKeys() []NodeID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]NodeID, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

func (s *InMemoryStorage) CleanExpired() int { return 0 }
