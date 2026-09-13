package session

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ralabarta/agentproof/internal/evidence"
)

func TestDiscoverRetainsNewestThousand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}

	base := time.Now().Add(-time.Hour)
	var oldest string
	for i := range 1000 {
		path := filepath.Join(root, fmt.Sprintf("%04d.jsonl", i))
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, base, base); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			oldest = path
		}
	}
	newest := filepath.Join(root, "z-newest.jsonl")
	tiedLast := filepath.Join(root, "zz-tied.jsonl")
	for _, path := range []string{newest, tiedLast} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(oldest, base.Add(-time.Minute), base.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newest, base.Add(time.Minute), base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tiedLast, base, base); err != nil {
		t.Fatal(err)
	}

	got := Discover("codex", time.Time{})
	if len(got) != 1000 {
		t.Fatalf("Discover() returned %d paths, want 1000", len(got))
	}
	if slices.Contains(got, oldest) {
		t.Fatalf("Discover() retained oldest path %q", oldest)
	}
	if !slices.Contains(got, newest) {
		t.Fatalf("Discover() omitted newest path %q", newest)
	}
	if slices.Contains(got, tiedLast) {
		t.Fatalf("Discover() retained path %q after modification-time tie-break", tiedLast)
	}
	if !slices.IsSorted(got) {
		t.Fatal("Discover() paths are not lexicographically sorted")
	}
}

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
