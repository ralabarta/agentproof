package session

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzSummarize drives the real normalization path with hostile bytes: input
// is written to a file and summarized exactly the way record does, so a panic
// here is the same failure that would break a recording run. Oversized inputs
// are rejected by the size bound before any parsing happens.
func FuzzSummarize(f *testing.F) {
	f.Add([]byte(`{"model":"gpt-test","prompt":"do work","usage":{"input_tokens":10}}` + "\n"))
	f.Add([]byte("{}\n{invalid\n"))
	f.Add([]byte("not json at all"))
	f.Add([]byte("{\"tool\":{\"name\":\"shell\"},\"prompt\":\"x\"}\n{\"a\":[{\"b\":[{\"c\":1}]}]}\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "session.jsonl")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _ = Summarize("codex", path) // must not panic
	})
}
