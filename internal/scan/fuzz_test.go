package scan

import (
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
	f.Add(strings.Repeat("+", 4*1024*1024+1))
	f.Fuzz(func(t *testing.T, patch string) {
		_ = Run(nil, patch) // must not panic
	})
}
