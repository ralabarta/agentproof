package scan

import (
	"slices"
	"strings"
	"testing"
)

// FuzzRedact keeps secret redaction panic-free on hostile input. Redaction
// runs before a patch is persisted, so a crash here would take down the
// recording step that exists to protect the evidence.
func FuzzRedact(f *testing.F) {
	f.Add("normal text")
	f.Add("password=secret123")
	f.Add("-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----")
	f.Add("AKIAIOSFODNN7EXAMPLE")
	f.Add("ghp_" + strings.Repeat("a", 40))
	f.Fuzz(func(t *testing.T, value string) {
		_ = RedactString(value) // must not panic
	})
}

// FuzzScanPatch keeps the bounded patch scanner honest: a hostile diff must
// never panic, and a line beyond the scanner cap must surface as visible
// incomplete-scan evidence instead of growing memory or crashing.
func FuzzScanPatch(f *testing.F) {
	f.Add("+++ b/auth.go\n@@ -0,0 +1 @@\n+token = abc123def\n")
	f.Add("+++ b/x.go\n@@ -1 +1 @@\n+password: 12345678\n")
	f.Add("+++ b/big.go\n@@" + strings.Repeat(" ", 1024) + "\n+" + strings.Repeat("x", 1024) + "\n")
	f.Add(strings.Repeat("+", maxPatchLine))
	f.Add(strings.Repeat("+", maxPatchLine) + "\n")
	f.Add(strings.Repeat("+", maxPatchLine+1))
	f.Fuzz(func(t *testing.T, patch string) {
		first := Run(nil, patch)
		second := Run(nil, patch)
		if !slices.Equal(first, second) {
			t.Fatalf("scan findings are not deterministic: first=%#v second=%#v", first, second)
		}
		if !hasLineOverScanLimit(patch) {
			return
		}
		count := 0
		for _, finding := range first {
			if finding.ID == "AP-SCAN-001" {
				count++
				if finding.Result != "unknown" {
					t.Fatalf("AP-SCAN-001 result = %q, want unknown", finding.Result)
				}
			}
		}
		if count != 1 {
			t.Fatalf("AP-SCAN-001 count = %d, want 1; findings=%#v", count, first)
		}
	})
}

func hasLineOverScanLimit(value string) bool {
	lineBytes := 0
	for i := 0; i < len(value); i++ {
		if value[i] == '\n' {
			lineBytes = 0
			continue
		}
		lineBytes++
		if lineBytes >= maxPatchLine {
			return true
		}
	}
	return false
}
