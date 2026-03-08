package mcp

import (
	"bufio"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"the-hive/internal/dht"
	"the-hive/internal/logger"
)

// DHT defines the interface for the DHT operations needed by the MCP server.
type DHT interface {
	FindValue(dht.NodeID) ([]byte, bool)
	StoreValue(dht.NodeID, []byte) error
	
	// Reputation Support
	GetReputation(pubKey []byte) int
	RateAuthor(pubKey []byte, delta int)

		// Knowledge Methods
	Search(query string) (string, error)
	Share(topic, content, parentID string) (string, error)
	Rate(chunkID string, score int) (string, error)
}

// Sanitizer defines the interface for inbound and outbound data validation.
type Sanitizer interface {
	ExtractAndInspect(chunk []byte) ([]byte, error)
	ExtractAndInspectSecure(chunk []byte, expectedID dht.NodeID) ([]byte, ed25519.PublicKey, string, error)
	Sanitize(raw []byte) []byte
	Sign(data []byte, parentID string) ([]byte, error)
	PackageChunk(sanitized []byte, parentID string) ([]byte, dht.NodeID, error)
	IsTopicBlocked(topic string) bool
}

// JSON-RPC 2.0 structures
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Server implements the MCP protocol over STDIO.
type Server struct {
	dht       DHT
	sanitizer Sanitizer
	input     io.Reader
	output    io.Writer
	mu        sync.Mutex
}

// NewServer creates a new MCPServer instance.
func NewServer(dht DHT, sanitizer Sanitizer) *Server {
	return &Server{
		dht:       dht,
		sanitizer: sanitizer,
		input:     os.Stdin,
		output:    os.Stdout,
	}
}

// Serve starts the JSON-RPC read loop on Stdin.
func (s *Server) Serve() {
	scanner := bufio.NewScanner(s.input)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		s.handleRequest(req)
	}

	if err := scanner.Err(); err != nil {
		logger.Error("MCP Server scan error: %v", err)
	}
}

func (s *Server) handleRequest(req Request) {
	switch req.Method {
	case "initialize":
		s.sendResponse(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"serverInfo": map[string]string{
				"name":    "the-hive",
				"version": "0.1.0",
			},
		})
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolCall(req)
	default:
		if req.ID != nil {
			s.sendError(req.ID, -32601, "Method not found", req.Method)
		}
	}
}

func (s *Server) sendResponse(id json.RawMessage, result any) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.write(resp)
}

func (s *Server) sendError(id json.RawMessage, code int, message string, data any) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	s.write(resp)
}

func (s *Server) write(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(v)
	if err != nil {
		logger.Error("Failed to marshal JSON-RPC response: %v", err)
		return
	}

	fmt.Fprintln(s.output, string(data))
}
