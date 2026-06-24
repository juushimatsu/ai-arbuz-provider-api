// mcp_wrapper.go — MCP → REST/OpenAI wrapper (§4.8): expose an MCP server that
// has no REST API as if it were an OpenAI-compatible service.
//
// The wrapper presents the MCP server's tools as OpenAI "models" (one model id
// per tool: "mcp-<server>-<tool>") and turns a chat-completions request for such
// a "model" into a tools/call on the underlying MCP server, returning the tool
// output as the assistant message.
//
// ponytail: ceiling — this is a DETERMINISTIC dispatcher, not an LLM agent
// loop. The client picks the tool by naming it as the "model" and supplies args
// in the message body as JSON. Growth path = a real agent loop that lets the
// model choose tools, with multi-turn tool_call orchestration.
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// MCPWrapper wraps an MCP server into an OpenAI-compatible chat interface.
type MCPWrapper struct {
	bridge ports.MCPBridge
	repo   ports.MCPRepo
}

func NewMCPWrapper(bridge ports.MCPBridge, repo ports.MCPRepo) *MCPWrapper {
	return &MCPWrapper{bridge: bridge, repo: repo}
}

// ListModelsAsOpenAI returns the tools of a saved MCP server as OpenAI model
// entries, so the server looks like a normal /v1/models provider.
func (w *MCPWrapper) ListModelsAsOpenAI(ctx context.Context, serverID domain.ID) ([]byte, error) {
	m, err := w.repo.Get(ctx, serverID)
	if err != nil {
		return nil, err
	}
	// Discover live tools if none cached.
	tools := m.Tools
	if len(tools) == 0 {
		discovered, derr := w.bridge.ListTools(ctx, ports.MCPEndpoint{Transport: m.Transport, Address: m.Address})
		if derr != nil {
			return nil, derr
		}
		tools = make([]domain.MCPTool, 0, len(discovered))
		for _, t := range discovered {
			tools = append(tools, domain.MCPTool{Name: t.Name, Description: t.Description, InputSchema: string(t.InputSchema)})
		}
	}
	data := make([]map[string]string, 0, len(tools))
	for _, t := range tools {
		data = append(data, map[string]string{"id": w.modelID(m.Name, t.Name), "object": "model"})
	}
	return json.Marshal(map[string]any{"object": "list", "data": data})
}

// InvokeChat turns an OpenAI chat-completions body whose "model" is an MCP-wrapped
// tool into a tools/call, returning an OpenAI-shaped chat completion response.
//
// Expected body shape: {"model":"mcp-<server>-<tool>","messages":[{"role":"user",
// "content":"<json arguments>"}]}. The LAST user message's content is parsed as
// the tool's JSON arguments.
func (w *MCPWrapper) InvokeChat(ctx context.Context, serverID domain.ID, body []byte) ([]byte, error) {
	m, err := w.repo.Get(ctx, serverID)
	if err != nil {
		return nil, err
	}
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid chat body: %w", err)
	}
	// Resolve the real tool name by matching the model id against this server's
	// tools (audit #12). Splitting on the last "-" was wrong when a server or
	// tool slug contained "-", and it returned the lossy *slug* instead of the
	// tool's real name (which CallTool needs). Exact id matching is unambiguous.
	tools := m.Tools
	if len(tools) == 0 {
		discovered, derr := w.bridge.ListTools(ctx, ports.MCPEndpoint{Transport: m.Transport, Address: m.Address})
		if derr != nil {
			return nil, derr
		}
		tools = make([]domain.MCPTool, 0, len(discovered))
		for _, t := range discovered {
			tools = append(tools, domain.MCPTool{Name: t.Name})
		}
	}
	toolName := ""
	for _, t := range tools {
		if w.modelID(m.Name, t.Name) == req.Model {
			toolName = t.Name
			break
		}
	}
	if toolName == "" {
		return nil, fmt.Errorf("model %q is not an MCP-wrapped tool of this server (expected mcp-<server>-<tool>)", req.Model)
	}
	// Take the last user message as the tool arguments (JSON).
	args := []byte("{}")
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			if c := strings.TrimSpace(req.Messages[i].Content); c != "" {
				args = []byte(c)
			}
			break
		}
	}
	result, err := w.bridge.CallTool(ctx, ports.MCPEndpoint{Transport: m.Transport, Address: m.Address}, toolName, args)
	if err != nil {
		return nil, err
	}
	text := string(result.Content)
	return json.Marshal(map[string]any{
		"id": "mcp-" + serverID, "object": "chat.completion", "model": req.Model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": text},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	})
}

// modelID builds the synthetic OpenAI "model" id for an MCP tool.
func (w *MCPWrapper) modelID(serverName, toolName string) string {
	return "mcp-" + slug(serverName) + "-" + slug(toolName)
}


func slug(s string) string {
	out := strings.ToLower(strings.TrimSpace(s))
	out = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, out)
	return out
}