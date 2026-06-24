package domain

// PromptRule is one pre-configured transformation applied to an incoming
// request body before it reaches the upstream (§4.7 "трансформация промптов").
//
// The router supports a small, predictable rule set (no scripting surface —
// AGENTS.md: no unrequested abstractions / no eval). Kind selects the action:
//
//   - "prepend_system": prepend a fixed system message to the messages array.
//   - "append_system":  append a fixed system message.
//   - "replace_model":  rewrite the "model" field.
//   - "inject_param":   set a top-level JSON parameter (temperature, …).
//
// ponytail: ceiling — name/value-based, JSON-aware but schema-light; no regex,
// no conditionals. Growth path = a typed rule DSL per format if needed.
type PromptRule struct {
	ID     ID
	Name   string
	Kind   string // prepend_system | append_system | replace_model | inject_param
	Value  string // text / model name / JSON-encoded param value
	Param  string // for inject_param: the top-level key (e.g. "temperature")
	Status Status
	CreatedAt Time
	UpdatedAt Time
}
