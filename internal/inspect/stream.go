package inspect

import (
	"errors"
	"io"
	"strings"
)

// ErrBlocked terminates a streamed response when the guard (block mode) detects
// a high-severity injection mid-stream. The transport surfaces it as a failed
// request; bytes already forwarded are partial, but a tool call is only acted
// on by the client once its arguments fully arrive — which is what we catch.
var ErrBlocked = errors.New("response blocked by security guard")

// guardReader taps a streamed (SSE) provider response: it forwards bytes
// unchanged while accumulating them and scanning the running buffer. In "block"
// mode a high-severity hit aborts the stream; in "alert" mode it only reports.
//
// ponytail: re-scans the accumulated buffer on each read (O(n^2) worst case) and
// caps the scanned window at maxScan bytes. Ceiling = fine for normal replies.
// Growth path = incremental SSE delta parsing if huge streams matter.
type guardReader struct {
	r       io.ReadCloser
	e       *Engine
	block   bool
	onFind  func([]Finding)
	buf     strings.Builder
	fired   bool
}

const maxScan = 256 * 1024

// NewStreamGuard wraps r so the response stream is inspected as it flows.
// onFind (may be nil) is invoked at most once with the findings. When block is
// true and a high-severity finding appears, the stream ends with ErrBlocked.
func NewStreamGuard(r io.ReadCloser, e *Engine, block bool, onFind func([]Finding)) io.ReadCloser {
	if e == nil || r == nil {
		return r
	}
	return &guardReader{r: r, e: e, block: block, onFind: onFind}
}

func (g *guardReader) Read(p []byte) (int, error) {
	n, err := g.r.Read(p)
	if n > 0 && !g.fired {
		if g.buf.Len() < maxScan {
			g.buf.Write(p[:n])
		}
		if fs := g.e.Inspect(g.buf.String(), "stream"); len(fs) > 0 {
			g.fired = true
			if g.onFind != nil {
				g.onFind(fs)
			}
			if g.block && MaxSeverity(fs) >= SevHigh {
				_ = g.r.Close()
				return n, ErrBlocked
			}
		}
	}
	return n, err
}

func (g *guardReader) Close() error { return g.r.Close() }
