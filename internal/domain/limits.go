package domain

// Limits — token & request caps per rolling window (§4.3).
// Used in TWO distinct roles, never mixed (§2):
//   - Issued limits (owner-defined caps on a generated key)
//   - Upstream limits (third-party caps on an upstream key)
//
// A zero-value field with Enabled=false means "no cap in this window".
type Limits struct {
	Tokens   map[LimitWindow]int64 // window -> max tokens
	Requests map[LimitWindow]int64 // window -> max request count
}

// NewLimits returns an empty Limits with maps ready.
func NewLimits() Limits {
	return Limits{Tokens: map[LimitWindow]int64{}, Requests: map[LimitWindow]int64{}}
}

// HasAnyCap reports whether at least one window cap is set.
func (l Limits) HasAnyCap() bool {
	for _, v := range l.Tokens {
		if v > 0 {
			return true
		}
	}
	for _, v := range l.Requests {
		if v > 0 {
			return true
		}
	}
	return false
}

// WindowDuration maps a window label to its rolling duration.
// ponytail: a fixed switch — adding a window means one line here.
func WindowDuration(w LimitWindow) int64 {
	switch w {
	case Window5h:
		return 5 * 60 * 60
	case Window24h:
		return 24 * 60 * 60
	case Window30d:
		return 30 * 24 * 60 * 60
	}
	return 0
}

// AllWindows returns the canonical window set (§4.3).
func AllWindows() []LimitWindow {
	return []LimitWindow{Window5h, Window24h, Window30d}
}
