package dht

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestMessageSerialization(t *testing.T) {
	msg := Message{
		Type:          Ping,
		TransactionID: "tx-123",
		SenderID:      NewNodeID("sender"),
		Payload:       json.RawMessage(`{"msg":"ping-payload"}`),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	var decoded Message
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if decoded.Type != msg.Type || decoded.TransactionID != msg.TransactionID || decoded.SenderID != msg.SenderID {
		t.Errorf("Decoded message does not match original: %+v vs %+v", decoded, msg)
	}
}

func TestNetworkListenAndStop(t *testing.T) {
	localID := NewNodeID("local")
	transport := NewTransport(localID, nil)

	// Listen on a dynamic local port
	err := transport.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	addr := transport.Addr().String()
	t.Logf("Listening on %s", addr)

	// Send a dummy UDP packet to verify it's listening
	conn, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	msg := Message{
		Type:          Ping,
		Version:       ProtocolVersion,
		TransactionID: "test-tx",
		SenderID:      NewNodeID("client"),
	}
	data, _ := json.Marshal(msg)
	_, err = conn.Write(data)
	if err != nil {
		t.Errorf("Failed to write to UDP: %v", err)
	}
	conn.Close()

	// Small delay to let the packet arrive (though we aren't asserting on processing yet)
	time.Sleep(50 * time.Millisecond)

	// Test graceful shutdown
	err = transport.Stop()
	if err != nil {
		t.Errorf("Failed to stop transport: %v", err)
	}
}
