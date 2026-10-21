package report

import (
	"strings"
	"testing"

	"github.com/ralabarta/agentproof/internal/evidence"
)

// Report values come from the repository and agent under observation, so a
// reviewer reading report.md must not be shown markup those inputs forged.
func TestMarkdownEscapesInlineActiveCharacters(t *testing.T) {
	run := evidence.Run{
		Objective:  "see [report](javascript:alert) and <img src=x> _stress_",
		Status:     "warning",
		Repository: evidence.Repository{Changes: []evidence.Change{{Path: "internal/a_b*.go", Status: "modified"}}},
		Findings:   []evidence.Finding{{ID: "AP001", Severity: "high", Title: "token in [config](http://evil.example)", Path: "a_b.go", Line: 3}},
	}
	markdown := string(Markdown(run, strings.Repeat("a", 64)))
	for _, forged := range []string{"](javascript:alert)", "](http://evil.example)", "<img src=x>"} {
		if strings.Contains(markdown, forged) {
			t.Fatalf("hostile inline markup survived escaping: %q\n%s", forged, markdown)
		}
	}
	if !strings.Contains(markdown, `\_stress\_`) {
		t.Fatalf("emphasis characters were not escaped:\n%s", markdown)
	}
	// Code spans render backslashes literally, so escaping there would corrupt
	// every path and hash the report shows.
	if !strings.Contains(markdown, "`internal/a_b*.go`") {
		t.Fatalf("code spans must stay verbatim, not backslash-mangled:\n%s", markdown)
	}
}

// An artifact the loader could not ingest is classified unknown, but its
// recorded cause never reaches the report, so the reader only learns
// "not passing (0 passed, 0 failed, 0 skipped)" with no explanation of why.
// The report must name each unobserved artifact and its reason, in both
// formats, because the ingestion contract records that reason for review.
func TestReportNamesUnobservedTestArtifactReasons(t *testing.T) {
	run := evidence.Run{
		Objective: "probe",
		Status:    "failed",
		Tests: evidence.TestResult{
			Ingested: true,
			Passed:   false,
			Summary:  "not passing: 0 passed, 0 failed, 0 skipped",
			Artifacts: []evidence.TestArtifact{
				{Path: "results.xml", Format: "junit-xml", State: evidence.Unknown, Reason: "malformed JUnit XML: <id | EOF"},
				{Path: "missing.xml", Format: "junit-xml", State: evidence.Missing, Reason: "declared test result does not exist"},
			},
		},
	}
	markdown := string(Markdown(run, strings.Repeat("a", 64)))
	if !strings.Contains(markdown, "Test evidence is not passing") {
		t.Fatalf("summary bullet missing:\n%s", markdown)
	}
	for _, want := range []string{
		"Artifact `results.xml` (unknown): malformed JUnit XML: \\<id \\| EOF",
		"Artifact `missing.xml` (missing): declared test result does not exist",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown does not name an unobserved artifact and its reason: %q\n%s", want, markdown)
		}
	}
	html, err := HTML(run, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	text := string(html)
	if !strings.Contains(text, "(unknown): malformed JUnit XML") || !strings.Contains(text, "(missing): declared test result does not exist") {
		t.Fatalf("HTML does not name unobserved artifact reasons:\n%s", text)
	}
	if strings.Contains(text, "<id | EOF</") && !strings.Contains(text, "&lt;id") {
		t.Fatalf("HTML echoed the hostile reason unescaped:\n%s", text)
	}
}

func TestReportsEscapeHostilePresentationData(t *testing.T) {
	run := evidence.Run{
		Objective:    "<script>alert(1)</script>\n| injected",
		Status:       "warning",
		Tests:        evidence.TestResult{Summary: "not supplied"},
		Repository:   evidence.Repository{Changes: []evidence.Change{{Path: "a|b.go", Status: "modified"}}, AssociationStatus: "clean-baseline"},
		Impact:       evidence.Impact{Complete: true},
		Completeness: evidence.Completeness{Observed: 1, Required: 1, Percent: 100, Complete: true},
	}
	markdown := string(Markdown(run, strings.Repeat("a", 64)))
	if strings.Contains(markdown, "\n| injected") || !strings.Contains(markdown, `a\|b.go`) {
		t.Fatalf("Markdown was not safely escaped:\n%s", markdown)
	}
	html, err := HTML(run, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	text := string(html)
	if strings.Contains(text, "<script>alert") || !strings.Contains(text, "Content-Security-Policy") {
		t.Fatalf("HTML was not safely escaped or lacks CSP: %s", text)
	}
}
