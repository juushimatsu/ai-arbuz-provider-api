package sqlite

import (
	"encoding/json"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// JSON helpers — store slices/maps as JSON text columns (SQLite has no native array).

func encodeStrings(v []string) string {
	if v == nil {
		return "[]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func decodeStrings(s string) []string {
	if s == "" {
		return nil
	}
	var v []string
	_ = json.Unmarshal([]byte(s), &v)
	return v
}

func encodeMap(v map[domain.LimitWindow]int64) string {
	if v == nil {
		return "{}"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func decodeMap(s string) map[domain.LimitWindow]int64 {
	if s == "" {
		return map[domain.LimitWindow]int64{}
	}
	var v map[domain.LimitWindow]int64
	_ = json.Unmarshal([]byte(s), &v)
	if v == nil {
		v = map[domain.LimitWindow]int64{}
	}
	return v
}

// Time helpers — SQLite has no datetime type; store RFC3339 strings.
// Zero time serializes as "" so we can distinguish "never" from epoch.

func encodeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func decodeTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
