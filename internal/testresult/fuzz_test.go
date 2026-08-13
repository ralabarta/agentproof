package testresult

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzIngest drives the bounded JUnit and Go test2json parsers with hostile
// bytes: declared artifacts are untrusted input that verify ingests as data
// and never executes, so the parsers must never panic. Oversized artifacts
// are rejected by the per-file byte bound before parsing happens.
func FuzzIngest(f *testing.F) {
	f.Add([]byte(`<testsuite tests="1"><testcase name="x"/></testsuite>`))
	f.Add([]byte(`<testsuite><testcase name="ok"><failure>no</failure></testcase></testsuite>`))
	f.Add([]byte(`{"Action":"pass","Package":"example","Test":"TestOne"}` + "\n"))
	f.Add([]byte(`{not json`))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		root := t.TempDir()
		path := filepath.Join(root, "artifact")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _ = Ingest(root, []string{"artifact"}, false) // must not panic
	})
}
