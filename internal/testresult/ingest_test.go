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

func TestIngestGoTestJSONHandlesRepeatedTerminalOutcomes(t *testing.T) {
	const contradictory = "Go test2json contains contradictory terminal outcomes"
	tests := []struct {
		name         string
		content      string
		wantObserved bool
		wantPassed   bool
		wantPassedN  int
		wantFailedN  int
		wantSkippedN int
		wantDuration int64
		wantReason   string
	}{
		{
			name:       "fail then pass for the same test",
			content:    "{\"Action\":\"fail\",\"Package\":\"example\",\"Test\":\"TestOne\"}\n{\"Action\":\"pass\",\"Package\":\"example\",\"Test\":\"TestOne\"}\n",
			wantReason: contradictory,
		},
		{
			name:       "pass then fail for the same test",
			content:    "{\"Action\":\"pass\",\"Package\":\"example\",\"Test\":\"TestOne\"}\n{\"Action\":\"fail\",\"Package\":\"example\",\"Test\":\"TestOne\"}\n",
			wantReason: contradictory,
		},
		{
			name:       "skip then pass for the same test",
			content:    "{\"Action\":\"skip\",\"Package\":\"example\",\"Test\":\"TestOne\"}\n{\"Action\":\"pass\",\"Package\":\"example\",\"Test\":\"TestOne\"}\n",
			wantReason: contradictory,
		},
		{
			name:       "bench then fail for the same benchmark",
			content:    "{\"Action\":\"bench\",\"Package\":\"example\",\"Test\":\"BenchmarkOne\"}\n{\"Action\":\"fail\",\"Package\":\"example\",\"Test\":\"BenchmarkOne\"}\n",
			wantReason: contradictory,
		},
		{
			name:       "package pass then package fail",
			content:    "{\"Action\":\"pass\",\"Package\":\"example\"}\n{\"Action\":\"fail\",\"Package\":\"example\"}\n",
			wantReason: contradictory,
		},
		{
			name:         "repeated test pass is accepted and counted once",
			content:      "{\"Action\":\"pass\",\"Package\":\"example\",\"Test\":\"TestOne\",\"Elapsed\":0.01}\n{\"Action\":\"pass\",\"Package\":\"example\",\"Test\":\"TestOne\",\"Elapsed\":0.02}\n",
			wantObserved: true,
			wantPassed:   true,
			wantPassedN:  1,
			wantDuration: 30,
		},
		{
			name:         "repeated test fail is accepted and counted once",
			content:      "{\"Action\":\"fail\",\"Package\":\"example\",\"Test\":\"TestOne\"}\n{\"Action\":\"fail\",\"Package\":\"example\",\"Test\":\"TestOne\"}\n",
			wantObserved: true,
			wantFailedN:  1,
		},
		{
			name:         "repeated test skip is accepted and counted once",
			content:      "{\"Action\":\"skip\",\"Package\":\"example\",\"Test\":\"TestOne\"}\n{\"Action\":\"skip\",\"Package\":\"example\",\"Test\":\"TestOne\"}\n",
			wantObserved: true,
			wantPassed:   true,
			wantSkippedN: 1,
		},
		{
			name:         "repeated bench is accepted without counting a test",
			content:      "{\"Action\":\"bench\",\"Package\":\"example\",\"Test\":\"BenchmarkOne\",\"Elapsed\":0.01}\n{\"Action\":\"bench\",\"Package\":\"example\",\"Test\":\"BenchmarkOne\",\"Elapsed\":0.02}\n",
			wantObserved: true,
			wantPassed:   true,
		},
		{
			name:         "repeated package pass is accepted without counting a test",
			content:      "{\"Action\":\"pass\",\"Package\":\"example\"}\n{\"Action\":\"pass\",\"Package\":\"example\"}\n",
			wantObserved: true,
			wantPassed:   true,
		},
		{
			name:         "repeated package skip is accepted without counting a test",
			content:      "{\"Action\":\"skip\",\"Package\":\"example\"}\n{\"Action\":\"skip\",\"Package\":\"example\"}\n",
			wantObserved: true,
			wantPassed:   true,
		},
		{
			name:         "repeated package fail is accepted and still fails the result",
			content:      "{\"Action\":\"fail\",\"Package\":\"example\"}\n{\"Action\":\"fail\",\"Package\":\"example\"}\n",
			wantObserved: true,
			wantFailedN:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "test.jsonl"), tt.content)

			result, records := Ingest(root, []string{"test.jsonl"}, true)

			if got := records[0].State == evidence.Observed; got != tt.wantObserved {
				t.Fatalf("observed = %v, want %v: %#v", got, tt.wantObserved, records[0])
			}
			if result.Passed != tt.wantPassed {
				t.Fatalf("passed = %v, want %v: %#v", result.Passed, tt.wantPassed, result)
			}
			if result.PassedTests != tt.wantPassedN || result.FailedTests != tt.wantFailedN || result.SkippedTests != tt.wantSkippedN {
				t.Fatalf("counts = (%d passed, %d failed, %d skipped), want (%d, %d, %d): %#v", result.PassedTests, result.FailedTests, result.SkippedTests, tt.wantPassedN, tt.wantFailedN, tt.wantSkippedN, result)
			}
			if result.DurationMS != tt.wantDuration {
				t.Fatalf("duration = %dms, want %dms: %#v", result.DurationMS, tt.wantDuration, result)
			}
			if records[0].Reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", records[0].Reason, tt.wantReason)
			}
		})
	}
}

func TestIngestGoTestJSONRequiresRecognizedEvent(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantObserved bool
		wantPassed   bool
		wantReason   string
	}{
		{name: "unrecognized JSON", content: "{}\n", wantReason: "test result contains no events"},
		{name: "incomplete package stream", content: "{\"Action\":\"start\",\"Package\":\"example\"}\n", wantReason: "test result contains no terminal outcome"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "test.jsonl"), tt.content)

			result, records := Ingest(root, []string{"test.jsonl"}, true)

			if got := records[0].State == evidence.Observed; got != tt.wantObserved {
				t.Fatalf("observed = %v, want %v: %#v", got, tt.wantObserved, records[0])
			}
			if result.Passed != tt.wantPassed {
				t.Fatalf("passed = %v, want %v: %#v", result.Passed, tt.wantPassed, result)
			}
			if records[0].Reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", records[0].Reason, tt.wantReason)
			}
		})
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

func TestIngestJUnitDurationValidation(t *testing.T) {
	tests := []struct {
		name          string
		timeAttribute string
		wantObserved  bool
		wantDuration  int64
	}{
		{name: "NaN is rejected", timeAttribute: ` time="NaN"`},
		{name: "positive infinity is rejected", timeAttribute: ` time="Inf"`},
		{name: "negative infinity is rejected", timeAttribute: ` time="-Inf"`},
		{name: "negative duration is rejected", timeAttribute: ` time="-1"`},
		{name: "omitted duration is accepted", wantObserved: true},
		{name: "ordinary duration is accepted", timeAttribute: ` time="1.25"`, wantObserved: true, wantDuration: 1250},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "junit.xml"), `<testsuite><testcase name="case"`+tt.timeAttribute+`/></testsuite>`)

			result, records := Ingest(root, []string{"junit.xml"}, true)

			gotObserved := records[0].State == evidence.Observed
			if gotObserved != tt.wantObserved || result.DurationMS != tt.wantDuration {
				t.Fatalf("observed = %v, duration = %dms; want observed = %v, duration = %dms: %#v", gotObserved, result.DurationMS, tt.wantObserved, tt.wantDuration, records[0])
			}
		})
	}
}

func TestIngestJUnitRequiresJUnitRoot(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantObserved bool
		wantPassed   bool
		wantReason   string
	}{
		{name: "testsuite root", content: `<testsuite><testcase name="ok"/></testsuite>`, wantObserved: true, wantPassed: true},
		{name: "testsuites root", content: `<testsuites><testsuite><testcase name="ok"/></testsuite></testsuites>`, wantObserved: true, wantPassed: true},
		{name: "non-JUnit wrapper", content: `<report><testsuite><testcase name="nested"/></testsuite></report>`, wantReason: "XML is not a JUnit test suite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "junit.xml"), tt.content)

			result, records := Ingest(root, []string{"junit.xml"}, true)

			if got := records[0].State == evidence.Observed; got != tt.wantObserved {
				t.Fatalf("observed = %v, want %v: %#v", got, tt.wantObserved, records[0])
			}
			if result.Passed != tt.wantPassed {
				t.Fatalf("passed = %v, want %v: %#v", result.Passed, tt.wantPassed, result)
			}
			if records[0].Reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", records[0].Reason, tt.wantReason)
			}
			wantPassedTests := 0
			if tt.wantObserved {
				wantPassedTests = 1
			}
			if result.PassedTests != wantPassedTests {
				t.Fatalf("passed tests = %d, want %d: %#v", result.PassedTests, wantPassedTests, result)
			}
		})
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
