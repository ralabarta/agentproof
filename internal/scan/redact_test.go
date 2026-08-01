package scan

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

// Raw command output is the one evidence file AgentProof copies verbatim from a
// process it does not control. A secret printed by that process reached disk
// unredacted while the same bytes in a patch were caught, so the retained log
// is redacted with the same rules on the way in.
func TestRedactingWriterRedactsSecretsSplitAcrossWrites(t *testing.T) {
	var sink bytes.Buffer
	writer := NewRedactingWriter(&sink)
	// A child process controls its own write boundaries, so a secret can arrive
	// in fragments that match nothing on their own.
	for _, fragment := range []string{"deploying with AKIA", "ABCDEFGHIJKLMNOP", " to prod\n"} {
		if _, err := writer.Write([]byte(fragment)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	got := sink.String()
	if strings.Contains(got, "AKIAABCDEFGHIJKLMNOP") {
		t.Fatalf("secret survived redaction: %q", got)
	}
	if !strings.Contains(got, "[REDACTED:AP-SECRET-001]") {
		t.Fatalf("expected a redaction marker: %q", got)
	}
	if !strings.Contains(got, "deploying with") || !strings.Contains(got, "to prod") {
		t.Fatalf("surrounding output was lost: %q", got)
	}
}

// The single-line rule matches only the BEGIN header. Emitting the header as
// redacted while writing the key material that follows it verbatim would be a
// redaction that announces itself and leaks anyway.
func TestRedactingWriterSuppressesPrivateKeyBody(t *testing.T) {
	var sink bytes.Buffer
	writer := NewRedactingWriter(&sink)
	body := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEAsecretkeymaterial\nMoreSecretBytesHere\n-----END RSA PRIVATE KEY-----\nback to normal\n"
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	got := sink.String()
	for _, leaked := range []string{"MIIEowIBAAKCAQEAsecretkeymaterial", "MoreSecretBytesHere"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("private key body survived redaction: %q", got)
		}
	}
	if !strings.Contains(got, "[REDACTED:AP-SECRET-003]") {
		t.Fatalf("expected a redaction marker: %q", got)
	}
	if !strings.Contains(got, "back to normal") {
		t.Fatalf("output after the key block was lost: %q", got)
	}
}

// A recorded command's stdout and stderr are copied by separate goroutines
// into the same log, so the writer holding the partial line and the key-block
// state is shared and must not be raced. Run with -race.
func TestRedactingWriterIsSafeForConcurrentStreams(t *testing.T) {
	var sink bytes.Buffer
	writer := NewRedactingWriter(&sink)
	var wg sync.WaitGroup
	for stream := 0; stream < 2; stream++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				if _, err := writer.Write([]byte("line of output\n")); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(sink.String(), "line of output\n"); got != 400 {
		t.Fatalf("interleaved writes lost output: got %d lines, want 400", got)
	}
}

// A process can emit an unbounded line, and buffering it whole to find a line
// boundary would let the observed process choose AgentProof's memory use.
func TestRedactingWriterBoundsAnUnterminatedLine(t *testing.T) {
	var sink bytes.Buffer
	writer := NewRedactingWriter(&sink)
	if _, err := writer.Write(bytes.Repeat([]byte("x"), 3*maxRedactBuffer)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if sink.Len() != 3*maxRedactBuffer {
		t.Fatalf("output was dropped or duplicated: wrote %d, got %d", 3*maxRedactBuffer, sink.Len())
	}
}

// Close is what flushes a trailing line, so output that never ends in a newline
// must still reach the file rather than being silently discarded.
func TestRedactingWriterFlushesUnterminatedOutputOnClose(t *testing.T) {
	var sink bytes.Buffer
	writer := NewRedactingWriter(&sink)
	if _, err := writer.Write([]byte("no trailing newline")); err != nil {
		t.Fatal(err)
	}
	if sink.Len() != 0 {
		t.Fatalf("a partial line was emitted before Close: %q", sink.String())
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if sink.String() != "no trailing newline" {
		t.Fatalf("unterminated output was not flushed: %q", sink.String())
	}
}
