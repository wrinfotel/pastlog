package codex

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// rolloutReader streams either form of a rollout and reports its disk size
// through the underlying file.
type rolloutReader struct {
	*os.File
	reader  io.Reader
	decoder *zstd.Decoder
}

func openRollout(path string) (*rolloutReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	rollout := &rolloutReader{File: file, reader: file}
	if strings.HasSuffix(path, ".zst") {
		decoder, err := zstd.NewReader(file, zstd.WithDecoderConcurrency(1))
		if err != nil {
			file.Close()
			return nil, err
		}
		rollout.reader = decoder
		rollout.decoder = decoder
	}
	return rollout, nil
}

func (r *rolloutReader) Read(p []byte) (int, error) { return r.reader.Read(p) }

func (r *rolloutReader) Close() error {
	if r.decoder != nil {
		r.decoder.Close()
	}
	return r.File.Close()
}

func rolloutStem(path string) string {
	return strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".zst"), ".jsonl")
}
