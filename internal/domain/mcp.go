package domain

// MCPKind classifies an MCP entry's role (§4.8).
type MCPKind string

const (
	MCPClientRole  MCPKind = "client"  // connect to external MCP server, expose its tools
	MCPServerRole  MCPKind = "server"  // publish own tools as an MCP server
	MCPWrapperRole MCPKind = "wrapper" // wrap an MCP server into a REST/OpenAI endpoint
)

// MCPServer — a managed MCP connection/publishment (§4.8).
type MCPServer struct {
	ID        ID
	Name      string
	Kind      MCPKind
	Transport string // "stdio" | "http"
	// Address: command+args (stdio) OR URL (http)
	Address   string
	Tools     []MCPTool
	Status    Status
	CreatedAt Time
	UpdatedAt Time
}

// MCPTool — one tool exposed by an MCP server.
type MCPTool struct {
	Name        string
	Description string
	InputSchema string // raw JSON schema string
}
