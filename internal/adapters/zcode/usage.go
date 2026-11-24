package zcode

import (
	"database/sql"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

type usageRow struct {
	sessionID, modelID string
	startedAt          int64
	tokens             agentlog.Usage
	gross              bool
}

// queryModelUsage uses the provider total when present and the computed
// total otherwise. Both columns are optional in older stores.
func queryModelUsage(db *sql.DB, sessionID string) (*sql.Rows, error) {
	var hasProviderTotal, hasComputedTotal bool
	err := db.QueryRow(`SELECT COUNT(CASE WHEN name='provider_total_tokens' THEN 1 END) > 0,
		COUNT(CASE WHEN name='computed_total_tokens' THEN 1 END) > 0
		FROM pragma_table_info('model_usage')`).Scan(&hasProviderTotal, &hasComputedTotal)
	if err != nil {
		return nil, err
	}
	provider, computed := "NULL", "NULL"
	if hasProviderTotal {
		provider = "NULLIF(provider_total_tokens, 0)"
	}
	if hasComputedTotal {
		computed = "NULLIF(computed_total_tokens, 0)"
	}
	query := `SELECT session_id, model_id, started_at, input_tokens, output_tokens, reasoning_tokens,
		cache_creation_input_tokens, cache_read_input_tokens, COALESCE(` + provider + `, ` + computed + `) FROM model_usage`
	const order = ` ORDER BY started_at, id`
	if sessionID != "" {
		return db.Query(query+` WHERE session_id = ? AND started_at IS NOT NULL`+order, sessionID)
	}
	return db.Query(query + order)
}

func scanUsageRow(rows *sql.Rows) (usageRow, error) {
	var row usageRow
	var total sql.NullInt64
	u := &row.tokens
	err := rows.Scan(&row.sessionID, &row.modelID, &row.startedAt,
		&u.Input, &u.Output, &u.Reasoning, &u.CacheWrite, &u.CacheRead, &total)
	if err != nil {
		return usageRow{}, err
	}
	// A total matching input + output identifies gross input and output in
	// current ZCode telemetry. Without that evidence keep the legacy split.
	row.gross = total.Valid && total.Int64 == u.Input+u.Output
	if row.gross && u.Reasoning <= u.Output {
		u.ReasoningIncluded = u.Reasoning
	}
	return row, nil
}

func (r usageRow) contextTokens() agentlog.CtxTokens {
	u := r.tokens
	cache := u.CacheRead + u.CacheWrite
	if r.gross && cache <= u.Input {
		u.Input -= cache
	}
	return agentlog.CtxTokens{
		Input: u.Input, Output: u.Output, Reasoning: u.Reasoning,
		CacheWrite: u.CacheWrite, CacheRead: u.CacheRead,
	}
}
