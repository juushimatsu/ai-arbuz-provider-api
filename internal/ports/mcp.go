package ports

import "context"

// MCPBridge connects to/exports MCP tools (§4.8).
// Narrow surface: tools discovery + tools invocation. ponytail: ceiling —
// resources/prompts/sampling are out of scope; growth path = mcp-go SDK.
type MCPBridge interface {
	// ListTools discovers tools from a connected MCP server.
	ListTools(ctx context.Context, endpoint MCPEndpoint) ([]MCPTool, error)
	// CallTool invokes one tool by name with JSON arguments.
	CallTool(ctx context.Context, endpoint MCPEndpoint, name string, arguments []byte) (MCPToolResult, error)
}

// MCPEndpoint describes how to reach an MCP server.
type MCPEndpoint struct {
	Transport string // "stdio" | "http"
	Address   string // command (stdio) or URL (http)
}

// MCPTool — one tool discovered from an MCP server.
type MCPTool struct {
	Name        string
	Description string
	InputSchema []byte // raw JSON schema
}

// MCPToolResult — outcome of a tool call.
type MCPToolResult struct {
	Content []byte // raw JSON content blocks
	IsError bool
}
