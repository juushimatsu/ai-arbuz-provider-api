package mcp

import (
	"context"
	"encoding/json"
)

// BuiltinTools describes the router's own MCP tools (§4.8 "publish own tools").
// The actual handlers are injected by the transport layer (which has access to
// the use-cases); this file only centralizes their schemas/descriptions.

// ToolRegistration pairs a tool's static metadata with its handler.
type ToolRegistration struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Handler     ToolHandler
}

// builtinSchema returns the input schema for a known built-in tool name, or nil.
func builtinSchema(name string) json.RawMessage {
	switch name {
	case "list_providers", "list_issued_keys", "list_logs":
		return []byte(`{"type":"object","properties":{"limit":{"type":"integer","default":50}}}`)
	case "run_chat":
		return []byte(`{
			"type":"object",
			"required":["issued_key","model","message"],
			"properties":{
				"issued_key":{"type":"string","description":"An active issued key to bill the request to"},
				"model":{"type":"string"},
				"message":{"type":"string","description":"User message content"}
			}
		}`)
	}
	return nil
}

// RegisterBuiltins registers a standard set of router introspection tools.
// Each handler is provided by the caller (transport), keeping this package free
// of use-case/transport imports (DIP).
type BuiltinDeps struct {
	ListProviders func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
	ListIssued    func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
	ListLogs      func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
	RunChat       func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
}

// RegisterBuiltins installs the builtin tool set onto s.
func RegisterBuiltins(s *Server, d BuiltinDeps) {
	type reg struct{ name, desc string; h ToolHandler }
	for _, r := range []reg{
		{"list_providers", "List configured providers.", d.ListProviders},
		{"list_issued_keys", "List issued (generated) keys, with masked tokens.", d.ListIssued},
		{"list_logs", "List recent request logs.", d.ListLogs},
		{"run_chat", "Run a proxied chat completion through the router.", d.RunChat},
	} {
		if r.h == nil {
			continue
		}
		s.Register(r.name, r.desc, builtinSchema(r.name), r.h)
	}
}
