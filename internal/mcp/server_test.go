package mcp

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"the-hive/internal/dht"
)

type mockDHT struct {
	data       map[dht.NodeID][]byte
	reputation map[string]int
	shareErr   error
	mu         sync.RWMutex
}

func (m *mockDHT) FindValue(key dht.NodeID) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.data[key]
	return d, ok
}

func (m *mockDHT) StoreValue(key dht.NodeID, data []byte, state dht.ChunkState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[dht.NodeID][]byte)
	}
	m.data[key] = data
	return nil
}

func (m *mockDHT) GetReputation(pubKey []byte) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.reputation == nil {
		return 0
	}
	return m.reputation[hex.EncodeToString(pubKey)]
}

func (m *mockDHT) RateAuthor(pubKey []byte, delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reputation == nil {
		m.reputation = make(map[string]int)
	}
	m.reputation[hex.EncodeToString(pubKey)] += delta
}

func (m *mockDHT) Search(q string) ([]dht.SearchResult, error) {
	if q == "fail" {
		return nil, fmt.Errorf("error")
	}
	return []dht.SearchResult{{Content: "Search Result for " + q, AuthorID: "test-node", Reputation: 10}}, nil
}

func (m *mockDHT) Share(t, c, p string, s dht.ChunkState) (string, error) {
	if m.shareErr != nil {
		return "", m.shareErr
	}
	if s != "" {
		return "Processed (" + string(s) + ")", nil
	}
	return "Shared", nil
}
func (m *mockDHT) Rate(id string, s int) (string, error) { return "Rated", nil }

type mockSanitizer struct {
	blockedTopics map[string]bool
	pubKey        ed25519.PublicKey
}

func (m *mockSanitizer) ExtractAndInspect(chunk []byte) ([]byte, error) { return chunk, nil }
func (m *mockSanitizer) ExtractAndInspectSecure(chunk []byte, id dht.NodeID) ([]byte, ed25519.PublicKey, string, error) {
	if m.pubKey == nil {
		m.pubKey = make([]byte, ed25519.PublicKeySize)
	}
	return chunk, m.pubKey, "", nil
}
func (m *mockSanitizer) Sanitize(raw []byte) []byte                 { return raw }
func (m *mockSanitizer) Sign(data []byte, p string) ([]byte, error) { return data, nil }
func (m *mockSanitizer) PackageChunk(sanitized []byte, p string) ([]byte, dht.NodeID, error) {
	return sanitized, dht.NewNodeID(string(sanitized)), nil
}
func (m *mockSanitizer) IsTopicBlocked(topic string) bool { return m.blockedTopics[topic] }

func TestMCPServerToolsList(t *testing.T) {
	srv := NewServer(&mockDHT{}, &mockSanitizer{})
	input := strings.NewReader(`{"jsonrpc":"2.0","method":"tools/list","id":1}` + "\n")
	output := &bytes.Buffer{}
	srv.input = input
	srv.output = output
	srv.Serve()
	var resp Response
	json.Unmarshal(output.Bytes(), &resp)
	result := resp.Result.(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) < 3 {
		t.Errorf("Expected at least 3 tools, got %d", len(tools))
	}
}

func TestMCPServerHiveSearch(t *testing.T) {
	srv := NewServer(&mockDHT{}, &mockSanitizer{})
	params := map[string]any{
		"name":      "hive_search",
		"arguments": map[string]string{"query": "test"},
	}
	paramsJSON, _ := json.Marshal(params)
	req := Request{JSONRPC: "2.0", Method: "tools/call", Params: paramsJSON, ID: json.RawMessage("1")}
	output := &bytes.Buffer{}
	srv.output = output

	srv.handleToolCall(req)
	if !strings.Contains(output.String(), "Search Result for test") {
		t.Errorf("Unexpected output: %s", output.String())
	}
}

func TestMCPServerHiveShare(t *testing.T) {
	srv := NewServer(&mockDHT{}, &mockSanitizer{})
	params := map[string]any{
		"name":      "hive_share",
		"arguments": map[string]string{"content": "content", "topic": "topic", "parent_id": "parent"},
	}
	paramsJSON, _ := json.Marshal(params)
	req := Request{JSONRPC: "2.0", Method: "tools/call", Params: paramsJSON, ID: json.RawMessage("1")}
	output := &bytes.Buffer{}
	srv.output = output

	srv.handleToolCall(req)
	if !strings.Contains(output.String(), "Shared") {
		t.Errorf("Unexpected output: %s", output.String())
	}
}

func TestMCPServerHiveShareReturnsErrorFromEngine(t *testing.T) {
	srv := NewServer(&mockDHT{shareErr: fmt.Errorf("security policy: %s", dhtShareBinaryError())}, &mockSanitizer{})
	params := map[string]any{
		"name":      "hive_share",
		"arguments": map[string]string{"content": "data:image/png;base64,AAAA", "topic": "topic"},
	}
	paramsJSON, _ := json.Marshal(params)
	req := Request{JSONRPC: "2.0", Method: "tools/call", Params: paramsJSON, ID: json.RawMessage("1")}
	output := &bytes.Buffer{}
	srv.output = output

	srv.handleToolCall(req)
	if !strings.Contains(output.String(), "\"isError\":true") {
		t.Fatalf("expected MCP error response, got: %s", output.String())
	}
}

func dhtShareBinaryError() string {
	return "community edition does not allow file or binary uploads"
}
