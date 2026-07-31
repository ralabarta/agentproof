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
