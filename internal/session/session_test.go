package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralabarta/agentproof/internal/evidence"
)

func TestSummarizeValidJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := `{"model":"gpt-test","prompt":"do work","tool":{"name":"shell"},"usage":{"input_tokens":10,"output_tokens":4}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Summarize("codex", path)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != evidence.Observed || got.PromptCount != 1 || got.Usage.InputTokens != 10 || len(got.Models) != 1 || got.Models[0] != "gpt-test" {
		t.Fatalf("unexpected summary: %#v", got)
	}
	if !strings.HasPrefix(got.Digest, "sha256:") {
		t.Fatalf("missing digest: %s", got.Digest)
	}
}

func TestSummarizeMalformedAndOversizedAreUnknown(t *testing.T) {
	root := t.TempDir()
	malformed := filepath.Join(root, "malformed.jsonl")
	if err := os.WriteFile(malformed, []byte("{not-json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := Summarize("codex", malformed)
	if got.State != evidence.Unknown || got.Reason == "" {
		t.Fatalf("malformed source was not unknown: %#v", got)
	}
	oversized := filepath.Join(root, "oversized.jsonl")
	f, err := os.Create(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(32*1024*1024 + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	got, _ = Summarize("codex", oversized)
	if got.State != evidence.Unknown || !strings.Contains(got.Reason, "32 MiB") {
		t.Fatalf("oversized source was not bounded: %#v", got)
	}
}
