package dht

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"
	"the-hive/internal/logger"
)

// Sender defines the interface for sending DHT messages.
type Sender interface {
	Send(addr net.Addr, msg Message) error
	Request(addr net.Addr, msg Message, timeout time.Duration) (Message, error)
	LocalID() NodeID
}

// Signer defines the interface for signing data.
type Signer interface {
	Sign(data []byte, parentID string) ([]byte, error)
	ExtractAndInspectSecure(chunk []byte, expectedID NodeID) ([]byte, ed25519.PublicKey, string, error)
}

// Router implements the Handler interface to route RPC messages.
type Router struct {
	sender         Sender
	rt             *RoutingTable
	storage        Storage
	newContactChan chan Contact
	subMgr         *SubscriptionManager
	subStore       *SubscriptionStore
	signer         Signer // Added for Phase 16
	mu             sync.Mutex // Protects atomic operations like index merging
}

// NewRouter creates a new Router.
func NewRouter(sender Sender, rt *RoutingTable, storage Storage) *Router {
	return &Router{
		sender:         sender,
		rt:             rt,
		storage:        storage,
		newContactChan: make(chan Contact, 64),
		subStore:       NewSubscriptionStore(),
	}
}

func (r *Router) SetSigner(s Signer) {
	r.signer = s
}

// ... helpers for merging ...

func (r *Router) isIndex(envelope []byte) (*IndexChunk, bool) {
	if r.signer == nil { return nil, false }
	// Indices don't follow CAS hash-matching during merge checks
	payload, _, _, err := r.signer.ExtractAndInspectSecure(envelope, NodeID{})
	if err != nil { return nil, false }
	
	var idx IndexChunk
	if err := json.Unmarshal(payload, &idx); err == nil && idx.Kind == IndexKind {
		return &idx, true
	}
	return nil, false
}

func (r *Router) mergePointers(p1, p2 []string) []string {
	unique := make(map[string]struct{})
	for _, p := range p1 { unique[p] = struct{}{} }
	for _, p := range p2 { unique[p] = struct{}{} }
	
	res := make([]string, 0, len(unique))
	for p := range unique {
		res = append(res, p)
	}
	// Sort for determinism
	sort.Strings(res)
	
	// Optional: cap size
	if len(res) > 50 {
		res = res[len(res)-50:]
	}
	return res
}

func (r *Router) signData(data []byte) ([]byte, error) {
	if r.signer == nil {
		return nil, fmt.Errorf("no signer")
	}
	return r.signer.Sign(data, "")
}

func (r *Router) SetSubscriptionManager(s *SubscriptionManager) {
	r.subMgr = s
}

// Node represents a contact with its ID and network address.
type Node struct {
	ID   NodeID `json:"id"`
	Addr string `json:"addr"`
}

// HandleMessage routes incoming messages to their respective handlers.
func (r *Router) HandleMessage(addr net.Addr, msg Message) {
	// Version Check (Phase 16)
	// We allow minor versions if we were using semver, but for now we require exact match
	// or at least that the major version matches if we implemented it.
	if msg.Version != ProtocolVersion {
		logger.Warn("Router: Mensaje rechazado de %s. Versión incompatible: %s (Local: %s)", 
			addr, msg.Version, ProtocolVersion)
		return
	}

	// Update routing table with sender if it's not us
	if isNew := r.rt.AddContact(msg.SenderID, addr); isNew {
		GlobalTelemetry.Emit(PeerJoined, fmt.Sprintf("%x", msg.SenderID), addr.String())
		// Notify background workers about the new contact for handoff/redistribution
		select {
		case r.newContactChan <- Contact{ID: msg.SenderID, Address: addr}:
		default:
			// Channel full, drop notification (periodic replication will eventually cover it)
		}
	}

	// If it's a response, we don't need to process it further here
	if msg.IsResponse {
		return
	}

	switch msg.Type {
	case Ping:
		r.handlePing(addr, msg)
	case FindNode:
		r.handleFindNode(addr, msg)
	case Store:
		r.handleStore(addr, msg)
	case FindValue:
		r.handleFindValue(addr, msg)
	case Subscribe:
		r.handleSubscribe(addr, msg)
	case Publish:
		r.handlePublish(addr, msg)
	default:
		logger.Info("Unhandled message type: %s", msg.Type)
	}
}

func (r *Router) handlePing(addr net.Addr, msg Message) {
	resp := Message{
		Type:          Ping,
		TransactionID: msg.TransactionID,
		SenderID:      r.sender.LocalID(),
		IsResponse:    true,
	}
	r.sender.Send(addr, resp)
}

// FindNodePayload contains the target ID for a FIND_NODE request.
type FindNodePayload struct {
	Target NodeID `json:"target"`
}

// FindNodeResponse contains the list of closest contacts found.
type FindNodeResponse struct {
	Contacts []Node `json:"contacts"`
}

func (r *Router) handleFindNode(addr net.Addr, msg Message) {
	var payload FindNodePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		logger.Error("Failed to unmarshal FIND_NODE payload: %v", err)
		return
	}

	closest := r.rt.FindClosestContacts(payload.Target, K)
	nodes := make([]Node, len(closest))
	for i, c := range closest {
		nodes[i] = Node{ID: c.ID, Addr: c.Address.String()}
	}
	
	respPayload, _ := json.Marshal(FindNodeResponse{Contacts: nodes})
	resp := Message{
		Type:          FindNode,
		TransactionID: msg.TransactionID,
		SenderID:      r.sender.LocalID(),
		IsResponse:    true,
		Payload:       respPayload,
	}
	r.sender.Send(addr, resp)
}

// StorePayload contains the data to be stored and its target ID.
type StorePayload struct {
	Key  NodeID `json:"key"`
	Data []byte `json:"data"`
}

func (r *Router) handleStore(addr net.Addr, msg Message) {
	var payload StorePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		logger.Error("Failed to unmarshal STORE payload: %v", err)
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Phase 16: Set Merging for Indices
	finalData := payload.Data
	
	// 1. Peek if incoming is an index
	if newIdx, ok := r.isIndex(payload.Data); ok {
		// 2. Check if we already have an index for this key
		if existing, found := r.storage.Retrieve(payload.Key); found {
			if oldIdx, ok := r.isIndex(existing); ok {
				// 3. Merge Pointers
				mergedPointers := r.mergePointers(oldIdx.Pointers, newIdx.Pointers)
				
				// 4. Create merged index chunk
				mergedIdx := IndexChunk{
					Kind:     IndexKind,
					Pointers: mergedPointers,
				}
				mergedRaw, _ := json.Marshal(mergedIdx)
				
				// 5. Re-sign the merged data (as broker/responsible node)
				// We need access to the Engine's Signer or just use the local signer.
				// Since Router has access to Sender which has identity info? No.
				// We need to inject the Signer into the Router.
				if signed, err := r.signData(mergedRaw); err == nil {
					finalData = signed
					logger.Info("Router: Fusionado índice para %x (%d -> %d punteros)", 
						payload.Key, len(oldIdx.Pointers), len(mergedPointers))
				}
			}
		}
	}

	if err := r.storage.Store(payload.Key, finalData); err != nil {
		logger.Error("Failed to store data: %v", err)
		return
	}

	// Kademlia STORE normally returns the SenderID as confirmation
	resp := Message{
		Type:          Store,
		TransactionID: msg.TransactionID,
		SenderID:      r.sender.LocalID(),
		IsResponse:    true,
	}
	r.sender.Send(addr, resp)
}

// SubscribePayload contains the topic key the sender is interested in.
type SubscribePayload struct {
	TopicID NodeID `json:"topic_id"`
}

func (r *Router) handleSubscribe(addr net.Addr, msg Message) {
	var payload SubscribePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil { return }

	r.subStore.AddSubscriber(payload.TopicID, Contact{ID: msg.SenderID, Address: addr})
	
	resp := Message{
		Type: Subscribe, TransactionID: msg.TransactionID, 
		SenderID: r.sender.LocalID(), IsResponse: true,
	}
	r.sender.Send(addr, resp)
}

// PublishPayload contains the keyword and the ID of the new chunk.
type PublishPayload struct {
	Keyword string `json:"keyword"`
	ChunkID NodeID `json:"chunk_id"`
}

func (r *Router) handlePublish(addr net.Addr, msg Message) {
	var payload PublishPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil { return }

	topicID := NewNodeID(payload.Keyword)
	subscribers := r.subStore.subscribers[topicID] // Local access for efficiency

	for _, sub := range subscribers {
		// If the subscriber is US (local subscription), notify SSE
		if sub.ID == r.sender.LocalID() {
			GlobalTelemetry.Emit(TopicMatch, payload.Keyword, fmt.Sprintf("Alert: New content for %s", payload.Keyword))
		} else {
			// In a more advanced version, we would forward the PUBLISH to them.
			// But for now, we assume local SSE clients on the broker node or the subscriber is watching.
			// Refinement: If we are the broker, and we have a record of a remote subscriber, 
			// we should technically notify them. But how? SSE is local.
			// Let's keep it simple: any node that is a broker and has a match, emits SSE.
			// This covers the case where the subscriber is also a broker or watching the broker.
			GlobalTelemetry.Emit(TopicMatch, payload.Keyword, fmt.Sprintf("Broker Alert: Content detected for %s", payload.Keyword))
		}
	}

	resp := Message{
		Type: Publish, TransactionID: msg.TransactionID,
		SenderID: r.sender.LocalID(), IsResponse: true,
	}
	r.sender.Send(addr, resp)
}

// FindValuePayload contains the key being searched.
type FindValuePayload struct {
	Key NodeID `json:"key"`
}

// FindValueResponse contains either the data (if found) or the closest contacts.
type FindValueResponse struct {
	Value    []byte `json:"value,omitempty"`
	Contacts []Node `json:"contacts,omitempty"`
}

func (r *Router) handleFindValue(addr net.Addr, msg Message) {
	var payload FindValuePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		logger.Error("Failed to unmarshal FIND_VALUE payload: %v", err)
		return
	}

	var respPayload FindValueResponse
	data, found := r.storage.Retrieve(payload.Key)
	if found {
		respPayload.Value = data
	} else {
		closest := r.rt.FindClosestContacts(payload.Key, K)
		nodes := make([]Node, len(closest))
		for i, c := range closest {
			nodes[i] = Node{ID: c.ID, Addr: c.Address.String()}
		}
		respPayload.Contacts = nodes
	}

	rawRespPayload, _ := json.Marshal(respPayload)
	resp := Message{
		Type:          FindValue,
		TransactionID: msg.TransactionID,
		SenderID:      r.sender.LocalID(),
		IsResponse:    true,
		Payload:       rawRespPayload,
	}
	r.sender.Send(addr, resp)
}

// FindValue performs a lookup for the given key. It first checks local storage,
// and if not found, it performs an iterative search in the network.
func (r *Router) FindValue(key NodeID) ([]byte, bool) {
	// 1. Check local storage
	data, found := r.storage.Retrieve(key)
	if found {
		return data, true
	}

	// 2. Network Search (Iterative)
	initial := r.rt.FindClosestContacts(key, K)
	if len(initial) == 0 {
		return nil, false
	}

	shortlist := NewShortlist(key, initial)
	resChan := make(chan []byte, 1)
	doneChan := make(chan struct{})
	var once sync.Once

	for {
		next := shortlist.GetNextToQuery(Alpha)
		if len(next) == 0 {
			if shortlist.IsFinished() {
				break
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}

		var wg sync.WaitGroup
		for _, contact := range next {
			wg.Add(1)
			go func(c Contact) {
				defer wg.Done()

				payload, _ := json.Marshal(FindValuePayload{Key: key})
				msg := Message{
					Type:          FindValue,
					TransactionID: fmt.Sprintf("findval-%x-%x", key[:4], c.ID[:4]),
					SenderID:      r.sender.LocalID(),
					Payload:       payload,
				}

				resp, err := r.sender.Request(c.Address, msg, 1000*time.Millisecond)
				if err != nil {
					GlobalTelemetry.Emit(RPCError, fmt.Sprintf("%x", c.ID), fmt.Sprintf("FindValue failed to %s: %v", c.Address, err))
					shortlist.MarkFailed(c.ID)
					return
				}

				var fvResp FindValueResponse
				if err := json.Unmarshal(resp.Payload, &fvResp); err != nil {
					shortlist.MarkFailed(c.ID)
					return
				}

				if fvResp.Value != nil {
					once.Do(func() {
						resChan <- fvResp.Value
						close(doneChan)
					})
					return
				}

				foundContacts := make([]Contact, 0, len(fvResp.Contacts))
				for _, node := range fvResp.Contacts {
					addr, err := net.ResolveUDPAddr("udp", node.Addr)
					if err == nil {
						foundContacts = append(foundContacts, Contact{ID: node.ID, Address: addr})
						r.rt.AddContact(node.ID, addr)
					}
				}
				shortlist.MarkResponded(c.ID, foundContacts)
			}(contact)
		}

		// Wait for this round or for value to be found
		finishedRound := make(chan struct{})
		go func() {
			wg.Wait()
			close(finishedRound)
		}()

		select {
		case result := <-resChan:
			return result, true
		case <-finishedRound:
			// Continue to next round
		}
	}

	return nil, false
}

// LookupNode performs an iterative search for the target NodeID.
// It returns the closest contacts found in the network.
func (r *Router) LookupNode(target NodeID) ([]Contact, error) {
	// 1. Get initial closest contacts from local routing table.
	initial := r.rt.FindClosestContacts(target, K)
	if len(initial) == 0 {
		return nil, fmt.Errorf("no initial contacts for lookup")
	}

	shortlist := NewShortlist(target, initial)

	for {
		next := shortlist.GetNextToQuery(Alpha)
		if len(next) == 0 {
			if shortlist.IsFinished() {
				break
			}
			// Wait a bit for pending queries to finish
			time.Sleep(50 * time.Millisecond)
			continue
		}

		var wg sync.WaitGroup
		for _, contact := range next {
			wg.Add(1)
			go func(c Contact) {
				defer wg.Done()

				payload, _ := json.Marshal(FindNodePayload{Target: target})
				msg := Message{
					Type:          FindNode,
					TransactionID: fmt.Sprintf("lookup-%x-%x", target[:4], c.ID[:4]),
					SenderID:      r.sender.LocalID(),
					Payload:       payload,
				}

				resp, err := r.sender.Request(c.Address, msg, 1000*time.Millisecond)
				if err != nil {
					GlobalTelemetry.Emit(RPCError, fmt.Sprintf("%x", c.ID), fmt.Sprintf("Lookup failed to %s: %v", c.Address, err))
					shortlist.MarkFailed(c.ID)
					return
				}

				var fnResp FindNodeResponse
				if err := json.Unmarshal(resp.Payload, &fnResp); err != nil {
					shortlist.MarkFailed(c.ID)
					return
				}

				found := make([]Contact, 0, len(fnResp.Contacts))
				for _, node := range fnResp.Contacts {
					addr, err := net.ResolveUDPAddr("udp", node.Addr)
					if err == nil {
						found = append(found, Contact{ID: node.ID, Address: addr})
						// Update our RT with new nodes discovered
						r.rt.AddContact(node.ID, addr)
					}
				}
				shortlist.MarkResponded(c.ID, found)
			}(contact)
		}
		wg.Wait()

		// If we haven't found any closer node than the best one we had, we can potentially stop
		// but Kademlia paper suggests continuing until K closest nodes have all been queried.
		// Our shortlist.IsFinished() handles the K-closest-queried condition.
	}

	return shortlist.GetClosest(), nil
}

// StoreValueRemote sends a STORE RPC to a remote address.
func (r *Router) StoreValueRemote(addr net.Addr, key NodeID, data []byte) error {
	payload, err := json.Marshal(StorePayload{Key: key, Data: data})
	if err != nil {
		return err
	}

	msg := Message{
		Type:          Store,
		TransactionID: fmt.Sprintf("store-%x", key[:4]),
		SenderID:      r.sender.LocalID(),
		Payload:       payload,
	}

	_, err = r.sender.Request(addr, msg, 2*time.Second)
	if err != nil {
		GlobalTelemetry.Emit(RPCError, "", fmt.Sprintf("Store RPC failed to %s: %v", addr, err))
	}
	return err
}

// StoreValue performs a local store operation in the storage.
func (r *Router) StoreValue(key NodeID, data []byte) error {
	return r.storage.Store(key, data)
}
