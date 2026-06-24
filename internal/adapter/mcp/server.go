// server.go — MCP SERVER role (§4.8): the router publishes its own tools as an
// MCP server over streamable-HTTP JSON-RPC 2.0. Built-in tools expose router
// introspection (list providers, list issued keys, run a proxied chat).
//
// ponytail: ceiling — tools surface only (no resources/prompts/sampling); the
// transport is one JSON-RPC response per POST (no long-lived SSE subscription).
// Growth path = the full mcp-go SDK + streamable-HTTP session semantics.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// ToolHandler executes one MCP tool by name with JSON arguments and returns
// raw JSON content. Implementations live in the use-case/transport layer.
type ToolHandler func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// Server is the router's own MCP server. Register tools before serving.
type Server struct {
	mu    sync.RWMutex
	tools map[string]registeredTool
}

type registeredTool struct {
	desc        string
	inputSchema json.RawMessage
	handler     ToolHandler
}

// NewServer builds an empty MCP server.
func NewServer() *Server { return &Server{tools: map[string]registeredTool{}} }

// Register adds (or replaces) a tool. inputSchema is a raw JSON-Schema object.
func (s *Server) Register(name, description string, inputSchema json.RawMessage, h ToolHandler) {
	if name == "" || h == nil {
		return
	}
	if len(inputSchema) == 0 {
		inputSchema = []byte(`{"type":"object","properties":{}}`)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[name] = registeredTool{desc: description, inputSchema: inputSchema, handler: h}
}

// ServeHTTP implements the streamable-HTTP JSON-RPC endpoint.
// It answers initialize / tools/list / tools/call; everything else → -32601.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req rpcRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeRPCError(w, req.ID, -32700, "parse error")
		return
	}
	switch req.Method {
	case "initialize":
		s.handleInitialize(w, req)
	case "tools/list":
		s.handleToolsList(w, req)
	case "tools/call":
		s.handleToolsCall(w, r, req)
	default:
		writeRPCError(w, req.ID, -32601, "method not found")
	}
}

func (s *Server) handleInitialize(w http.ResponseWriter, req rpcRequest) {
	result := map[string]any{
		"protocolVersion": "2024-11-05",
		"serverInfo":      map[string]any{"name": "ai-arbuz-provider", "version": "1.0.0"},
		"capabilities":    map[string]any{"tools": map[string]any{}},
	}
	writeRPCResult(w, req.ID, result)
}

func (s *Server) handleToolsList(w http.ResponseWriter, req rpcRequest) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tools := make([]map[string]any, 0, len(s.tools))
	for name, t := range s.tools {
		tools = append(tools, map[string]any{
			"name": name, "description": t.desc, "inputSchema": json.RawMessage(t.inputSchema),
		})
	}
	writeRPCResult(w, req.ID, map[string]any{"tools": tools})
}

func (s *Server) handleToolsCall(w http.ResponseWriter, r *http.Request, req rpcRequest) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	// req.Params is json.RawMessage in the rpcRequest envelope (see bridge.go).
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeRPCError(w, req.ID, -32602, "invalid params")
			return
		}
	}
	s.mu.RLock()
	t, ok := s.tools[params.Name]
	s.mu.RUnlock()
	if !ok {
		writeRPCError(w, req.ID, -32602, "unknown tool: "+params.Name)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	content, err := t.handler(ctx, params.Arguments)
	if err != nil {
		// Tool failure is a result with isError=true (MCP), not an RPC error.
		writeRPCResult(w, req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		})
		return
	}
	if len(content) == 0 {
		content = []byte(`{}`)
	}
	writeRPCResult(w, req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(content)}},
		"isError": false,
	})
}

// --- JSON-RPC envelope helpers ---

func writeRPCResult(w http.ResponseWriter, id int64, result any) {
	writeJSONRPC(w, map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func writeRPCError(w http.ResponseWriter, id int64, code int, message string) {
	writeJSONRPC(w, map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": message},
	})
}

func writeJSONRPC(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
