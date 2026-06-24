package domain

import "strings"

// DetectFormatByPath identifies the incoming wire format from the request path
// (§4.5 auto-detection). Returns FormatOpenAI as the default.
//
// ponytail: path-based detection is robust because the client picks the SDK
// (OpenAI SDK hits /v1/chat/completions, Anthropic SDK hits /v1/messages).
func DetectFormatByPath(path string) Format {
	switch {
	case strings.HasSuffix(path, "/v1/messages"), strings.Contains(path, "/v1/messages"):
		return FormatAnthropic
	case strings.Contains(path, "/v1/chat/completions"),
		strings.Contains(path, "/v1/models"),
		strings.Contains(path, "/v1/embeddings"):
		return FormatOpenAI
	}
	return FormatOpenAI
}
