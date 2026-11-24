package antigravity

import (
	"time"
	"unicode/utf8"
)

func readDatabaseSteps(path string, visit func(step) error) error {
	db := openReadDB(path)
	if db == nil {
		return nil
	}
	defer db.Close()
	rows, err := db.Query(`SELECT idx, step_type, metadata, step_payload FROM steps
		WHERE step_type IN (14, 15, 23) ORDER BY idx`)
	if err != nil {
		return nil // usage-only databases can still populate stats and context
	}
	defer rows.Close()
	for rows.Next() {
		var index int64
		var kind int
		var meta, payload []byte
		if rows.Scan(&index, &kind, &meta, &payload) != nil {
			continue
		}
		s := databaseStep(index, kind, meta, payload)
		if s.Type != "" {
			if err := visit(s); err != nil {
				return err
			}
		}
	}
	return rows.Err()
}

// Decode the user, assistant and checkpoint payloads verified in the public
// SQLite sample. Tool-specific payloads use separate schemas; prefer JSONL
// for a complete transcript when the CLI saved one.
func databaseStep(index int64, kind int, meta, payload []byte) step {
	s := step{Index: &index}
	if at := protoTime(protoBytes(meta, 1)); !at.IsZero() {
		s.Created = at.UTC().Format(time.RFC3339Nano)
	}
	switch kind {
	case 14:
		s.Type = "USER_INPUT"
		s.Content = protoText(protoBytes(payload, 19), 2)
	case 15:
		s.Type = "PLANNER_RESPONSE"
		planner := protoBytes(payload, 20)
		s.Content = protoText(planner, 1)
		s.Thinking = protoText(planner, 3)
	case 23:
		s.Type = "CHECKPOINT"
	}
	return s
}

func protoText(message []byte, field int) string {
	data := protoBytes(message, field)
	if utf8.Valid(data) {
		return string(data)
	}
	return ""
}
