package search

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/pastlog/pastlog/internal/adapters/claudecode"
	"github.com/pastlog/pastlog/internal/adapters/codex"
	"github.com/pastlog/pastlog/internal/agentlog"
)

// Benchmarks over a synthetic JSONL corpus (spec §7). The corpus is generated
// AT TEST TIME into a temp dir — nothing is committed — and covers both
// line-oriented shapes: claude-code project JSONL and codex rollout JSONL.
//
// Size control (so plain `go test` stays fast — benchmarks only ever run with
// -bench, which is when generation happens at all):
//   - PASTLOG_BENCH_MB=<n>  target corpus size in MB (decimal), default 200
//   - -short                caps the corpus at ~5 MB (bench smoke runs)
//
// The corpus is generated once per test-binary run and shared by every
// benchmark; TestMain removes it on exit.

const benchNeedle = "calibrate" // appears in ~1.5% of generated entries

var benchCorpus struct {
	once sync.Once
	root string // temp root holding the generated home
	home string // the --home passed to the adapters
	size int64  // total JSONL bytes generated
	err  error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if benchCorpus.root != "" {
		os.RemoveAll(benchCorpus.root)
	}
	os.Exit(code)
}

// benchTargetBytes resolves the requested corpus size (see the file comment).
func benchTargetBytes() int64 {
	if v := os.Getenv("PASTLOG_BENCH_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return int64(n) * 1_000_000
		}
	}
	if testing.Short() {
		return 5_000_000
	}
	return 200_000_000
}

func benchHome(b *testing.B) (string, int64) {
	b.Helper()
	benchCorpus.once.Do(func() {
		root, err := os.MkdirTemp("", "pastlog-bench-")
		if err != nil {
			benchCorpus.err = err
			return
		}
		benchCorpus.root = root
		benchCorpus.home = filepath.Join(root, "home")
		benchCorpus.size, err = generateBenchCorpus(benchCorpus.home, benchTargetBytes())
		if err != nil {
			benchCorpus.err = err
		}
	})
	if benchCorpus.err != nil {
		b.Fatal(benchCorpus.err)
	}
	return benchCorpus.home, benchCorpus.size
}

// generateBenchCorpus writes claude-code and codex session files until the
// target size is reached, alternating adapters. Content is deterministic and
// synthetic; every session carries a handful of searchable entries, and the
// needle lands in roughly one entry per session.
func generateBenchCorpus(home string, target int64) (int64, error) {
	var written int64
	for i := 0; written < target; i++ {
		var n int64
		var err error
		if i%2 == 0 {
			n, err = writeBenchClaudeSession(home, i)
		} else {
			n, err = writeBenchCodexSession(home, i)
		}
		if err != nil {
			return written, err
		}
		written += n
	}
	return written, nil
}

func writeBenchClaudeSession(home string, i int) (int64, error) {
	id := fmt.Sprintf("%08x-1111-4111-8111-111111111111", i)
	dir := filepath.Join(home, ".claude", "projects", fmt.Sprintf("C--Users-dev-proj%03d", i%17))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(filepath.Join(dir, id+".jsonl"))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	stamp := func(sec int) string {
		return fmt.Sprintf("2026-08-%02dT%02d:%02d:%02d.150Z", 1+i%27, sec/3600%24, sec/60%60, sec%60)
	}
	fmt.Fprintf(w, `{"type":"summary","summary":"session %d about the widget flux","sessionId":%q}`+"\n", i, id)
	for m := 0; m < 24; m++ {
		base := m * 90
		text := fmt.Sprintf("reviewing module %d of build %d: checking the flux settings and dependencies of component %d", m, i, i+m)
		if m == 12 {
			text = fmt.Sprintf("we need to calibrate the sensor thresholds for module %d before shipping", m)
		}
		fmt.Fprintf(w, `{"type":"user","sessionId":%q,"cwd":"C:\\Users\\dev\\proj%03d","timestamp":%q,"message":{"role":"user","content":%q}}`+"\n",
			id, i%17, stamp(base), text)
		fmt.Fprintf(w, `{"type":"assistant","sessionId":%q,"cwd":"C:\\Users\\dev\\proj%03d","timestamp":%q,"message":{"role":"assistant","content":[{"type":"text","text":"working on module %d: adjusting the widget flux parameters and validating the outputs for build %d"},{"type":"tool_use","name":"edit","input":{"file_path":"C:/dev/proj%03d/src/module_%d.ts","old_string":"threshold = 0.%02d","new_string":"threshold = 0.%02d"}}]}}`+"\n",
			id, i%17, stamp(base+15), m, i, i%17, m, m+10, m+42)
		fmt.Fprintf(w, `{"type":"tool_result","sessionId":%q,"cwd":"C:\\Users\\dev\\proj%03d","timestamp":%q,"message":{"role":"tool","content":[{"type":"tool_result","content":"updated module_%d.ts successfully; %d checks passed"}]}}`+"\n",
			id, i%17, stamp(base+30), m, 12+m)
	}
	fmt.Fprintf(w, `{"type":"system","sessionId":%q,"cwd":"C:\\Users\\dev\\proj%03d","timestamp":%q,"message":{"role":"system","content":"hook: PostToolUse edit"}}`+"\n", id, i%17, stamp(2300))
	if err := w.Flush(); err != nil {
		return 0, err
	}
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func writeBenchCodexSession(home string, i int) (int64, error) {
	id := fmt.Sprintf("%08x-2222-4222-8222-222222222222", i)
	dir := filepath.Join(home, ".codex", "sessions", "2026", "08", fmt.Sprintf("%02d", 1+i%27))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("rollout-2026-08-%02dT%02d-00-00-%s.jsonl", 1+i%27, i%24, id)))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	stamp := func(sec int) string {
		return fmt.Sprintf("2026-08-%02dT%02d:%02d:%02dZ", 1+i%27, sec/3600%24, sec/60%60, sec%60)
	}
	fmt.Fprintf(w, `{"timestamp":%q,"type":"session_meta","payload":{"id":%q,"cwd":"/home/dev/api%03d","cli_version":"0.42.1"}}`+"\n",
		stamp(0), id, i%17)
	for m := 0; m < 24; m++ {
		base := m * 90
		text := fmt.Sprintf("tracing request %d through the gateway: latency budget for route %d looks fine at p99", i*100+m, m)
		if m == 7 {
			text = fmt.Sprintf("we should calibrate the retry budget for route %d against the new SLO", m)
		}
		fmt.Fprintf(w, `{"timestamp":%q,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}}`+"\n",
			stamp(base), text)
		fmt.Fprintf(w, `{"timestamp":%q,"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"inspecting route %d of deployment %d: the token bucket and the circuit breaker settings"},{"type":"output_text","text":"patching gateway route %d: adjusted the flux limiter and re-ran the %d load checks"}]}}`+"\n",
			stamp(base+20), m, i, m, 5+m)
		fmt.Fprintf(w, `{"timestamp":%q,"type":"event_msg","payload":{"type":"agent_message","message":"turn %d of session %d finished"}}`+"\n",
			stamp(base+40), m, i)
	}
	if err := w.Flush(); err != nil {
		return 0, err
	}
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// benchAdapters builds the adapter pair over the corpus home.
func benchAdapters(b *testing.B) ([]agentlog.Adapter, int64) {
	b.Helper()
	home, size := benchHome(b)
	return []agentlog.Adapter{claudecode.New(home), codex.New(home)}, size
}

// BenchmarkPrefilterRaw isolates the raw-line scan: the ASCII case-insensitive
// prefilter predicate applied to every line of the corpus (spec §7 hot path).
// SetBytes reports the scan throughput in MB/s (÷1000 for GB/s).
func BenchmarkPrefilterRaw(b *testing.B) {
	home, size := benchHome(b)
	m, err := NewMatcher(benchNeedle, MatchOptions{})
	if err != nil {
		b.Fatal(err)
	}
	keep := m.KeepRaw()
	var files []string
	err = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	if len(files) == 0 {
		b.Fatal("corpus has no jsonl files")
	}
	b.SetBytes(size)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range files {
			f, err := os.Open(p)
			if err != nil {
				b.Fatal(err)
			}
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
			for sc.Scan() {
				keep(sc.Bytes())
			}
			f.Close()
		}
	}
}

// BenchmarkSearchLiteral is the spec §7 hot path: default case-insensitive
// literal search over the whole corpus (claude-code + codex), with the
// ASCII raw-line prefilter active.
func BenchmarkSearchLiteral(b *testing.B) {
	adapters, size := benchAdapters(b)
	m, err := NewMatcher(benchNeedle, MatchOptions{})
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(size)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := Run(adapters, m, EngineOptions{})
		if len(results) == 0 {
			b.Fatal("benchmark corpus must produce hits")
		}
	}
}

// BenchmarkSearchRegex runs the same corpus in regex mode: the prefilter is
// disabled and every entry is parsed (spec §7 keeps regex opt-in for exactly
// this reason).
func BenchmarkSearchRegex(b *testing.B) {
	adapters, size := benchAdapters(b)
	m, err := NewMatcher("calibrat(e|ion)", MatchOptions{Regex: true})
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(size)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := Run(adapters, m, EngineOptions{})
		if len(results) == 0 {
			b.Fatal("benchmark corpus must produce hits")
		}
	}
}
