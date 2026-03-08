package dht

import (
	"encoding/json"
	"net"
	"testing"
	"crypto/ed25519"
)

type mockSigner struct{}

func (m *mockSigner) Sign(data []byte, parentID string) ([]byte, error) {
	// Simple mock: wrap in a dummy SignedEnvelope JSON
	envelope := struct {
		Data      []byte `json:"data"`
		Signature []byte `json:"signature"`
		ParentID  string `json:"parent_id"`
	}{
		Data:      data,
		Signature: []byte("fake-sig"),
		ParentID:  parentID,
	}
	return json.Marshal(envelope)
}

func (m *mockSigner) ExtractAndInspectSecure(chunk []byte, expectedID NodeID) ([]byte, ed25519.PublicKey, string, error) {
	// Simple mock: unwrap the dummy envelope
	var envelope struct {
		Data      []byte `json:"data"`
		Signature []byte `json:"signature"`
		ParentID  string `json:"parent_id"`
	}
	if err := json.Unmarshal(chunk, &envelope); err != nil {
		return chunk, nil, "", nil // Fallback for raw data
	}
	return envelope.Data, nil, envelope.ParentID, nil
}

func TestIndexMerging(t *testing.T) {
	localID := NewNodeID("local")
	rt := NewRoutingTable(localID)
	storage := NewInMemoryStorage()
	sender := &mockSender{localID: localID}
	router := NewRouter(sender, rt, storage)
	signer := &mockSigner{}
	router.SetSigner(signer)

	keyword := "golang"
	kwID := NewNodeID(keyword)

	// 1. First index for "golang" with pointer "A"
	idx1 := IndexChunk{Kind: IndexKind, Pointers: []string{"A"}}
	raw1, _ := json.Marshal(idx1)
	signed1, _ := signer.Sign(raw1, "")
	
	payload1, _ := json.Marshal(StorePayload{Key: kwID, Data: signed1})
	msg1 := Message{
		Type: Store, Version: ProtocolVersion, TransactionID: "tx1", 
		SenderID: NewNodeID("remote1"), Payload: payload1,
	}
	router.HandleMessage(&net.UDPAddr{}, msg1)

	// 2. Second index for "golang" with pointer "B"
	idx2 := IndexChunk{Kind: IndexKind, Pointers: []string{"B"}}
	raw2, _ := json.Marshal(idx2)
	signed2, _ := signer.Sign(raw2, "")
	
	payload2, _ := json.Marshal(StorePayload{Key: kwID, Data: signed2})
	msg2 := Message{
		Type: Store, Version: ProtocolVersion, TransactionID: "tx2", 
		SenderID: NewNodeID("remote2"), Payload: payload2,
	}
	router.HandleMessage(&net.UDPAddr{}, msg2)

	// 3. Verify storage has BOTH A and B
	data, found := storage.Retrieve(kwID)
	if !found {
		t.Fatalf("Index not found in storage")
	}

	payload, _, _, _ := signer.ExtractAndInspectSecure(data, NodeID{})
	var merged IndexChunk
	json.Unmarshal(payload, &merged)

	if len(merged.Pointers) != 2 {
		t.Errorf("Expected 2 pointers, got %d: %v", len(merged.Pointers), merged.Pointers)
	}
	
	hasA, hasB := false, false
	for _, p := range merged.Pointers {
		if p == "A" { hasA = true }
		if p == "B" { hasB = true }
	}
	if !hasA || !hasB {
		t.Errorf("Merged index is missing pointers: %v", merged.Pointers)
	}
}
