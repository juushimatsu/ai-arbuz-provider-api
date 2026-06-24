package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// Stats implements ports.Stats over the request_logs table (§4.11).
// All aggregates are SQL-side; we only post-process bucketing for Series.
type Stats struct{ db *sql.DB }

func NewStats(db *sql.DB) *Stats { return &Stats{db: db} }

func (s *Stats) Summary(ctx context.Context, q ports.StatsQuery) (ports.StatsSummary, error) {
	from, to := bounds(q)
	row := s.db.QueryRowContext(ctx, `
SELECT COUNT(*),
       COALESCE(SUM(CASE WHEN success=1 THEN 1 ELSE 0 END),0),
       COALESCE(SUM(CASE WHEN success=0 THEN 1 ELSE 0 END),0),
       COALESCE(SUM(prompt_tokens),0),
       COALESCE(SUM(completion_tokens),0),
       COALESCE(AVG(latency_ttfb_ms),0)
FROM request_logs WHERE timestamp>=? AND timestamp<=?`, from, to)
	var sum ports.StatsSummary
	var avgTTFB float64
	if err := row.Scan(&sum.TotalRequests, &sum.SuccessCount, &sum.ErrorCount,
		&sum.PromptTokens, &sum.CompletionTokens, &avgTTFB); err != nil {
		return ports.StatsSummary{}, err
	}
	if sum.TotalRequests > 0 {
		sum.ErrorRate = float64(sum.ErrorCount) / float64(sum.TotalRequests)
	}
	sum.AvgTTFBMs = avgTTFB
	return sum, nil
}

func (s *Stats) Series(ctx context.Context, q ports.StatsQuery) ([]ports.StatsPoint, error) {
	from, to := bounds(q)
	bucket := q.BucketSeconds
	if bucket <= 0 {
		bucket = 3600
	}
	// ponytail: ceiling — bucket assignment via unix epoch arithmetic in SQL;
	// sqlite has no date_trunc. Works for any bucket >= 1s.
	rows, err := s.db.QueryContext(ctx, `
SELECT (strftime('%s', timestamp) / ?) * ? AS bkt,
       COUNT(*),
       COALESCE(SUM(CASE WHEN success=0 THEN 1 ELSE 0 END),0),
       COALESCE(SUM(total_tokens),0),
       COALESCE(AVG(latency_ttfb_ms),0)
FROM request_logs
WHERE timestamp>=? AND timestamp<=?
GROUP BY bkt ORDER BY bkt`, bucket, bucket, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ports.StatsPoint
	for rows.Next() {
		var bkt int64
		var p ports.StatsPoint
		if err := rows.Scan(&bkt, &p.Count, &p.Errors, &p.Tokens, &p.TTFBMs); err != nil {
			return nil, err
		}
		p.TS = time.Unix(bkt, 0).UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Stats) Breakdown(ctx context.Context, q ports.StatsQuery, dimension string) ([]ports.StatsBucket, error) {
	from, to := bounds(q)
	// ponytail: allowlist dimension → column to avoid SQL injection at this
	// trust boundary (dimension comes from a query string).
	col, ok := breakdownColumn(dimension)
	if !ok {
		col = "model"
	}
	query := `SELECT ` + col + `, COUNT(*), COALESCE(SUM(total_tokens),0)
FROM request_logs WHERE timestamp>=? AND timestamp<=?
GROUP BY ` + col + ` ORDER BY COUNT(*) DESC LIMIT 50`
	rows, err := s.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ports.StatsBucket
	for rows.Next() {
		var b ports.StatsBucket
		var label sql.NullString
		if err := rows.Scan(&label, &b.Count, &b.Tokens); err != nil {
			return nil, err
		}
		if label.Valid {
			b.Label = label.String
		} else {
			b.Label = "(unknown)"
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func breakdownColumn(d string) (string, bool) {
	switch d {
	case "model":
		return "model", true
	case "provider":
		return "provider_id", true
	case "key":
		return "issued_key_id", true
	case "status":
		return "CASE WHEN success=1 THEN 'success' ELSE 'error' END", true
	case "format":
		return "in_format", true
	}
	return "", false
}

func bounds(q ports.StatsQuery) (string, string) {
	from := q.From
	to := q.To
	if from.IsZero() {
		from = time.Now().UTC().Add(-24 * time.Hour)
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	return encodeTime(from), encodeTime(to)
}

// silence unused import warning if domain ever drops out (keeps the package
// self-contained as the schema references domain types indirectly).
var _ = domain.StatusActive
