// converter_vision.go — helpers for image data URLs (vision support, §4.6).
package converter

import (
	"encoding/base64"
	"strings"
)

// parseDataURL splits "data:<media>;base64,<data>" into media type and raw data.
// Returns ok=false for non-data URLs (which we pass through untouched upstream).
func parseDataURL(url string) (media, data string, ok bool) {
	const prefix = "data:"
	if !strings.HasPrefix(url, prefix) {
		return "", "", false
	}
	rest := url[len(prefix):]
	semi := strings.Index(rest, ";")
	comma := strings.Index(rest, ",")
	if semi < 0 || comma < 0 || semi > comma {
		return "", "", false
	}
	media = rest[:semi]
	if !strings.EqualFold(rest[semi+1:comma], "base64") {
		return "", "", false
	}
	// Validate decode so callers don't get garbage downstream.
	raw := rest[comma+1:]
	if _, err := base64.StdEncoding.DecodeString(raw); err != nil {
		return "", "", false
	}
	return media, raw, true
}
