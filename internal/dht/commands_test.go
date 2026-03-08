package dht

import (
	"bytes"
	"encoding/json"
	"net"
	"testing"
	"time"
)

type mockSender struct {
	localID  NodeID
	sentMsg  Message
	sentAddr net.Addr
}

func (m *mockSender) Send(addr net.Addr, msg Message) error {
	m.sentMsg = msg
	m.sentAddr = addr
	return nil
}

func (m *mockSender) Request(addr net.Addr, msg Message, timeout time.Duration) (Message, error) {
	m.sentMsg = msg
	m.sentAddr = addr
	return Message{}, nil // Simplified for test
}

func (m *mockSender) LocalID() NodeID {
	return m.localID
}

func TestRouterHandlePing(t *testing.T) {
	localID := NewNodeID("local")
	rt := NewRoutingTable(localID)
	storage := NewInMemoryStorage()
	sender := &mockSender{localID: localID}
	router := NewRouter(sender, rt, storage)

	remoteAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9999}
	msg := Message{
		Type:          Ping,
		Version:       ProtocolVersion,
		TransactionID: "ping-tx",
		SenderID:      NewNodeID("remote"),
	}

	router.HandleMessage(remoteAddr, msg)

	if sender.sentMsg.Type != Ping {
		t.Errorf("Expected PING response, got %s", sender.sentMsg.Type)
	}
	if sender.sentMsg.TransactionID != "ping-tx" {
		t.Errorf("Expected TransactionID ping-tx, got %s", sender.sentMsg.TransactionID)
	}
	
	// Check if sender was added to routing table
	closest := rt.FindClosestContacts(msg.SenderID, 1)
	if len(closest) == 0 || closest[0].ID != msg.SenderID {
		t.Errorf("Sender was not added to routing table")
	}
}

func TestRouterHandleFindNode(t *testing.T) {
	localID := NewNodeID("local")
	rt := NewRoutingTable(localID)
	storage := NewInMemoryStorage()
	sender := &mockSender{localID: localID}
	router := NewRouter(sender, rt, storage)

	// Add a contact to the table
	contactID := NewNodeID("contact-1")
	contactAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}
	rt.AddContact(contactID, contactAddr)

	targetID := NewNodeID("target")
	payload, _ := json.Marshal(FindNodePayload{Target: targetID})
	
	remoteAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9999}
	msg := Message{
		Type:          FindNode,
		Version:       ProtocolVersion,
		TransactionID: "find-tx",
		SenderID:      NewNodeID("remote"),
		Payload:       payload,
	}

	router.HandleMessage(remoteAddr, msg)

	if sender.sentMsg.Type != FindNode {
		t.Errorf("Expected FIND_NODE response, got %s", sender.sentMsg.Type)
	}

	var resp FindNodeResponse
	if err := json.Unmarshal(sender.sentMsg.Payload, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response payload: %v", err)
	}

	found := false
	for _, c := range resp.Contacts {
		if c.ID == contactID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Response did not contain the expected contact")
	}
}

func TestRouterHandleStore(t *testing.T) {
	localID := NewNodeID("local")
	rt := NewRoutingTable(localID)
	storage := NewInMemoryStorage()
	sender := &mockSender{localID: localID}
	router := NewRouter(sender, rt, storage)

	key := NewNodeID("test-key")
	data := []byte("test-data")
	payload, _ := json.Marshal(StorePayload{Key: key, Data: data})

	remoteAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9999}
	msg := Message{
		Type:          Store,
		Version:       ProtocolVersion,
		TransactionID: "store-tx",
		SenderID:      NewNodeID("remote"),
		Payload:       payload,
	}

	router.HandleMessage(remoteAddr, msg)

	if sender.sentMsg.Type != Store {
		t.Errorf("Expected STORE response, got %s", sender.sentMsg.Type)
	}

	retrieved, ok := storage.Retrieve(key)
	if !ok || !bytes.Equal(retrieved, data) {
		t.Errorf("Stored data not found or incorrect")
	}
}

func TestProtocolVersioning(t *testing.T) {
	localID := NewNodeID("local")
	rt := NewRoutingTable(localID)
	storage := NewInMemoryStorage()
	sender := &mockSender{localID: localID}
	router := NewRouter(sender, rt, storage)

	remoteAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9999}
	
	// Incompatible version
	msg := Message{
		Type:          Ping,
		Version:       "v9.9.9",
		TransactionID: "bad-version",
		SenderID:      NewNodeID("remote"),
	}

	router.HandleMessage(remoteAddr, msg)

	// Should NOT respond
	if sender.sentMsg.TransactionID == "bad-version" {
		t.Errorf("Router should have rejected message with incompatible version")
	}
	
	// Compatible version (current)
	msg2 := Message{
		Type:          Ping,
		Version:       ProtocolVersion,
		TransactionID: "good-version",
		SenderID:      NewNodeID("remote"),
	}
	
	router.HandleMessage(remoteAddr, msg2)
	
	// Should respond
	if sender.sentMsg.TransactionID != "good-version" {
		t.Errorf("Router should have accepted message with compatible version")
	}
}

func TestRouterHandleFindValue(t *testing.T) {
	localID := NewNodeID("local")
	rt := NewRoutingTable(localID)
	storage := NewInMemoryStorage()
	sender := &mockSender{localID: localID}
	router := NewRouter(sender, rt, storage)

	key := NewNodeID("test-key")
	data := []byte("test-data")

	remoteAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9999}
	payload, _ := json.Marshal(FindValuePayload{Key: key})
	msg := Message{
		Type:          FindValue,
		Version:       ProtocolVersion,
		TransactionID: "find-val-tx",
		SenderID:      NewNodeID("remote"),
		Payload:       payload,
	}

	// 1. Test when value exists
	storage.Store(key, data)
	router.HandleMessage(remoteAddr, msg)

	var resp FindValueResponse
	json.Unmarshal(sender.sentMsg.Payload, &resp)
	if !bytes.Equal(resp.Value, data) {
		t.Errorf("Expected found value %s, got %s", data, resp.Value)
	}
	if len(resp.Contacts) != 0 {
		t.Errorf("Expected 0 contacts when value is found")
	}

	// 2. Test when value does not exist (returns closest contacts)
	contactID := NewNodeID("contact-1")
	contactAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}
	rt.AddContact(contactID, contactAddr)
	
	key2 := NewNodeID("non-existent")
	payload2, _ := json.Marshal(FindValuePayload{Key: key2})
	msg2 := Message{
		Type:          FindValue,
		Version:       ProtocolVersion,
		TransactionID: "find-val-tx-2",
		SenderID:      NewNodeID("remote"),
		Payload:       payload2,
	}

	router.HandleMessage(remoteAddr, msg2)
	
	var resp2 FindValueResponse
	json.Unmarshal(sender.sentMsg.Payload, &resp2)
	if resp2.Value != nil {
		t.Errorf("Expected nil value for non-existent key")
	}
	
	foundContact := false
	for _, c := range resp2.Contacts {
		if c.ID == contactID {
			foundContact = true
			break
		}
	}
	if !foundContact {
		t.Errorf("Expected contactID in closest contacts for non-existent key")
	}
}
