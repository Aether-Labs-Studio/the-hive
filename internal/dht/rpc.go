package dht

import (
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"
	"the-hive/internal/logger"
)

// MessageType defines the RPC methods available in Kademlia.
type MessageType string

const (
	Ping      MessageType = "PING"
	Store     MessageType = "STORE"
	FindNode  MessageType = "FIND_NODE"
	FindValue MessageType = "FIND_VALUE"
	Subscribe MessageType = "SUBSCRIBE"
	Publish   MessageType = "PUBLISH"
)

// ProtocolVersion is the current version of the DHT communication protocol.
const ProtocolVersion = "v1.0.0"

// Message is the standard RPC message for The Hive DHT.
type Message struct {
	Type          MessageType     `json:"type"`
	Version       string          `json:"version"` // Added for Phase 16
	TransactionID string          `json:"tx_id"`
	SenderID      NodeID          `json:"sender_id"`
	IsResponse    bool            `json:"is_response,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

// Handler defines the interface for processing incoming RPC messages.
type Handler interface {
	HandleMessage(addr net.Addr, msg Message)
}

// Transport handles the network communication for the DHT.
type Transport struct {
	localID  NodeID
	conn     net.PacketConn
	handler  Handler
	pending  map[string]chan Message
	done     chan struct{}
	mu       sync.Mutex
	wg       sync.WaitGroup
}

// NewTransport creates a new Transport instance for the local NodeID.
func NewTransport(localID NodeID, handler Handler) *Transport {
	return &Transport{
		localID: localID,
		handler: handler,
		pending: make(map[string]chan Message),
		done:    make(chan struct{}),
	}
}

// Listen starts a UDP listener on the given address and runs the read loop.
func (t *Transport) Listen(addr string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	t.conn = conn

	t.wg.Add(1)
	go t.readLoop()

	return nil
}

// SetHandler allows setting or updating the message handler for the transport.
func (t *Transport) SetHandler(h Handler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handler = h
}

// Send serializes and sends a Message to the specified remote address.
func (t *Transport) Send(addr net.Addr, msg Message) error {
	if msg.Version == "" {
		msg.Version = ProtocolVersion
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()

	if conn == nil {
		return errors.New("transport not listening")
	}

	_, err = conn.WriteTo(data, addr)
	return err
}

// Request sends a message and waits for a response with the same TransactionID.
func (t *Transport) Request(addr net.Addr, msg Message, timeout time.Duration) (Message, error) {
	ch := make(chan Message, 1)
	
	t.mu.Lock()
	t.pending[msg.TransactionID] = ch
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.pending, msg.TransactionID)
		t.mu.Unlock()
	}()

	if err := t.Send(addr, msg); err != nil {
		return Message{}, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(timeout):
		return Message{}, errors.New("request timeout")
	case <-t.done:
		return Message{}, errors.New("transport stopped")
	}
}

// readLoop listens for incoming UDP packets and unmarshals them.
func (t *Transport) readLoop() {
	defer t.wg.Done()
	buffer := make([]byte, 2048) // MTU is usually 1500, 2048 is safe.

	for {
		select {
		case <-t.done:
			return
		default:
			n, remoteAddr, err := t.conn.ReadFrom(buffer)
			if err != nil {
				// check if the error is due to closing the connection
				if errors.Is(err, net.ErrClosed) {
					return
				}
				logger.Error("Transport read error: %v", err)
				continue
			}

			// Processing the incoming message
			var msg Message
			if err := json.Unmarshal(buffer[:n], &msg); err != nil {
				logger.Error("Failed to unmarshal message from %v: %v", remoteAddr, err)
				continue
			}

			// Log reception for debugging (to Stderr)
			logger.Info("Received %s RPC from %x (tx: %s)", msg.Type, msg.SenderID, msg.TransactionID)

			if t.handler != nil {
				// Always notify handler to update routing table, etc.
				// For responses, we might want a different path, but for now
				// let's ensure the routing table is updated.
				t.handler.HandleMessage(remoteAddr, msg)
			}

			// Check if this is a response to a pending request
			t.mu.Lock()
			ch, isPending := t.pending[msg.TransactionID]
			t.mu.Unlock()

			if isPending {
				ch <- msg
				continue
			}
		}
	}
}

// Stop closes the UDP listener and waits for the read loop to exit.
func (t *Transport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn == nil {
		return nil
	}

	close(t.done)
	err := t.conn.Close()
	t.wg.Wait()
	return err
}

// LocalID returns the local node ID.
func (t *Transport) LocalID() NodeID {
	return t.localID
}

// Addr returns the network address the transport is listening on.
func (t *Transport) Addr() net.Addr {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return nil
	}
	return t.conn.LocalAddr()
}
