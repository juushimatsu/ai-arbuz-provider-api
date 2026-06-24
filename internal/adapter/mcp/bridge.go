// Package mcp implements ports.MCPBridge — a minimal MCP (Model Context
// Protocol) client over JSON-RPC 2.0. Supports the tool surface only
// (tools/list, tools/call). ponytail: ceiling — no resources/prompts/sampling;
// growth path = github.com/mark3labs/mcp-go.
//
// Transport: streamable HTTP (a single POST per request; newline-delimited
// JSON-RPC). stdio is intentionally omitted: in a server context a child
// process per MCP server is an ops burden we can avoid.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// Bridge is a ports.MCPBridge using streamable-HTTP JSON-RPC.
type Bridge struct {
	http *http.Client
}

func NewBridge() *Bridge {
	return &Bridge{http: &http.Client{Timeout: 60 * time.Second}}
}

// rpcRequest is a JSON-RPC 2.0 request envelope.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcResponse is a JSON-RPC 2.0 response envelope.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// idCounter gives unique request ids per process. ponytail: global mutex; fine
// for a single-node router. Growth path = atomic counter or per-connection.
var (
	idMu  sync.Mutex
	idCtr int64
)

func nextID() int64 {
	idMu.Lock()
	defer idMu.Unlock()
	idCtr++
	return idCtr
}

// call performs one JSON-RPC request to addr and returns the result.
func (b *Bridge) call(ctx context.Context, addr, method string, params any) (json.RawMessage, error) {
	rawParams, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	req := rpcRequest{JSONRPC: "2.0", ID: nextID(), Method: method, Params: rawParams}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, addr, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := b.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mcp call %s: %w", method, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, err
	}
	// Streamable-HTTP may wrap the response in an SSE `data:` line; unwrap if so.
	raw = unwrapSSE(raw)
	var rr rpcResponse
	if err := json.Unmarshal(raw, &rr); err != nil {
		return nil, fmt.Errorf("mcp decode %s: %w (body: %s)", method, err, snippet(raw))
	}
	if rr.Error != nil {
		return nil, fmt.Errorf("mcp %s: [%d] %s", method, rr.Error.Code, rr.Error.Message)
	}
	return rr.Result, nil
}

// unwrapSSE strips a single optional SSE `data:` prefix (streamable-HTTP form).
func unwrapSSE(raw []byte) []byte {
	s := string(raw)
	// take the last `data:` line if any (the JSON-RPC response).
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' {
			line := s[i+1:]
			if len(line) > 5 && line[:5] == "data:" {
				return []byte(line[5:])
			}
		}
	}
	return raw
}

func snippet(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}

// ListTools discovers tools from an MCP server (§4.8).
func (b *Bridge) ListTools(ctx context.Context, endpoint ports.MCPEndpoint) ([]ports.MCPTool, error) {
	if endpoint.Transport != "http" {
		return nil, fmt.Errorf("only http transport supported (got %s)", endpoint.Transport)
	}
	res, err := b.call(ctx, endpoint.Address, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return nil, fmt.Errorf("mcp tools/list decode: %w", err)
	}
	tools := make([]ports.MCPTool, 0, len(out.Tools))
	for _, t := range out.Tools {
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = []byte("{}")
		}
		tools = append(tools, ports.MCPTool{Name: t.Name, Description: t.Description, InputSchema: schema})
	}
	return tools, nil
}

// CallTool invokes one tool by name with JSON arguments (§4.8).
func (b *Bridge) CallTool(ctx context.Context, endpoint ports.MCPEndpoint, name string, arguments []byte) (ports.MCPToolResult, error) {
	if endpoint.Transport != "http" {
		return ports.MCPToolResult{}, fmt.Errorf("only http transport supported (got %s)", endpoint.Transport)
	}
	var args any
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return ports.MCPToolResult{}, fmt.Errorf("invalid tool arguments: %w", err)
		}
	}
	res, err := b.call(ctx, endpoint.Address, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return ports.MCPToolResult{}, err
	}
	var out struct {
		Content json.RawMessage `json:"content"`
		IsError bool            `json:"isError"`
	}
	_ = json.Unmarshal(res, &out)
	content := out.Content
	if len(content) == 0 {
		content = res
	}
	return ports.MCPToolResult{Content: content, IsError: out.IsError}, nil
}

// keep strconv referenced for future id formatting without churn.
var _ = strconv.Itoa
