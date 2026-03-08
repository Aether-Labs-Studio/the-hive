package dht

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"the-hive/internal/logger"
)

// Sanitizer defines the interface for data validation and signing.
type Sanitizer interface {
	ExtractAndInspectSecure(chunk []byte, expectedID NodeID) ([]byte, ed25519.PublicKey, string, error)
	Sanitize(raw []byte) []byte
	Sign(data []byte, parentID string) ([]byte, error)
	PackageChunk(sanitized []byte, parentID string) ([]byte, NodeID, error)
	IsTopicBlocked(topic string) bool
}

// Manifest and Index Constants
const (
	ManifestKind   = "hive_manifest_v1"
	IndexKind      = "hive_index_v1"
	ChunkThreshold = 32768 // 32KB
	MinReputation  = -3    // Authors below this are filtered out
)

// Manifest represents a large data structure split into multiple chunks.
type Manifest struct {
	Kind     string   `json:"kind"`
	Chunks   []string `json:"chunks"`
	ParentID string   `json:"parent_id,omitempty"` // Added for Phase 16 traceability
}

// IndexChunk represents a list of pointers to content chunks for a specific keyword.
type IndexChunk struct {
	Kind     string   `json:"kind"`
	Pointers []string `json:"pointers"`
}

// NetworkState represents the current operational mode of the DHT.
type NetworkState string

const (
	StateOffline NetworkState = "offline"
	StateLocal   NetworkState = "local"
	StateOnline  NetworkState = "online"
)

// Engine orchestrates the DHT's high-level P2P operations.
type Engine struct {
	router *Router
	
	// State management
	state      NetworkState
	stateMu    sync.Mutex
	cancelFunc context.CancelFunc
	mainCtx    context.Context

	// Hot Keywords tracking
	hotKeywords   map[string]int
	hotKeywordsMu sync.Mutex

	// Reputation
	repStore *ReputationStore

	// Subscriptions
	subMgr *SubscriptionManager

	// Sanitizer reference
	sanitizer Sanitizer
}

// NewEngine creates a new Engine instance.
func NewEngine(router *Router, rep *ReputationStore) *Engine {
	return &Engine{
		router:      router,
		state:       StateOnline,
		hotKeywords: make(map[string]int),
		repStore:    rep,
		subMgr:      NewSubscriptionManager(),
	}
}

// SetSanitizer allows injecting the sanitizer into the engine.
func (e *Engine) SetSanitizer(s Sanitizer) {
	e.sanitizer = s
}

func (e *Engine) GetReputation(pubKey []byte) int {
	if e.repStore == nil { return 0 }
	return e.repStore.GetScore(pubKey)
}

func (e *Engine) RateAuthor(pubKey []byte, delta int) {
	if e.repStore != nil { e.repStore.AddScore(pubKey, delta) }
}

func (e *Engine) FindValue(key NodeID) ([]byte, bool) {
	return e.router.FindValue(key)
}

func (e *Engine) StoreValue(key NodeID, data []byte) error {
	return e.router.StoreValue(key, data)
}

func (e *Engine) Subscribe(keyword string) {
	e.subMgr.Subscribe(keyword)

	// Active Subscription: Find k-closest nodes to the keyword hash and send SUBSCRIBE RPC
	kwID := NewNodeID(keyword)
	go func() {
		closest, err := e.router.LookupNode(kwID)
		if err != nil { return }

		payload, _ := json.Marshal(SubscribePayload{TopicID: kwID})
		for _, c := range closest {
			msg := Message{
				Type: Subscribe, TransactionID: fmt.Sprintf("sub-%x", kwID[:4]),
				SenderID: e.router.sender.LocalID(), Payload: payload,
			}
			_, _ = e.router.sender.Request(c.Address, msg, 1*time.Second)
		}
	}()
}

func (e *Engine) Unsubscribe(keyword string) { e.subMgr.Unsubscribe(keyword) }
func (e *Engine) IsSubscribed(keyword string) bool { return e.subMgr.IsSubscribed(keyword) }
func (e *Engine) GetAllSubscriptions() []string { return e.subMgr.GetAllSubscriptions() }
func (e *Engine) GetSubscriptionManager() *SubscriptionManager { return e.subMgr }

// SearchResult represents a single piece of knowledge found in the swarm.
type SearchResult struct {
	Content    string `json:"content"`
	AuthorID   string `json:"author_id,omitempty"`
	Reputation int    `json:"reputation"`
	ParentID   string `json:"parent_id,omitempty"`
}

// --- Knowledge Logic (Search / Share / Rate) ---

func (e *Engine) Search(query string) ([]SearchResult, error) {
	queryTerms := ExtractKeywords(query)
	if len(queryTerms) == 0 {
		return e.performExactSearch(query)
	}
	if len(queryTerms) == 1 {
		return e.performExactSearch(queryTerms[0])
	}
	return e.performMultiTermSearch(queryTerms)
}

func (e *Engine) performExactSearch(query string) ([]SearchResult, error) {
	targetID := NewNodeID(query)
	logger.Info("Engine: Search started for '%s' (ID: %x)", query, targetID)

	data, found := e.router.FindValue(targetID)
	if !found {
		return nil, nil
	}

	// 1. Verify and extract (handles signatures and decompression)
	payload, pub, parentID, err := e.sanitizer.ExtractAndInspectSecure(data, targetID)
	if err != nil {
		// Try again without hash verification if it's potentially an index/manifest
		payload, pub, parentID, err = e.sanitizer.ExtractAndInspectSecure(data, NodeID{})
		if err != nil {
			return nil, fmt.Errorf("🚨 Error de seguridad: %v", err)
		}
	}

	// 2. Check Reputation
	if pub != nil {
		rep := e.GetReputation(pub)
		if rep < MinReputation {
			return nil, fmt.Errorf("Contenido bloqueado: baja reputación del autor.")
		}
	}

	// 3. Resolve by Kind
	if m, ok := isManifest(payload); ok {
		return e.resolveManifest(targetID, m)
	}
	if idx, ok := isIndexChunk(payload); ok {
		return e.resolveIndex(targetID, idx)
	}

	// 4. Regular data chunk
	return e.formatDataResponse(targetID, payload, pub, parentID)
}

func (e *Engine) performMultiTermSearch(terms []string) ([]SearchResult, error) {
	logger.Info("Engine: Multi-term search (AND) for: %v", terms)

	var wg sync.WaitGroup
	var mu sync.Mutex
	termResults := make(map[string]map[string]struct{})

	for _, term := range terms {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			kwID := NewNodeID(t)
			data, found := e.router.FindValue(kwID)
			if !found { return }

			// Extract index
			payload, _, _, err := e.sanitizer.ExtractAndInspectSecure(data, NodeID{})
			if err != nil { return }

			if idx, ok := isIndexChunk(payload); ok {
				set := make(map[string]struct{})
				for _, p := range idx.Pointers { set[p] = struct{}{} }
				mu.Lock()
				termResults[t] = set
				mu.Unlock()
			}
		}(term)
	}
	wg.Wait()

	if len(termResults) < len(terms) {
		return nil, fmt.Errorf("No se encontraron coincidencias que cumplan con todos los términos.")
	}

	// Intersection
	var intersection []string
	firstTerm := terms[0]
	for ptr := range termResults[firstTerm] {
		inAll := true
		for i := 1; i < len(terms); i++ {
			if _, exists := termResults[terms[i]][ptr]; !exists {
				inAll = false
				break
			}
		}
		if inAll { intersection = append(intersection, ptr) }
	}

	if len(intersection) == 0 {
		return nil, fmt.Errorf("No se encontraron memorias que contengan todos los términos.")
	}

	dummyIdx := &IndexChunk{Kind: IndexKind, Pointers: intersection}
	return e.resolveIndex(NewNodeID(strings.Join(terms, " ")), dummyIdx)
}

func (e *Engine) Share(topic, content, parentID string) (string, error) {
	logger.Info("Engine: Sharing topic: %s (Parent: %s)", topic, parentID)

	if e.sanitizer.IsTopicBlocked(topic) {
		return "", fmt.Errorf("security policy: topic blocked")
	}

	sanitized := e.sanitizer.Sanitize([]byte(content))

	var finalID NodeID
	if len(sanitized) > ChunkThreshold {
		id, err := e.shareLargeContent(sanitized, parentID)
		if err != nil { return "", err }
		finalID = id
	} else {
		chunk, id, err := e.sanitizer.PackageChunk(sanitized, parentID)
		if err != nil { return "", err }
		_ = e.router.StoreValue(id, chunk)
		finalID = id
	}

	// Indexing
	go e.indexContent(topic, string(sanitized), finalID)

	return fmt.Sprintf("Memoria compartida e indexada. ID: %x", finalID), nil
}

func (e *Engine) Rate(chunkIDHex string, score int) (string, error) {
	decoded, err := hex.DecodeString(chunkIDHex)
	if err != nil || len(decoded) != 20 {
		return "", fmt.Errorf("invalid chunk_id format")
	}
	var targetID NodeID
	copy(targetID[:], decoded)

	data, found := e.router.FindValue(targetID)
	if !found {
		return "", fmt.Errorf("chunk not found in network")
	}

	_, pub, _, err := e.sanitizer.ExtractAndInspectSecure(data, NodeID{})
	if err != nil {
		return "", fmt.Errorf("verification failed: %v", err)
	}
	if pub == nil {
		return "", fmt.Errorf("cannot rate unsigned data")
	}

	e.RateAuthor(pub, score)
	newScore := e.GetReputation(pub)
	authorHash := NewNodeID(string(pub))

	return fmt.Sprintf("Calificación procesada. Autor: %x... | Reputación local: %d", authorHash[:4], newScore), nil
}

// --- Internal Helpers ---

func (e *Engine) shareLargeContent(data []byte, parentID string) (NodeID, error) {
	segments := Split(data, ChunkThreshold)
	chunkIDs := make([]string, len(segments))

	for i, seg := range segments {
		// Segments are just data, we don't link them to parents individually
		chunk, id, err := e.sanitizer.PackageChunk(seg, "")
		if err != nil { return NodeID{}, err }
		_ = e.router.StoreValue(id, chunk)
		chunkIDs[i] = hex.EncodeToString(id[:])
	}

	manifest := Manifest{
		Kind:     ManifestKind,
		Chunks:   chunkIDs,
		ParentID: parentID,
	}
	manifestRaw, _ := json.Marshal(manifest)
	manifestData, _ := e.sanitizer.Sign(manifestRaw, parentID)
	manifestID := NewNodeID(string(manifestRaw))
	_ = e.router.StoreValue(manifestID, manifestData)

	return manifestID, nil
}

func (e *Engine) indexContent(topic, content string, chunkID NodeID) {
	topicKeywords := ExtractKeywords(topic)
	contentKeywords := ExtractTopKeywords(content, 10)
	
	keywordMap := make(map[string]struct{})
	for _, k := range topicKeywords { keywordMap[k] = struct{}{} }
	for _, k := range contentKeywords { keywordMap[k] = struct{}{} }

	chunkIDHex := hex.EncodeToString(chunkID[:])

	for kw := range keywordMap {
		kwID := NewNodeID(kw)
		var index IndexChunk
		data, found := e.router.FindValue(kwID)
		if found {
			payload, _, _, err := e.sanitizer.ExtractAndInspectSecure(data, NodeID{})
			if err == nil {
				if err := json.Unmarshal(payload, &index); err != nil || index.Kind != IndexKind {
					index = IndexChunk{Kind: IndexKind, Pointers: []string{}}
				}
			} else {
				index = IndexChunk{Kind: IndexKind, Pointers: []string{}}
			}
		} else {
			index = IndexChunk{Kind: IndexKind, Pointers: []string{}}
		}

		exists := false
		for _, p := range index.Pointers {
			if p == chunkIDHex { exists = true; break }
		}

		if !exists {
			index.Pointers = append(index.Pointers, chunkIDHex)
			if len(index.Pointers) > 50 { index.Pointers = index.Pointers[1:] }
			
			rawJSON, _ := json.Marshal(index)
			newData, _ := e.sanitizer.Sign(rawJSON, "")
			_ = e.router.StoreValue(kwID, newData)
			GlobalTelemetry.Emit(EventType("KeywordSeen"), "", kw)
			
			// Active Publish: Notify k-closest nodes about the update
			go func(topic string, cid NodeID) {
				closest, err := e.router.LookupNode(kwID)
				if err != nil { return }
				
				payload, _ := json.Marshal(PublishPayload{Keyword: topic, ChunkID: cid})
				for _, c := range closest {
					msg := Message{
						Type: Publish, TransactionID: fmt.Sprintf("pub-%x", kwID[:4]),
						SenderID: e.router.sender.LocalID(), Payload: payload,
					}
					_, _ = e.router.sender.Request(c.Address, msg, 1*time.Second)
				}
			}(kw, chunkID)
		}
	}
}

func (e *Engine) resolveManifest(manifestID NodeID, m *Manifest) ([]SearchResult, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	segments := make([][]byte, len(m.Chunks))
	var authors []string
	errorsFound := false
	filteredCount := 0

	for i, chunkHex := range m.Chunks {
		wg.Add(1)
		go func(idx int, hexID string) {
			defer wg.Done()
			decoded, _ := hex.DecodeString(hexID)
			var id NodeID
			copy(id[:], decoded)

			chunk, found := e.router.FindValue(id)
			if !found { mu.Lock(); errorsFound = true; mu.Unlock(); return }

			payload, pub, _, err := e.sanitizer.ExtractAndInspectSecure(chunk, id)
			if err != nil { mu.Lock(); errorsFound = true; mu.Unlock(); return }

			if pub != nil {
				rep := e.GetReputation(pub)
				if rep < MinReputation { mu.Lock(); filteredCount++; mu.Unlock(); return }
				authorHash := NewNodeID(string(pub))
				mu.Lock(); authors = append(authors, fmt.Sprintf("%x", authorHash)[:8]); mu.Unlock()
			}
			mu.Lock(); segments[idx] = payload; mu.Unlock()
		}(i, chunkHex)
	}
	wg.Wait()

	if filteredCount > 0 { return nil, fmt.Errorf("Contenido bloqueado por baja reputación del autor.") }
	if errorsFound { return nil, fmt.Errorf("Error al recuperar segmentos del manifiesto.") }

	fullPayload := Join(segments)
	uniqueAuthors := make(map[string]struct{})
	for _, a := range authors { uniqueAuthors[a] = struct{}{} }
	authorList := strings.Join(getMapKeys(uniqueAuthors), ", ")

	return []SearchResult{{
		Content:  string(fullPayload),
		AuthorID: authorList,
		ParentID: m.ParentID,
	}}, nil
}

func (e *Engine) resolveIndex(indexID NodeID, idx *IndexChunk) ([]SearchResult, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []SearchResult

	for _, ptrHex := range idx.Pointers {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			decoded, _ := hex.DecodeString(p)
			var ptrID NodeID
			copy(ptrID[:], decoded)

			chunk, found := e.router.FindValue(ptrID)
			if !found { return }

			payload, pub, parentID, err := e.sanitizer.ExtractAndInspectSecure(chunk, ptrID)
			if err != nil { return }

			rep := 0
			authorStr := "Desconocido"
			if pub != nil {
				rep = e.GetReputation(pub)
				if rep < MinReputation { return }
				authorHash := NewNodeID(string(pub))
				authorStr = fmt.Sprintf("%x", authorHash)[:8]
			}

			mu.Lock()
			results = append(results, SearchResult{
				Content:    string(payload),
				AuthorID:   authorStr,
				Reputation: rep,
				ParentID:   parentID,
			})
			mu.Unlock()
		}(ptrHex)
	}
	wg.Wait()

	if len(results) == 0 { return nil, fmt.Errorf("No hay resultados de autores confiables.") }
	return results, nil
}

func (e *Engine) formatDataResponse(targetID NodeID, payload []byte, pub ed25519.PublicKey, parentID string) ([]SearchResult, error) {
	rep := 0
	authorStr := "Desconocido"
	if pub != nil {
		rep = e.GetReputation(pub)
		authorHash := NewNodeID(string(pub))
		authorStr = fmt.Sprintf("%x", authorHash)[:8]
	}

	return []SearchResult{{
		Content:    string(payload),
		AuthorID:   authorStr,
		Reputation: rep,
		ParentID:   parentID,
	}}, nil
}

// --- Infrastructure Management ---

func (e *Engine) TrackKeyword(kw string) {
	e.hotKeywordsMu.Lock()
	defer e.hotKeywordsMu.Unlock()
	e.hotKeywords[kw]++
}

func (e *Engine) GetHotKeywords(limit int) []string {
	e.hotKeywordsMu.Lock()
	defer e.hotKeywordsMu.Unlock()
	var list []struct { k string; c int }
	for k, v := range e.hotKeywords { list = append(list, struct{k string; c int}{k, v}) }
	sort.Slice(list, func(i, j int) bool { return list[i].c > list[j].c })
	if len(list) > limit { list = list[:limit] }
	res := make([]string, len(list))
	for i, item := range list { res[i] = item.k }
	return res
}

func (e *Engine) emitTopology() {
	contacts := e.router.rt.GetAllContacts()
	neighbors := make([]string, len(contacts))
	for i, c := range contacts { neighbors[i] = fmt.Sprintf("%x", c.ID) }
	keys := e.router.storage.GetAllKeys()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	payload := struct {
		Neighbors   []string `json:"neighbors"`
		HotKeywords []string `json:"hot_keywords"`
		Stats       struct {
			LocalKeys   int    `json:"local_keys"`
			MemoryBytes uint64 `json:"memory_bytes"`
		} `json:"stats"`
	}{
		Neighbors:   neighbors,
		HotKeywords: e.GetHotKeywords(10),
		Stats: struct {
			LocalKeys   int    `json:"local_keys"`
			MemoryBytes uint64 `json:"memory_bytes"`
		}{LocalKeys: len(keys), MemoryBytes: m.Alloc},
	}
	localID := fmt.Sprintf("%x", e.router.sender.LocalID())
	GlobalTelemetry.EmitWithPayload(TopologySync, localID, "Periodic topology snapshot", payload)
}

func (e *Engine) SetSwarmContext(ctx context.Context) { e.mainCtx = ctx }

func (e *Engine) SetState(newState NetworkState, bootstrapAddr string) error {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	if e.state == newState { return nil }
	logger.Info("Swarm: Cambiando estado de red de %s a %s", e.state, newState)
	if e.cancelFunc != nil { e.cancelFunc() }
	if newState == StateOffline { _ = e.router.sender.(*Transport).Stop() }
	e.state = newState
	if newState == StateOffline { return nil }
	ctx, cancel := context.WithCancel(e.mainCtx)
	e.cancelFunc = cancel
	e.StartWorkers(ctx, DefaultWorkerOptions)
	if newState == StateOnline && bootstrapAddr != "" { go e.Bootstrap(ctx, bootstrapAddr) }
	return nil
}

func (e *Engine) GetState() NetworkState {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	return e.state
}

func (e *Engine) Bootstrap(ctx context.Context, seedAddr string) error {
	logger.Info("Bootstrap: Iniciando unión a la red mediante el nodo semilla: %s", seedAddr)
	addr, _ := net.ResolveUDPAddr("udp", seedAddr)
	pingMsg := Message{Type: Ping, TransactionID: "bootstrap-ping", SenderID: e.router.sender.LocalID()}
	resp, err := e.router.sender.Request(addr, pingMsg, 2*time.Second)
	if err != nil { return nil }
	logger.Info("Bootstrap: Conectado con nodo semilla ID: %x", resp.SenderID)
	found, _ := e.router.LookupNode(e.router.sender.LocalID())
	logger.Info("Bootstrap: Unión completada. Se descubrieron %d nuevos contactos.", len(found))
	return nil
}

// Helpers for detection
func isManifest(data []byte) (*Manifest, bool) {
	if len(data) == 0 || data[0] != '{' { return nil, false }
	var m Manifest
	if err := json.Unmarshal(data, &m); err == nil && m.Kind == ManifestKind { return &m, true }
	return nil, false
}

func isIndexChunk(data []byte) (*IndexChunk, bool) {
	if len(data) == 0 || data[0] != '{' { return nil, false }
	var idx IndexChunk
	if err := json.Unmarshal(data, &idx); err == nil && idx.Kind == IndexKind { return &idx, true }
	return nil, false
}

func mustMarshal(v any) string { d, _ := json.Marshal(v); return string(d) }

func getMapKeys(m map[string]struct{}) []string {
	var res []string
	for k := range m { res = append(res, k) }
	return res
}
