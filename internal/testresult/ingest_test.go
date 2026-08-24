package testresult

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ralabarta/agentproof/internal/evidence"
)

func TestIngestGoTestJSON(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "test.jsonl"), "{\"Action\":\"pass\",\"Package\":\"example\",\"Test\":\"TestOne\",\"Elapsed\":0.01}\n{\"Action\":\"skip\",\"Package\":\"example\",\"Test\":\"TestTwo\"}\n")
	result, records := Ingest(root, []string{"test.jsonl"}, true)
	if !result.Ingested || !result.Passed || result.PassedTests != 1 || result.SkippedTests != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(records) != 1 || records[0].State != evidence.Observed || !records[0].Discovered {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestIngestJUnitFailure(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "junit.xml"), `<testsuite><testcase name="ok" time="0.1"/><testcase name="bad"><failure>no</failure></testcase></testsuite>`)
	result, _ := Ingest(root, []string{"junit.xml"}, true)
	if result.Passed || result.PassedTests != 1 || result.FailedTests != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestIngestJUnitRequiresTestCase(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "junit.xml"), `<testsuite/>`)

	result, records := Ingest(root, []string{"junit.xml"}, true)

	if records[0].State == evidence.Observed {
		t.Fatalf("empty JUnit suite was observed: %#v", records[0])
	}
	if result.Passed {
		t.Fatalf("empty JUnit suite yielded a passing result: %#v", result)
	}
}

func TestIngestRejectsTraversalAndMissing(t *testing.T) {
	root := t.TempDir()
	_, traversal := Ingest(root, []string{"../outside.xml"}, true)
	if traversal[0].State != evidence.Unknown {
		t.Fatalf("expected unknown traversal, got %#v", traversal)
	}
	_, missing := Ingest(root, []string{"missing.xml"}, true)
	if missing[0].State != evidence.Missing {
		t.Fatalf("expected missing artifact, got %#v", missing)
	}
}

// Rejecting a symlinked final component is not containment: an intermediate
// directory symlink puts the whole subtree outside the repository while every
// lexical check still reports a path inside it.
func TestIngestRejectsIntermediateDirectorySymlink(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(outside, "junit.xml"), `<testsuite><testcase name="ok"/></testsuite>`)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	result, records := Ingest(root, []string{"link/junit.xml"}, true)
	if records[0].State == evidence.Observed {
		t.Fatalf("content outside the repository was ingested: %#v", records[0])
	}
	if result.PassedTests != 0 {
		t.Fatalf("tests outside the repository were counted: %#v", result)
	}
}

func TestNoDeclaredResultsRemainOptionalByDefault(t *testing.T) {
	result, records := Ingest(t.TempDir(), nil, false)
	if result.Ingested || records[0].State != evidence.NotObserved || records[0].Required {
		t.Fatalf("unexpected optional result: %#v %#v", result, records)
	}
}

func write(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
