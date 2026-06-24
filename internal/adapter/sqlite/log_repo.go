package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

type LogRepo struct{ db *sql.DB }

func NewLogRepo(db *sql.DB) *LogRepo { return &LogRepo{db: db} }

func (r *LogRepo) Insert(ctx context.Context, l *domain.RequestLog) error {
	l.ID = domain.NewID()
	if l.Timestamp.IsZero() {
		l.Timestamp = time.Now().UTC()
	}
	var reqBody, respBody string
	if l.Payload != nil {
		reqBody = l.Payload.RequestBody
		respBody = l.Payload.ResponseBody
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO request_logs(id, issued_key_id, provider_id, upstream_key_id, model, in_format, out_format,
  prompt_tokens, completion_tokens, total_tokens, success, error_code, latency_ttfb_ms, total_ms,
  streamed, timestamp, payload_request, payload_response)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		l.ID, l.IssuedKeyID, l.ProviderID, l.UpstreamKeyID, l.Model,
		string(l.InFormat), string(l.OutFormat), l.PromptTokens, l.CompletionTokens, l.TotalTokens,
		boolToInt(l.Success), l.ErrorCode, l.LatencyTTFBMs, l.TotalMs, boolToInt(l.Streamed),
		encodeTime(l.Timestamp), reqBody, respBody)
	return err
}

func (r *LogRepo) Get(ctx context.Context, id domain.ID) (*domain.RequestLog, error) {
	row := r.db.QueryRowContext(ctx, selectLog+` WHERE id=?`, id)
	return scanLog(row)
}

func (r *LogRepo) List(ctx context.Context, f ports.LogFilter) ([]domain.RequestLog, error) {
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 200
	}
	var (
		sb   strings.Builder
		args []any
	)
	sb.WriteString(selectLog)
	sb.WriteString(" WHERE 1=1")
	if f.IssuedKeyID != "" {
		sb.WriteString(" AND issued_key_id=?")
		args = append(args, f.IssuedKeyID)
	}
	if f.ProviderID != "" {
		sb.WriteString(" AND provider_id=?")
		args = append(args, f.ProviderID)
	}
	if f.Model != "" {
		sb.WriteString(" AND model=?")
		args = append(args, f.Model)
	}
	if f.Success != nil {
		sb.WriteString(" AND success=?")
		args = append(args, boolToInt(*f.Success))
	}
	if !f.From.IsZero() {
		sb.WriteString(" AND timestamp>=?")
		args = append(args, encodeTime(f.From))
	}
	if !f.To.IsZero() {
		sb.WriteString(" AND timestamp<=?")
		args = append(args, encodeTime(f.To))
	}
	sb.WriteString(" ORDER BY timestamp DESC LIMIT ? OFFSET ?")
	args = append(args, f.Limit, f.Offset)

	rows, err := r.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.RequestLog
	for rows.Next() {
		l, err := scanLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// UsageSum — rolling-window consumption for the limiter (§4.3, §2 issued limits).
// Returns totals since `since` for the issued key. Index idx_log_key_ts serves this.
func (r *LogRepo) UsageSum(ctx context.Context, issuedKeyID domain.ID, since domain.Time) (ports.WindowUsage, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(total_tokens),0), COUNT(*) FROM request_logs
		 WHERE issued_key_id=? AND timestamp>=? AND success=1`,
		issuedKeyID, encodeTime(since))
	var u ports.WindowUsage
	if err := row.Scan(&u.Tokens, &u.Requests); err != nil {
		return ports.WindowUsage{}, err
	}
	return u, nil
}

const selectLog = `SELECT id, issued_key_id, provider_id, upstream_key_id, model, in_format, out_format,
  prompt_tokens, completion_tokens, total_tokens, success, error_code, latency_ttfb_ms, total_ms,
  streamed, timestamp, payload_request, payload_response FROM request_logs`

func scanLog(s scanner) (*domain.RequestLog, error) {
	var l domain.RequestLog
	var inFmt, outFmt, ts string
	var success, streamed int
	var reqBody, respBody string
	if err := s.Scan(&l.ID, &l.IssuedKeyID, &l.ProviderID, &l.UpstreamKeyID, &l.Model,
		&inFmt, &outFmt, &l.PromptTokens, &l.CompletionTokens, &l.TotalTokens,
		&success, &l.ErrorCode, &l.LatencyTTFBMs, &l.TotalMs, &streamed, &ts,
		&reqBody, &respBody); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	l.InFormat = domain.Format(inFmt)
	l.OutFormat = domain.Format(outFmt)
	l.Success = success == 1
	l.Streamed = streamed == 1
	l.Timestamp = decodeTime(ts)
	if reqBody != "" || respBody != "" {
		l.Payload = &domain.PayloadSnapshot{RequestBody: reqBody, ResponseBody: respBody}
	}
	return &l, nil
}
