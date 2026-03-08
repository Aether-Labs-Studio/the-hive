package mcp

import (
	"encoding/json"
	"fmt"
	"the-hive/internal/logger"
)

type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type HiveSearchParams struct {
	Query string `json:"query"`
}

type HiveShareParams struct {
	Content  string `json:"content"`
	Topic    string `json:"topic,omitempty"`
	ParentID string `json:"parent_id,omitempty"` // Added for Phase 16 traceability
}

type HiveRateParams struct {
	ChunkID string `json:"chunk_id"`
	Score   int    `json:"score"`
}

func (s *Server) handleToolsList(req Request) {
	tools := []map[string]any{
		{
			"name":        "hive_search",
			"description": "Search the Hive for shared memories and knowledge.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query or topic to look up in the DHT.",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "hive_share",
			"description": "Share a high-value discovery or memory with the Hive. Content will be sanitized before sharing.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{
						"type":        "string",
						"description": "The memory or knowledge to share.",
					},
					"topic": map[string]any{
						"type":        "string",
						"description": "Optional topic or category.",
					},
					"parent_id": map[string]any{
						"type":        "string",
						"description": "Optional ID of the previous version of this discovery.",
					},
				},
				"required": []string{"content"},
			},
		},
		{
			"name":        "hive_rate",
			"description": "Rate a piece of shared knowledge. +1 for good, -1 for bad.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"chunk_id": map[string]any{
						"type":        "string",
						"description": "The ID of the chunk to rate (hex format).",
					},
					"score": map[string]any{
						"type":        "integer",
						"description": "The score: +1 or -1.",
						"enum":        []int{1, -1},
					},
				},
				"required": []string{"chunk_id", "score"},
			},
		},
	}
	s.sendResponse(req.ID, map[string]any{
		"tools": tools,
	})
}

func (s *Server) handleToolCall(req Request) {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

	switch params.Name {
	case "hive_search":
		s.handleHiveSearch(req, params.Arguments)
	case "hive_share":
		s.handleHiveShare(req, params.Arguments)
	case "hive_rate":
		s.handleHiveRate(req, params.Arguments)
	default:
		s.sendError(req.ID, -32601, "Tool not found", params.Name)
	}
}

func (s *Server) handleHiveSearch(req Request, rawArgs json.RawMessage) {
	var args HiveSearchParams
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		s.sendError(req.ID, -32602, "Invalid arguments", err.Error())
		return
	}

	// Delegar al Engine (que ahora implementa la lógica de búsqueda descentralizada)
	res, err := s.dht.Search(args.Query)
	if err != nil {
		s.sendError(req.ID, -32603, "Search error", err.Error())
		return
	}

	s.sendResponse(req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": res}},
	})
}

func (s *Server) handleHiveShare(req Request, rawArgs json.RawMessage) {
	var args HiveShareParams
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		s.sendError(req.ID, -32602, "Invalid arguments", err.Error())
		return
	}

	res, err := s.dht.Share(args.Topic, args.Content, args.ParentID)
	if err != nil {
		logger.Error("MCP: Share failed: %v", err)
		s.sendResponse(req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("Error al compartir: %v", err)}},
			"isError": true,
		})
		return
	}

	s.sendResponse(req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": res}},
	})
}

func (s *Server) handleHiveRate(req Request, rawArgs json.RawMessage) {
	var args HiveRateParams
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		s.sendError(req.ID, -32602, "Invalid arguments", err.Error())
		return
	}

	res, err := s.dht.Rate(args.ChunkID, args.Score)
	if err != nil {
		s.sendError(req.ID, -32603, "Rating error", err.Error())
		return
	}

	s.sendResponse(req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": res}},
	})
}
