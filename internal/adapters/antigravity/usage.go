package antigravity

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
	_ "modernc.org/sqlite"
)

type generation struct {
	step    *int64
	at      time.Time
	model   string
	primary agentlog.Usage
	retries []agentlog.Usage
}

func databaseUsage(path string) (agentlog.ModelSplit, bool) {
	var split agentlog.ModelSplit
	found := false
	for _, request := range readGenerations(path) {
		split.Note(request.model)
		if request.primary != (agentlog.Usage{}) {
			split.Observe(request.model, request.primary)
			found = true
		}
		for _, retry := range request.retries {
			split.Observe(request.model, retry)
			found = true
		}
	}
	return split, found
}

func openReadDB(path string) *sql.DB {
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	file := url.URL{Path: filepath.ToSlash(path)}
	db, err := sql.Open("sqlite", "file:"+file.EscapedPath()+"?mode=ro&_pragma=busy_timeout(200)")
	if err != nil {
		return nil
	}
	return db
}

func readGenerations(path string) []generation {
	db := openReadDB(path)
	if db == nil {
		return nil
	}
	defer db.Close()
	rows, err := db.Query(`SELECT data FROM gen_metadata ORDER BY idx`)
	if err != nil {
		return nil // the transcript remains usable on older or busy stores
	}
	defer rows.Close()
	var requests []generation
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) != nil {
			continue
		}
		if request := decodeGeneration(raw); request.model != "" || request.primary != (agentlog.Usage{}) || len(request.retries) > 0 {
			requests = append(requests, request)
		}
	}
	if rows.Err() != nil {
		return nil
	}
	return requests
}

func decodeGeneration(raw []byte) generation {
	var g generation
	var model, primary []byte
	visitProto(raw, func(field int, v uint64, data []byte) {
		switch field {
		case 1:
			model = data
		case 2:
			if data != nil {
				value, n := binary.Uvarint(data)
				if n <= 0 {
					return
				}
				v = value
			}
			if g.step == nil && v <= math.MaxInt64 {
				index := int64(v)
				g.step = &index
			}
		}
	})
	var retries [][]byte
	var displayModel string
	visitProto(model, func(field int, _ uint64, data []byte) {
		switch field {
		case 4:
			primary = data
		case 9:
			g.at = protoTime(protoBytes(data, 4))
		case 17:
			retries = append(retries, protoBytes(data, 2))
		case 19:
			g.model = string(data)
		case 21:
			displayModel = string(data)
		}
	})
	if g.model == "" {
		g.model = displayModel
	}
	g.primary = decodeUsage(primary)
	mirrored := false
	for _, rawRetry := range retries {
		if !mirrored && len(primary) > 0 && bytes.Equal(primary, rawRetry) {
			mirrored = true
			continue
		}
		if retry := decodeUsage(rawRetry); retry != (agentlog.Usage{}) {
			g.retries = append(g.retries, retry)
		}
	}
	return g
}

func decodeUsage(raw []byte) agentlog.Usage {
	var u agentlog.Usage
	visitProto(raw, func(field int, v uint64, data []byte) {
		if data != nil || v > math.MaxInt64 {
			return
		}
		n := int64(v)
		switch field {
		case 2:
			u.Input = n
		case 3:
			u.Output = n
		case 4:
			u.CacheWrite = n
		case 5:
			u.CacheRead = n
		case 9:
			u.Reasoning = n
		}
	})
	u.Input += u.CacheRead + u.CacheWrite
	if u.Reasoning <= u.Output {
		u.ReasoningIncluded = u.Reasoning
	}
	return u
}

func protoTime(raw []byte) time.Time {
	var seconds, nanos uint64
	visitProto(raw, func(field int, value uint64, _ []byte) {
		if field == 1 {
			seconds = value
		}
		if field == 2 {
			nanos = value
		}
	})
	if seconds == 0 || seconds > math.MaxInt64 || nanos >= 1e9 {
		return time.Time{}
	}
	return time.Unix(int64(seconds), int64(nanos))
}

func protoBytes(raw []byte, want int) []byte {
	var result []byte
	visitProto(raw, func(field int, _ uint64, data []byte) {
		if field == want && data != nil {
			result = data
		}
	})
	return result
}

// visitProto visits scalar and length-delimited fields without materializing
// other fields, which can contain entire requests or multimodal responses.
func visitProto(raw []byte, visit func(field int, value uint64, data []byte)) {
	for len(raw) > 0 {
		tag, n := binary.Uvarint(raw)
		if n <= 0 || tag>>3 == 0 || tag>>3 > (1<<29)-1 {
			return
		}
		raw = raw[n:]
		field := int(tag >> 3)
		size := 0
		switch tag & 7 {
		case 0:
			value, n := binary.Uvarint(raw)
			if n <= 0 {
				return
			}
			visit(field, value, nil)
			size = n
		case 1:
			size = 8
		case 2:
			length, n := binary.Uvarint(raw)
			if n <= 0 || length > uint64(len(raw)-n) {
				return
			}
			size = n + int(length)
			visit(field, 0, raw[n:size])
		case 5:
			size = 4
		default:
			return
		}
		if size > len(raw) {
			return
		}
		raw = raw[size:]
	}
}
